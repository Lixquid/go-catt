package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageMagicFormat(t *testing.T) {
	tests := []struct {
		name string
		head []byte
		want string
	}{
		{"png", []byte("\x89PNG\r\n\x1a\n\x00\x00"), "png"},
		{"jpeg", []byte("\xff\xd8\xff\xe0\x00\x10"), "jpeg"},
		{"gif87a", []byte("GIF87a extra"), "gif"},
		{"gif89a", []byte("GIF89a extra"), "gif"},
		// Truncated magic bytes must not match.
		{"png truncated", []byte("\x89PNG\r\n\x1a"), ""},
		{"jpeg truncated", []byte("\xff\xd8"), ""},
		{"gif truncated", []byte("GIF89"), ""},
		// Non-images must not match.
		{"text", []byte("hello world\n"), ""},
		{"empty", nil, ""},
		{"elf", []byte("\x7fELF\x02\x01\x01\x00"), ""},
	}
	for _, tt := range tests {
		if got := imageMagicFormat(tt.head); got != tt.want {
			t.Errorf("imageMagicFormat(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestRenderBytesMagicImage checks that an image whose name gives no
// hint (no extension) is routed to the image renderer by magic bytes.
// The forced kitty protocol makes the routing observable: only the
// image renderer emits kitty APC escape sequences.
func TestRenderBytesMagicImage(t *testing.T) {
	t.Setenv("CATT_COLOR", "yes")
	t.Setenv("CATT_IMAGE_PROTOCOL", "kitty")
	data := tinyPNG(t)

	for _, name := range []string{"photo", "photo.bin", "photo.png"} {
		var buf bytes.Buffer
		if err := renderBytes(name, data, &buf); err != nil {
			t.Fatalf("renderBytes(%q): %v", name, err)
		}
		if out := buf.String(); !strings.Contains(out, "\x1b_G") {
			t.Errorf("renderBytes(%q) should route PNG magic bytes to the image renderer, got %q", name, out)
		}
	}
}

// TestRenderBytesMagicStdinImage checks the same routing for stdin
// input, whose name "<stdin>" can never match an extension.
func TestRenderBytesMagicStdinImage(t *testing.T) {
	t.Setenv("CATT_COLOR", "yes")
	t.Setenv("CATT_IMAGE_PROTOCOL", "kitty")
	data := tinyPNG(t)

	var buf bytes.Buffer
	head := data
	if len(head) > archiveHeadSize {
		head = head[:archiveHeadSize]
	}
	if err := renderStream("<stdin>", head, bytes.NewReader(data[len(head):]), &buf); err != nil {
		t.Fatalf("renderStream: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "\x1b_G") {
		t.Errorf("stdin PNG should route to the image renderer via magic bytes, got %q", out)
	}
}

// TestRenderBytesMagicNoFalsePositive checks that text without image
// magic still goes to the code renderer, which highlights the shebang.
func TestRenderBytesMagicNoFalsePositive(t *testing.T) {
	t.Setenv("CATT_COLOR", "yes")
	t.Setenv("CATT_IMAGE_PROTOCOL", "kitty")
	var buf bytes.Buffer
	script := "#!/bin/sh\necho hi\n"
	if err := renderBytes("no-extension", []byte(script), &buf); err != nil {
		t.Fatalf("renderBytes: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "\x1b[") {
		t.Errorf("shebang script should be highlighted as code, got %q", out)
	}
	if out := buf.String(); strings.Contains(out, "\x1b_G") {
		t.Errorf("shebang script must not be routed to the image renderer, got %q", out)
	}
}

// TestRunMagicImageFile is an end-to-end check: an extensionless PNG
// on disk passes through byte for byte, like cat, when no graphics
// protocol is in play.
func TestRunMagicImageFile(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	path := filepath.Join(t.TempDir(), "photo")
	data := tinyPNG(t)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{path}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr: %q", code, stderr.String())
	}
	if got, want := stdout.String(), string(data)+"\n"; got != want {
		t.Errorf("extensionless PNG output = %d bytes, want passthrough of %d bytes", len(got), len(want))
	}
}
