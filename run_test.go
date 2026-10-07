package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunStdinDash is a regression test: "-" used to be rejected as an
// unknown flag; it now reads stdin like cat.
func TestRunStdinDash(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	var stdout, stderr bytes.Buffer
	code := run([]string{"-"}, strings.NewReader("hello\n"), &stdout, &stderr)
	if code != 0 {
		t.Errorf("run([\"-\"]) exit code = %d, want 0 (stderr: %q)", code, stderr.String())
	}
	if got := stdout.String(); got != "hello\n" {
		t.Errorf("run([\"-\"]) stdout = %q, want %q", got, "hello\n")
	}
}

// TestRunContinuesAfterFailedFile is a regression test: a file that
// cannot be opened used to abort the whole run; now the remaining
// arguments are still processed and the exit code is non-zero at the
// end, like cat.
func TestRunContinuesAfterFailedFile(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	good := filepath.Join("examples", "hello.go")
	var stdout, stderr bytes.Buffer
	code := run([]string{"/no/such/file", good}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "/no/such/file") {
		t.Errorf("stderr should mention the failed file, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "package main") {
		t.Errorf("the file after the failed one should still be rendered, got %q", stdout.String())
	}
}

func TestRunMultipleStdin(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	// "-" can appear among file arguments and is read in order.
	var stdout, stderr bytes.Buffer
	code := run([]string{"-", "extra.txt"}, strings.NewReader("from stdin\n"), &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1 (missing file)", code)
	}
	if !strings.Contains(stdout.String(), "from stdin") {
		t.Errorf("stdin should be rendered before the failure, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "extra.txt") {
		t.Errorf("stderr should mention extra.txt, got %q", stderr.String())
	}
}

func TestRunUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-x"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `unknown flag "-x"`) {
		t.Errorf("stderr = %q, want unknown flag message", stderr.String())
	}
}

func TestRunHelp(t *testing.T) {
	tests := [][]string{{"-h"}, {"--help"}}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		code := run(args, strings.NewReader(""), &stdout, &stderr)
		if code != 0 {
			t.Errorf("run(%q) exit code = %d, want 0", args, code)
		}
		if !strings.Contains(stdout.String(), "usage: catt") {
			t.Errorf("run(%q) should print usage, got %q", args, stdout.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("run(%q) should write nothing to stderr, got %q", args, stderr.String())
		}
	}
}

// TestRunNoArgsReadsStdin checks that stdin is read when no file
// arguments are given.
func TestRunNoArgsReadsStdin(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	var stdout, stderr bytes.Buffer
	code := run(nil, strings.NewReader("plain text\n"), &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (stderr: %q)", code, stderr.String())
	}
	if got := stdout.String(); got != "plain text\n" {
		t.Errorf("stdout = %q, want %q", got, "plain text\n")
	}
}

// TestRunStdinRenderedByType checks that stdin content is rendered
// according to the requested type (e.g. "-" feeding a markdown file
// is rendered as markdown).
func TestRunStdinRenderedByType(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	var stdout, stderr bytes.Buffer
	code := run([]string{"-", "a.md"}, strings.NewReader("# Hi\n"), &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "Hi") {
		t.Errorf("stdin should be rendered as markdown, got %q", stdout.String())
	}
}

// TestRunDoesNotTouchRealStdio guards against the tests accidentally
// using os.Stdin/os.Stdout: run must only write to the writers it is
// given.
func TestRunDoesNotTouchRealStdio(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run([]string{"-h"}, os.Stdin, &stdout, &stderr)
	if !strings.Contains(stdout.String(), "usage: catt") {
		t.Error("usage should go to the stdout writer passed to run")
	}
}
