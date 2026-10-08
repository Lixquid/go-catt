package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
)

func TestConfiguredImageProtocol(t *testing.T) {
	tests := []struct {
		env  map[string]string
		want imageProtocol
	}{
		// Nothing set: automatic detection.
		{nil, protocolAuto},
		{map[string]string{"CATT_IMAGE_PROTOCOL": "auto"}, protocolAuto},
		// Protocol variable wins, in all its spellings.
		{map[string]string{"CATT_IMAGE_PROTOCOL": "kitty"}, protocolKitty},
		{map[string]string{"CATT_IMAGE": "kitty"}, protocolKitty},
		{map[string]string{"CATT_GRAPHICS": "kitty"}, protocolKitty},
		{map[string]string{"CATT_IMAGE_PROTOCOL": "sixel"}, protocolSixel},
		{map[string]string{"CATT_IMAGE_PROTOCOL": "none"}, protocolNone},
		{map[string]string{"CATT_IMAGE_PROTOCOL": "raw"}, protocolNone},
		{map[string]string{"CATT_IMAGE_PROTOCOL": "Kitty"}, protocolKitty},
		{map[string]string{"CATT_IMAGE_PROTOCOL": " sixel "}, protocolSixel},
		// Unrecognized values leave the choice to detection.
		{map[string]string{"CATT_IMAGE_PROTOCOL": "bogus"}, protocolAuto},
		// CATT_KITTY=yes is shorthand for forcing kitty.
		{map[string]string{"CATT_KITTY": "yes"}, protocolKitty},
		{map[string]string{"CATT_KITTY": "1"}, protocolKitty},
		{map[string]string{"CATT_KITTY": "kitty"}, protocolKitty},
		// CATT_KITTY=no only disables detection; it does not force a
		// protocol, so sixels remain a fallback.
		{map[string]string{"CATT_KITTY": "no"}, protocolAuto},
		// The protocol variable takes precedence over the shorthand.
		{map[string]string{"CATT_KITTY": "yes", "CATT_IMAGE_PROTOCOL": "sixel"}, protocolSixel},
	}
	for _, tt := range tests {
		// t.Setenv values persist for the rest of the test, so start
		// each case from a clean slate.
		for _, name := range []string{"CATT_IMAGE_PROTOCOL", "CATT_IMAGE", "CATT_GRAPHICS", "CATT_KITTY"} {
			t.Setenv(name, "")
			os.Unsetenv(name)
		}
		for name, value := range tt.env {
			t.Setenv(name, value)
		}
		if got := configuredImageProtocol(); got != tt.want {
			t.Errorf("configuredImageProtocol(%v) = %d, want %d", tt.env, got, tt.want)
		}
	}
}

func TestKittyDisabled(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"no", true},
		{"0", true},
		{"false", true},
		{"off", true},
		{"yes", false},
		{"bogus", false},
	}
	for _, tt := range tests {
		t.Setenv("CATT_KITTY", tt.value)
		if got := kittyDisabled(); got != tt.want {
			t.Errorf("kittyDisabled(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestKittyScaledSize(t *testing.T) {
	// Without a terminal to query, the width is capped at
	// fallbackSixelWidth. Unlike sixels, kitty pixels are square so no
	// width doubling is applied.
	tests := []struct {
		srcW, srcH   int
		wantW, wantH int
	}{
		// Small images keep their native size.
		{100, 50, 100, 50},
		// Exactly at the cap.
		{2000, 1000, 2000, 1000},
		// Wide images are shrunk to fit the cap.
		{3000, 1000, 2000, 2000 * 1000 / 3000},
		// Degenerate sizes are clamped to at least one pixel.
		{0, 0, 1, 1},
	}
	for _, tt := range tests {
		w, h := kittyScaledSize(tt.srcW, tt.srcH)
		if w != tt.wantW || h != tt.wantH {
			t.Errorf("kittyScaledSize(%d, %d) = (%d, %d), want (%d, %d)",
				tt.srcW, tt.srcH, w, h, tt.wantW, tt.wantH)
		}
	}
}

func TestEncodeKitty(t *testing.T) {
	// A 2x2 image becomes one escape sequence carrying RGBA data for
	// four pixels, with m=0 because no chunks follow.
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{A: 255})

	var raw []byte
	for y := 0; y < 2; y++ {
		raw = append(raw, img.Pix[y*img.Stride:y*img.Stride+8]...)
	}
	want := "\x1b_Ga=T,f=32,s=2,v=2,q=2,m=0;" +
		base64.RawStdEncoding.EncodeToString(raw) + "\x1b\\\r\n"

	var buf bytes.Buffer
	if err := encodeKitty(img, &buf); err != nil {
		t.Fatalf("encodeKitty: %v", err)
	}
	if got := buf.String(); got != want {
		t.Errorf("encodeKitty = %q, want %q", got, want)
	}
}

// tinyPNG returns a valid 2x2 PNG image, encoded in memory.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRenderImageForcedKitty(t *testing.T) {
	// Forcing the kitty protocol renders even without terminal
	// detection, producing a kitty graphics escape sequence.
	t.Setenv("CATT_IMAGE_PROTOCOL", "kitty")
	var buf bytes.Buffer
	if err := renderImage("x.png", tinyPNG(t), &buf); err != nil {
		t.Fatalf("renderImage: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "\x1b_Ga=T,f=32,s=2,v=2,") {
		t.Errorf("renderImage output %q is not a kitty graphics sequence", got)
	}
	if !strings.HasSuffix(got, "\x1b\\\r\n") {
		t.Errorf("renderImage output %q is not properly terminated", got)
	}
}

func TestRenderImageForcedSixel(t *testing.T) {
	// Forcing sixels produces a sixel DCS sequence.
	t.Setenv("CATT_IMAGE_PROTOCOL", "sixel")
	var buf bytes.Buffer
	if err := renderImage("x.png", tinyPNG(t), &buf); err != nil {
		t.Fatalf("renderImage: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "\x1bP") || !strings.HasSuffix(got, "\x1b\\\r\n") {
		t.Errorf("renderImage output %q is not a sixel sequence", got)
	}
}

func TestRenderImageForcedNone(t *testing.T) {
	// CATT_IMAGE_PROTOCOL=none passes image bytes through untouched,
	// even when they are a valid image.
	t.Setenv("CATT_IMAGE_PROTOCOL", "none")
	data := tinyPNG(t)
	var buf bytes.Buffer
	if err := renderImage("x.png", data, &buf); err != nil {
		t.Fatalf("renderImage: %v", err)
	}
	if got := buf.String(); got != string(data)+"\n" {
		t.Errorf("renderImage = %q, want raw PNG passthrough", got)
	}
}

func TestRenderImageInvalidDataForcedProtocol(t *testing.T) {
	// Invalid image data passes through even with a protocol forced,
	// since there is nothing to decode.
	for _, proto := range []string{"kitty", "sixel"} {
		t.Setenv("CATT_IMAGE_PROTOCOL", proto)
		var buf strings.Builder
		if err := renderImage("x.png", []byte("definitely not a png"), &buf); err != nil {
			t.Fatalf("renderImage with %s: %v", proto, err)
		}
		if got := buf.String(); got != "definitely not a png\n" {
			t.Errorf("renderImage with %s = %q, want passthrough with newline", proto, got)
		}
	}
}
