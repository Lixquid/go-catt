// Syntax highlighting for code files, powered by chroma.
package main

import (
	"fmt"
	"io"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// defaultCodeStyle is the chroma style used when stdout is a
// terminal. It is catt's own dark style, derived from glamour's dark
// markdown style so code files and markdown code blocks look the same.
const defaultCodeStyle = cattDarkStyleName

// highlightable reports whether chroma has a lexer registered for the
// file's name (by extension or well-known filename, e.g. Makefile).
func highlightable(name string) bool {
	return lexers.Match(name) != nil
}

// renderCode pretty-prints a source file with syntax highlighting.
// Color output on a terminal (style "native", matching the dark
// styling used for markdown), plain passthrough like cat when piped;
// CATT_COLOR=yes/no forces color on or off.
func renderCode(filename, content string, w io.Writer) error {
	lexer := lexers.Match(filename)
	if lexer == nil {
		lexer = lexers.Analyse(content)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	// Chroma's line splitting swallows a missing final newline, which
	// would glue the last token to the next prompt line; restore it.
	if !endsWithNewline(content) {
		content += "\n"
	}

	styleName := ""
	formatterName := "noop" // plain text, identical to cat
	if useDarkStyle() {
		styleName = defaultCodeStyle
		formatterName = "terminal256"
	}

	style := styles.Fallback
	if styleName != "" {
		style = styles.Get(styleName)
	}
	formatter := formatters.Get(formatterName)

	iterator, err := lexer.Tokenise(nil, content)
	if err != nil {
		return fmt.Errorf("failed to tokenize %s: %w", filename, err)
	}
	if err := formatter.Format(w, style, iterator); err != nil {
		return fmt.Errorf("failed to format %s: %w", filename, err)
	}
	return nil
}

func endsWithNewline(s string) bool {
	return len(s) == 0 || s[len(s)-1] == '\n'
}
