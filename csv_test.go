package main

import (
	"strings"
	"testing"
)

func TestIsCSV(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"data.csv", true},
		{"DATA.CSV", true},
		{"data.tsv", false},
		{"csv", false},
	}
	for _, tt := range tests {
		if got := isCSV(tt.name); got != tt.want {
			t.Errorf("isCSV(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParseCSVRagged(t *testing.T) {
	records, err := parseCSV("a,b,c\n1\n2,3\n")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"a", "b", "c"}, {"1"}, {"2", "3"}}
	if len(records) != len(want) {
		t.Fatalf("got %d records, want %d", len(records), len(want))
	}
	for i, row := range records {
		if strings.Join(row, ",") != strings.Join(want[i], ",") {
			t.Errorf("record %d = %q, want %q", i, row, want[i])
		}
	}
}

func TestEscapeMarkdownCell(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{`has | pipe`, `has \| pipe`},
		{"two\nlines", "two lines"},
		{"crlf\r\nlines", "crlf lines"},
		{"cr\rlines", "cr lines"},
	}
	for _, tt := range tests {
		if got := escapeMarkdownCell(tt.in); got != tt.want {
			t.Errorf("escapeMarkdownCell(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderCSVPlain(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	var buf strings.Builder
	err := renderCSV("name,desc\nalice,\"has | pipe\"\nbob,\n", &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"name", "desc", "alice", "bob", "has | pipe", "---"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderCSV output missing %q: %q", want, out)
		}
	}
	// The escaped pipe must not break the table: the header separator
	// still has exactly two columns.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "--") {
			if n := strings.Count(line, "|"); n != 1 {
				t.Errorf("separator row has %d inner pipes, want 1 (escaped pipe broke the table): %q", n, line)
			}
		}
	}
	// The second CSV row is the markdown separator, so "alice" is the
	// first body row and comes before "bob".
	if strings.Index(out, "alice") > strings.Index(out, "bob") {
		t.Errorf("rows out of order: %q", out)
	}
}

func TestRenderCSVEmpty(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	// Empty content falls back to the markdown renderer.
	var buf strings.Builder
	if err := renderCSV("", &buf); err != nil {
		t.Fatalf("renderCSV of empty content: %v", err)
	}
}

func TestRenderCSVInvalid(t *testing.T) {
	var buf strings.Builder
	// An unclosed quote is a parse error.
	if err := renderCSV("a,\"b\n", &buf); err == nil {
		t.Error("renderCSV with malformed CSV should fail")
	}
}
