// catt — cat(1) clone.
//
// Like cat, it prints any file to output. Files with a markdown
// extension are rendered with glamour.
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

// renderMarkdown renders a markdown document with glamour. Color
// output on a terminal, plain rendering when piped; CATT_STYLE
// overrides the style. Trailing padding is stripped from the output.
func renderMarkdown(content string, w io.Writer) error {
	style := "ascii"
	if isatty.IsTerminal(os.Stdout.Fd()) {
		style = "dark"
	}
	if s := os.Getenv("CATT_STYLE"); s != "" {
		style = s
	}

	out, err := glamour.Render(content, style)
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
	fmt.Fprintf(w, `usage: catt [file ...]
       catt < file
       catt -h | catt --help

Like cat, catt prints any file to stdout. Feed it stdin when no file is given.

Markdown files are converted into pretty output.

Style will use dark by default but automatically switch to ascii when piped.
Override with CATT_STYLE.

Options:
  -h, --help    show this help
`, "")
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
		if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
			fmt.Fprintf(os.Stderr, "catt: could not read stdin: %v\n", err)
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

	if isMarkdown(path) {
		return renderMarkdown(string(data), w)
	}
	_, err = w.Write(data)
	if err == nil && len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Fprintln(w)
	}
	return err
}
