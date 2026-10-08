// catt — cat(1) clone.
//
// Like cat, it prints any file to output. Files with a markdown
// extension are rendered with glamour. Code files are printed with
// syntax highlighting via chroma. PNG, JPEG, and GIF files are displayed
// as sixel graphics on terminals that support them. Zip and tar
// archives are listed as a tree of files.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/mattn/go-isatty"
)

var markdownExtensions = map[string]bool{
	".md":       true,
	".markdown": true,
	".mdown":    true,
	".mkd":      true,
	".mdx":      true,
}

func isMarkdown(name string) bool {
	return markdownExtensions[strings.ToLower(filepath.Ext(name))]
}

// useDarkStyle decides whether output is styled ("dark") or plain
// ("ascii"). CATT_COLOR=yes forces styling on, CATT_COLOR=no forces it
// off; otherwise styling is used on a terminal and plain output when
// piped. The same decision applies to markdown and code files so both
// always share the same styling.
func useDarkStyle() bool {
	switch strings.ToLower(os.Getenv("CATT_COLOR")) {
	case "yes", "y", "true", "1":
		return true
	case "no", "n", "false", "0":
		return false
	}
	return isatty.IsTerminal(os.Stdout.Fd())
}

// renderMarkdown renders a markdown document with glamour. Dark
// styling on a terminal, ascii rendering when piped; CATT_COLOR=yes/no
// overrides the detection. The selected style is customized to use a
// zero document margin. Trailing padding is stripped from the output.
func renderMarkdown(content string, w io.Writer) error {
	style := "ascii"
	if useDarkStyle() {
		style = "dark"
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithStylesFromJSONBytes([]byte(`{
    "document": {
        "margin": 0
    }
}`)))
	if err != nil {
		return fmt.Errorf("failed to create renderer: %w", err)
	}

	out, err := r.Render(content)
	if err != nil {
		return fmt.Errorf("failed to render markdown: %w", err)
	}
	fmt.Fprint(w, stripTrailingPadding(out))
	return nil
}

// stripTrailingPadding removes trailing whitespace from every line of
// glamour output, which pads lines out to the wrap width.
func stripTrailingPadding(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func usage(w io.Writer) {
	fmt.Fprint(w, `usage: catt [file ...]
       catt < file
       catt -
       catt -h | catt --help

Like cat, catt prints any file to stdout. Feed it stdin when no file is
given, or pass "-" explicitly as a file name.

Markdown files are converted into pretty output. Code files are
printed with syntax highlighting. CSV files are rendered as tables,
identical in appearance to tables from markdown files. Archive files
(zip, tar, tgz, tar.gz, tbz, tar.bz2) are listed as a tree of files,
drawn with grey box-drawing characters. All use the same
dark styling on a terminal and plain output when piped.

PNG, JPEG, and GIF files are displayed as sixel graphics on terminals
that support them, and passed through untouched otherwise. Animated
GIFs are shown as their first frame.

Set CATT_COLOR=yes to force dark styling even when piped, or
CATT_COLOR=no to force plain output even on a terminal (which also
disables sixel rendering).

When output goes to a terminal and is larger than the viewport, it is
piped through a pager. The pager comes from the PAGER environment
variable (which may include arguments) and defaults to more on
Windows and less elsewhere. Set CATT_PAGE=yes to always paginate, or
CATT_PAGE=no to never paginate.

Set CATT_MAX_ARCHIVE_SIZE (e.g. 10MB, 500KB, or a plain byte count)
to skip archives that would need decompressing (tgz, tar.gz, tbz,
tar.bz2) or spooling to disk (zip fed over stdin) when they exceed
the limit; plain tar and zip files are read in place and never hit
the limit.

Options:
  -h, --help    show this help
`)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run implements the catt command line and returns the process exit
// code. stdin is read when there are no file arguments or when "-" is
// given; like cat, run keeps going after a failed file so the
// remaining arguments are still processed, and the exit code is
// non-zero if anything failed.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			usage(stdout)
			return 0
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			fmt.Fprintf(stderr, "catt: unknown flag %q\n", a)
			return 1
		}
	}

	// Output larger than the terminal viewport is paged; see
	// newPagerWriter for the automatic decision and CATT_PAGE.
	out := newPagerWriter(stdout)

	catIn := func() error {
		if f, ok := stdin.(*os.File); ok && isatty.IsTerminal(f.Fd()) {
			usage(stderr)
			os.Exit(1)
		}
		// Sniff a small prefix so archives can be streamed or spooled
		// instead of being buffered in full.
		head := make([]byte, archiveHeadSize)
		n, err := io.ReadFull(stdin, head)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return fmt.Errorf("could not read stdin: %w", err)
		}
		return renderStream("<stdin>", head[:n], stdin, out)
	}

	exitCode := 0
	if len(args) == 0 {
		if err := catIn(); err != nil {
			fmt.Fprintf(stderr, "catt: %v\n", err)
			exitCode = 1
		}
	}
	for _, path := range args {
		// Each file makes its own pagination decision, so a short
		// file after a long one still prints directly.
		out.reset()
		var err error
		if path == "-" {
			err = catIn()
		} else {
			err = catFile(path, out)
		}
		if err != nil {
			fmt.Fprintf(stderr, "catt: %v\n", err)
			exitCode = 1
		}
	}
	if err := out.finish(); err != nil {
		fmt.Fprintf(stderr, "catt: %v\n", err)
		exitCode = 1
	}
	return exitCode
}

func catFile(path string, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer f.Close()

	// Archives are never read into memory in full: tar-family ones
	// stream, and zip ones are read in place via the seekable file.
	// The extension decides first, with a small header sniff as
	// fallback for misnamed files.
	format := archiveFormatName(path)
	var head []byte
	if format == "" {
		head = make([]byte, archiveHeadSize)
		n, err := io.ReadFull(f, head)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return fmt.Errorf("%s: %w", path, err)
		}
		head = head[:n]
		format = archiveFormatHead(head)
	}
	if format != "" {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return renderArchiveFile(path, f, format, w)
	}

	// Regular content: read it all (the sniffed prefix is prepended
	// so nothing is lost) and dispatch by name.
	data, err := io.ReadAll(io.MultiReader(bytes.NewReader(head), f))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	return renderBytes(path, data, w)
}

// renderStream renders input that may not be seekable (stdin), given
// a small sniffed prefix and the remaining reader. Archives are
// handled without buffering the whole input; other content is read
// into memory and dispatched by name (and magic bytes).
func renderStream(name string, head []byte, r io.Reader, w io.Writer) error {
	if format := archiveFormatHead(head); format != "" {
		compressed := archiveHeadCompressed(head)
		return renderArchiveStream(name, format, compressed, io.MultiReader(bytes.NewReader(head), r), w)
	}
	data, err := io.ReadAll(io.MultiReader(bytes.NewReader(head), r))
	if err != nil {
		return fmt.Errorf("could not read %s: %w", name, err)
	}
	return renderBytes(name, data, w)
}

// renderBytes renders already-loaded file contents based on the file
// name. Archives are dispatched earlier (catFile or renderStream) so
// they never reach this buffered path.
func renderBytes(name string, data []byte, w io.Writer) error {
	if isMarkdown(name) {
		return renderMarkdown(string(data), w)
	}
	if isImage(name) {
		// Images manage the terminal themselves (sixel graphics), so
		// they never go through the pager.
		return renderImage(name, data, unwrapPager(w))
	}
	if isCSV(name) {
		return renderCSV(string(data), w)
	}
	return renderCode(name, string(data), w)
}
