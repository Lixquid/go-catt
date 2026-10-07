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

func TestSanitizeCSVCell(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		// Pipes are no longer escaped: cell contents are not parsed as
		// markdown, so they render verbatim.
		{`has | pipe`, `has | pipe`},
		{"two\nlines", "two lines"},
		{"crlf\r\nlines", "crlf lines"},
		{"cr\rlines", "cr lines"},
	}
	for _, tt := range tests {
		if got := sanitizeCSVCell(tt.in); got != tt.want {
			t.Errorf("sanitizeCSVCell(%q) = %q, want %q", tt.in, got, tt.want)
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
	for _, want := range []string{"name", "desc", "alice", "bob", "has | pipe"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderCSV output missing %q: %q", want, out)
		}
	}
	// The header separator has exactly two columns: the pipe inside the
	// data cell must not have created extra columns.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "--") {
			if n := strings.Count(line, "|"); n != 1 {
				t.Errorf("separator row has %d inner pipes, want 1: %q", n, line)
			}
		}
	}
	// Rows appear in order: header first, then alice, then bob.
	if strings.Index(out, "alice") > strings.Index(out, "bob") {
		t.Errorf("rows out of order: %q", out)
	}
	// No leading or trailing newlines: output starts and ends at the
	// table's own border lines.
	if strings.HasPrefix(out, "\n") {
		t.Errorf("renderCSV output has a leading newline: %q", out)
	}
	if strings.HasSuffix(out, "\n") {
		t.Errorf("renderCSV output has a trailing newline: %q", out)
	}
}

// Cells must be treated as raw CSV data: markdown syntax is displayed
// verbatim, not interpreted by the renderer.
func TestRenderCSVMixedContentVerbatim(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	cells := []string{
		"# not a heading",
		"*not* **emphasis**",
		"`not code`",
		"[not a link](http://example.com)",
		":smile: not an emoji",
		"\\not an escape",
		"has | pipe",
		"a\\*b",
	}
	content := "cell\n"
	for _, c := range cells {
		content += "\"" + strings.ReplaceAll(c, "\"", "\"\"") + "\"\n"
	}
	var buf strings.Builder
	if err := renderCSV(content, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range cells {
		if !strings.Contains(out, want) {
			t.Errorf("mixed-content cell not rendered verbatim, missing %q in %q", want, out)
		}
	}
}

// A single-row CSV renders as a well-formed table (header only), not
// as a paragraph of raw markdown.
func TestRenderCSVSingleRow(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	var buf strings.Builder
	if err := renderCSV("a,b\n", &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Errorf("single-row CSV missing cells: %q", out)
	}
	// It must look like a table: a separator row of dashes with one
	// inner pipe for the two columns.
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "--") {
			found = true
			if n := strings.Count(line, "|"); n != 1 {
				t.Errorf("single-row separator has %d inner pipes, want 1: %q", n, line)
			}
		}
	}
	if !found {
		t.Errorf("single-row CSV did not render a table separator: %q", out)
	}
}

// Ragged rows are padded with empty cells to the widest row.
func TestRenderCSVRaggedRows(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	var buf strings.Builder
	if err := renderCSV("a,b,c\n1\n2,3\n", &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "--") {
			if n := strings.Count(line, "|"); n != 2 {
				t.Errorf("separator row has %d inner pipes, want 2: %q", n, line)
			}
		}
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

// With styling forced on, cell text carries the dark style's color and
// every line is padded to the wrap width, matching glamour's markdown
// table output.
func TestRenderCSVDarkStyle(t *testing.T) {
	t.Setenv("CATT_COLOR", "yes")
	var buf strings.Builder
	if err := renderCSV("a,b\n1,2\n", &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "\x1b[38;5;252ma\x1b[0m") {
		t.Errorf("dark-style cell text missing color 252: %q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;252m \x1b[0m") {
		t.Errorf("dark-style output missing styled padding spaces: %q", out)
	}
}
