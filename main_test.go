package main

import (
	"strings"
	"testing"
)

func TestIsMarkdown(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"README.md", true},
		{"notes.markdown", true},
		{"notes.mdown", true},
		{"notes.mkd", true},
		{"notes.mdx", true},
		{"README.MD", true},
		{"file.txt", false},
		{"markdown", false},
	}
	for _, tt := range tests {
		if got := isMarkdown(tt.name); got != tt.want {
			t.Errorf("isMarkdown(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestStripTrailingPadding(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"hello  \nworld\t\n", "hello\nworld\n"},
		{"no trailing\n", "no trailing\n"},
		{"one line  ", "one line"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := stripTrailingPadding(tt.in); got != tt.want {
			t.Errorf("stripTrailingPadding(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderMarkdownPlain(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	var buf strings.Builder
	err := renderMarkdown("# Title\n\nHello **world**.\n", &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Title") || !strings.Contains(out, "Hello") || !strings.Contains(out, "world") {
		t.Errorf("markdown heading and body missing from output: %q", out)
	}
	// stripTrailingPadding: no output line may end in padding.
	for _, line := range strings.Split(out, "\n") {
		if line != strings.TrimRight(line, " \t") {
			t.Errorf("output line has trailing padding: %q", line)
		}
	}
}

func TestRenderMarkdownStyled(t *testing.T) {
	t.Setenv("CATT_COLOR", "yes")
	var buf strings.Builder
	if err := renderMarkdown("**bold**\n", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("CATT_COLOR=yes should produce ANSI-styled output, got %q", buf.String())
	}
}

// TestRenderBytesDispatch checks that files are routed to the right
// renderer based on their name.
func TestRenderBytesDispatch(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	tests := []struct {
		name     string
		data     string
		contains string
	}{
		{"doc.md", "# Hello\n", "Hello"},
		{"table.csv", "a,b\n1,2\n", "a"},
	}
	for _, tt := range tests {
		var buf strings.Builder
		if err := renderBytes(tt.name, []byte(tt.data), &buf); err != nil {
			t.Errorf("renderBytes(%q): %v", tt.name, err)
			continue
		}
		if !strings.Contains(buf.String(), tt.contains) {
			t.Errorf("renderBytes(%q) = %q, want it to contain %q", tt.name, buf.String(), tt.contains)
		}
	}
}
