package main

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestScaledSize(t *testing.T) {
	// Without a terminal to query, the width is capped at
	// fallbackSixelWidth. Sixel pixels are about half as wide as tall,
	// so images are doubled in width first.
	tests := []struct {
		srcW, srcH   int
		wantW, wantH int
	}{
		// Small images are only doubled.
		{100, 50, 200, 100},
		// Exactly at the cap: doubled width equals the fallback.
		{1000, 500, 2000, 1000},
		// Wide images are shrunk to fit the cap.
		{1500, 1000, 2000, 2000 * 2000 / 3000},
	}
	for _, tt := range tests {
		w, h := scaledSize(tt.srcW, tt.srcH)
		if w != tt.wantW || h != tt.wantH {
			t.Errorf("scaledSize(%d, %d) = (%d, %d), want (%d, %d)",
				tt.srcW, tt.srcH, w, h, tt.wantW, tt.wantH)
		}
	}
}

func TestScaleImageSampling(t *testing.T) {
	// Nearest-neighbor sampling: each source pixel of a 2x2 image
	// occupies a 2x2 block of the 4x4 output.
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{A: 255})

	out := scaleImage(img, 4, 4)
	// NRGBA.At returns color.NRGBA values.
	for _, c := range []struct {
		x, y  int
		want  color.NRGBA
		label string
	}{
		{0, 0, color.NRGBA{R: 255, A: 255}, "red"},
		{3, 0, color.NRGBA{G: 255, A: 255}, "green"},
		{0, 3, color.NRGBA{B: 255, A: 255}, "blue"},
		{3, 3, color.NRGBA{A: 255}, "opaque"},
	} {
		if got := out.NRGBAAt(c.x, c.y); got != c.want {
			t.Errorf("out(%d,%d) = %v, want %s", c.x, c.y, got, c.label)
		}
	}
}

func TestRenderImagePassthrough(t *testing.T) {
	t.Setenv("CATT_COLOR", "no")
	// With styling off, auto protocol detection is disabled and the
	// raw bytes pass through untouched, like cat. Invalid image data
	// passes through even when graphics are enabled.
	tests := []string{"no", "yes"}
	for _, color := range tests {
		t.Setenv("CATT_COLOR", color)
		data := []byte("definitely not a png")
		var buf strings.Builder
		if err := renderImage("x.png", data, &buf); err != nil {
			t.Fatalf("renderImage with CATT_COLOR=%s: %v", color, err)
		}
		if got := buf.String(); got != "definitely not a png\n" {
			t.Errorf("renderImage with CATT_COLOR=%s = %q, want passthrough with newline", color, got)
		}
	}
}

func TestPassthrough(t *testing.T) {
	tests := []struct {
		data string
		want string
	}{
		{"already ends\n", "already ends\n"},
		{"no newline", "no newline\n"},
		{"", ""},
	}
	for _, tt := range tests {
		var buf strings.Builder
		if err := passthrough(&buf, []byte(tt.data)); err != nil {
			t.Fatal(err)
		}
		if got := buf.String(); got != tt.want {
			t.Errorf("passthrough(%q) = %q, want %q", tt.data, got, tt.want)
		}
	}
}
