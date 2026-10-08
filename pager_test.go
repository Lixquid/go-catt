package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPagerOffWhenPiped checks the default: output to a non-terminal
// (here a bytes.Buffer, like a pipe) is never paged.
func TestPagerOffWhenPiped(t *testing.T) {
	t.Setenv("CATT_PAGE", "")
	t.Setenv("PAGER", "")
	var stdout, stderr bytes.Buffer
	long := strings.Repeat("line\n", 1000)
	code := run([]string{"-"}, strings.NewReader(long), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d (stderr: %q)", code, stderr.String())
	}
	if stdout.String() != long {
		t.Error("long piped output should pass through untouched")
	}
}

// TestPagerForcedOn pipes output through the pager even though stdout
// is not a terminal. cat stands in for a pager so the content must
// come out unchanged.
func TestPagerForcedOn(t *testing.T) {
	t.Setenv("CATT_PAGE", "yes")
	t.Setenv("PAGER", "cat")
	var stdout, stderr bytes.Buffer
	code := run([]string{"-"}, strings.NewReader("through the pager\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d (stderr: %q)", code, stderr.String())
	}
	if stdout.String() != "through the pager\n" {
		t.Errorf("stdout = %q, want paged content", stdout.String())
	}
}

// TestPagerForcedOff checks that CATT_PAGE=no keeps output direct even
// when pagination would otherwise be considered.
func TestPagerForcedOff(t *testing.T) {
	t.Setenv("CATT_PAGE", "no")
	var stdout, stderr bytes.Buffer
	code := run([]string{"-"}, strings.NewReader("no pager here\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d (stderr: %q)", code, stderr.String())
	}
	if stdout.String() != "no pager here\n" {
		t.Errorf("stdout = %q, want direct content", stdout.String())
	}
}

// TestPagerShortOutputNotPaged drives a pagerWriter with a known
// viewport: output that fits must be written straight to the writer.
func TestPagerShortOutputNotPaged(t *testing.T) {
	t.Setenv("PAGER", "")
	pw := &pagerWriter{w: &bytes.Buffer{}, rows: 24, cols: 80}
	if _, err := pw.Write([]byte("just a few lines\n")); err != nil {
		t.Fatal(err)
	}
	if pw.cmd != nil {
		t.Fatal("short output must not start a pager")
	}
	if err := pw.finish(); err != nil {
		t.Fatal(err)
	}
	if got := pw.w.(*bytes.Buffer).String(); got != "just a few lines\n" {
		t.Errorf("output = %q, want direct write", got)
	}
}

// TestPagerLongOutputPaged drives a pagerWriter with a two-row
// viewport: the third line of output must trigger the pager, and cat
// (standing in for the pager) delivers the full content.
func TestPagerLongOutputPaged(t *testing.T) {
	t.Setenv("PAGER", "cat")
	var out bytes.Buffer
	pw := &pagerWriter{w: &out, rows: 2, cols: 80}
	long := strings.Repeat("line\n", 5)
	if _, err := pw.Write([]byte(long)); err != nil {
		t.Fatal(err)
	}
	if pw.cmd == nil {
		t.Fatal("output taller than the viewport must start a pager")
	}
	if err := pw.finish(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != long {
		t.Errorf("paged output = %q, want %q", got, long)
	}
}

// TestPagerWrappedLines exercises the row estimate: with a 10-column
// terminal a 21-character line wraps onto three rows.
func TestPagerWrappedLines(t *testing.T) {
	pw := &pagerWriter{w: &bytes.Buffer{}, rows: 3, cols: 10}
	// Three wrapped rows on the first line would exceed the viewport
	// once more lines are added, but the first write alone stays in
	// the buffer.
	if _, err := pw.Write([]byte("012345678901234567890\n")); err != nil {
		t.Fatal(err)
	}
	if pw.row != 3 || pw.cmd != nil {
		t.Fatalf("row = %d, want 3; pager started = %v", pw.row, pw.cmd != nil)
	}
	// Any further content must overflow and start the pager.
	t.Setenv("PAGER", "cat")
	var out bytes.Buffer
	pw.w = &out
	if _, err := pw.Write([]byte("more\n")); err != nil {
		t.Fatal(err)
	}
	if pw.cmd == nil {
		t.Fatal("wrapped output must overflow the viewport")
	}
	if err := pw.finish(); err != nil {
		t.Fatal(err)
	}
	want := "012345678901234567890\nmore\n"
	if got := out.String(); got != want {
		t.Errorf("paged output = %q, want %q", got, want)
	}
}

// TestPagerANSINotCounted checks that ANSI styling in the buffered
// output does not inflate the display width.
func TestPagerANSINotCounted(t *testing.T) {
	pw := &pagerWriter{w: &bytes.Buffer{}, rows: 4, cols: 10}
	// 5 visible characters wrapped in style codes.
	if _, err := pw.Write([]byte("\x1b[31mhello\x1b[0m\n")); err != nil {
		t.Fatal(err)
	}
	if pw.row != 1 || pw.col != 0 {
		t.Fatalf("row = %d, col = %d, want 1 and 0", pw.row, pw.col)
	}
}

// TestPagerResetBetweenFiles checks that a short file after a long one
// is written directly again when no pager is running.
func TestPagerResetBetweenFiles(t *testing.T) {
	t.Setenv("PAGER", "cat")
	var out bytes.Buffer
	pw := &pagerWriter{w: &out, rows: 2, cols: 80}
	if _, err := pw.Write([]byte(strings.Repeat("long\n", 5))); err != nil {
		t.Fatal(err)
	}
	if pw.cmd == nil {
		t.Fatal("expected the first file to start a pager")
	}
	// With a pager running, reset must keep the session alive.
	pw.reset()
	if pw.cmd == nil {
		t.Fatal("reset must not stop a running pager")
	}
	if _, err := pw.Write([]byte("still paged\n")); err != nil {
		t.Fatal(err)
	}
	if err := pw.finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), "still paged\n") {
		t.Errorf("output = %q, want continued pager session", out.String())
	}
}

// TestPagerArgv checks pager command resolution.
func TestPagerArgv(t *testing.T) {
	t.Setenv("PAGER", "")
	if got := pagerArgv(); strings.Join(got, " ") != "less -R" {
		t.Errorf("pagerArgv() = %q, want [less -R]", got)
	}
	t.Setenv("PAGER", "less -FX")
	got := pagerArgv()
	if len(got) != 2 || got[0] != "less" || got[1] != "-FX" {
		t.Errorf("pagerArgv() = %q, want [less -FX]", got)
	}
}

// TestPageModeFromEnv checks CATT_PAGE parsing.
func TestPageModeFromEnv(t *testing.T) {
	tests := []struct {
		value           string
		wantOn, wantOff bool
	}{
		{"", false, false},
		{"yes", true, false},
		{"YES", true, false},
		{"1", true, false},
		{"no", false, true},
		{"false", false, true},
		{"0", false, true},
		{"maybe", false, false},
	}
	for _, tt := range tests {
		t.Setenv("CATT_PAGE", tt.value)
		on, off := pageModeFromEnv()
		if on != tt.wantOn || off != tt.wantOff {
			t.Errorf("CATT_PAGE=%q: on=%v off=%v, want on=%v off=%v",
				tt.value, on, off, tt.wantOn, tt.wantOff)
		}
	}
}

// TestPagerSixelBypasses checks that image output reaches the terminal
// directly instead of going through the pager.
func TestPagerSixelBypasses(t *testing.T) {
	t.Setenv("CATT_PAGE", "")
	var buf bytes.Buffer
	pw := &pagerWriter{w: &buf}
	if got := unwrapPager(pw); got != &buf {
		t.Error("unwrapPager must return the writer under the pagerWriter")
	}
	if got := unwrapPager(&buf); got != &buf {
		t.Error("unwrapPager must pass plain writers through")
	}

	// End to end: an image-named file that is not a valid image passes
	// through like cat, bypassing the pager even when pagination is
	// forced on.
	t.Setenv("CATT_PAGE", "yes")
	t.Setenv("PAGER", "cat")
	f := filepath.Join(t.TempDir(), "broken.png")
	if err := os.WriteFile(f, []byte("raw bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{f}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d (stderr: %q)", code, stderr.String())
	}
	if stdout.String() != "raw bytes\n" {
		t.Errorf("stdout = %q, want direct passthrough", stdout.String())
	}
}
