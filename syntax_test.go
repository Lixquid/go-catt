package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestEndsWithNewline(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"x\n", true},
		{"x", false},
		{"x\ny", false},
	}
	for _, tt := range tests {
		if got := endsWithNewline(tt.in); got != tt.want {
			t.Errorf("endsWithNewline(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestRenderCodePlain(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	// With styling off, the noop formatter reproduces the source text
	// byte for byte.
	content := "package main\n\nfunc main() { println(\"hi\") }\n"
	var buf bytes.Buffer
	if err := renderCode("hello.go", content, &buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != content {
		t.Errorf("renderCode plain output = %q, want %q", got, content)
	}
}

// TestRenderCodeRestoresFinalNewline checks that the missing final
// newline is restored, so the last token is not glued to the next
// prompt line.
func TestRenderCodeRestoresFinalNewline(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	var buf bytes.Buffer
	if err := renderCode("hello.go", "package main", &buf); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "package main\n"; got != want {
		t.Errorf("renderCode output = %q, want %q", got, want)
	}
}

func TestRenderCodeStyled(t *testing.T) {
	t.Setenv("CATT_COLOR", "yes")
	var buf bytes.Buffer
	if err := renderCode("hello.go", "func main() {}\n", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("CATT_COLOR=yes should produce ANSI-styled output, got %q", buf.String())
	}
}

// TestRenderCodeUnknownFilePassthrough is a regression test: files
// chroma has no lexer for used to be run through lexers.Fallback and
// the color formatter, so plain text was recolored on a terminal. They
// are now passed through untouched, like cat.
func TestRenderCodeUnknownFilePassthrough(t *testing.T) {
	t.Setenv("CATT_COLOR", "yes")
	content := "just some ordinary words, nothing special\n"
	var buf bytes.Buffer
	if err := renderCode("notes.xyzunknown", content, &buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != content {
		t.Errorf("unknown file should pass through untouched, got %q", got)
	}
}

// TestRenderCodeAnalysedContent checks that content with no matching
// extension is still highlighted when chroma can identify it from the
// text itself.
func TestRenderCodeAnalysedContent(t *testing.T) {
	t.Setenv("CATT_COLOR", "yes")
	var buf bytes.Buffer
	if err := renderCode("no-extension", "#!/bin/sh\necho hi\n", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("shebang script should be analysed and styled, got %q", buf.String())
	}
}
