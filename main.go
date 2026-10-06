// catt — cat(1) clone.
//
// Like cat, it prints any file to output. Files with a markdown
// extension are rendered with glamour. Code files are printed with
// syntax highlighting via chroma. PNG, JPEG, and GIF files are displayed
// as sixel graphics on terminals that support them. Zip and tar
// archives are listed as a tree of files.
package main

import (
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
       catt -h | catt --help

Like cat, catt prints any file to stdout. Feed it stdin when no file is given.

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

Options:
  -h, --help    show this help
`)
}

func main() {
	args := os.Args[1:]

	for _, a := range args {
		if a == "-h" || a == "--help" {
			usage(os.Stdout)
			return
		}
		if strings.HasPrefix(a, "-") {
			fmt.Fprintf(os.Stderr, "catt: unknown flag %q\n", a)
			os.Exit(1)
		}
	}

	if len(args) == 0 {
		// No file: read stdin like cat does.
		if isatty.IsTerminal(os.Stdin.Fd()) {
			usage(os.Stderr)
			os.Exit(1)
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "catt: could not read stdin: %v\n", err)
			os.Exit(1)
		}
		if err := renderBytes("<stdin>", data, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "catt: %v\n", err)
			os.Exit(1)
		}
		return
	}

	for _, path := range args {
		if err := catFile(path, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "catt: %v\n", err)
			os.Exit(1)
		}
	}
}

func catFile(path string, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	return renderBytes(path, data, w)
}

// renderBytes renders already-loaded file contents based on the file
// name (and, for archives, magic bytes).
func renderBytes(name string, data []byte, w io.Writer) error {
	if isMarkdown(name) {
		return renderMarkdown(string(data), w)
	}
	if isImage(name) {
		return renderImage(name, data, w)
	}
	if isCSV(name) {
		return renderCSV(string(data), w)
	}
	if isArchive(name) || archiveFormat(name, data) != "" {
		return renderArchive(name, data, w)
	}
	if highlightable(name) {
		return renderCode(name, string(data), w)
	}
	_, err := w.Write(data)
	if err == nil && len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Fprintln(w)
	}
	return err
}
