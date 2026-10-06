// PNG rendering: display images as sixel graphics on terminals that
// support them, falling back to a plain passthrough like cat otherwise.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/mattn/go-isatty"
	"github.com/mattn/go-sixel"
	"golang.org/x/term"
)

var pngExtensions = map[string]bool{
	".png": true,
}

func isPNG(name string) bool {
	return pngExtensions[strings.ToLower(name[len(name)-len(".png"):])]
}

// Sixel support probing talks to the terminal, so it is done at most
// once per process and the answer is cached.
var (
	sixelOnce   sync.Once
	sixelUsable bool
)

// useSixel reports whether PNG files should be rendered as sixel
// graphics. It requires stdout to be a terminal; the terminal must then
// advertise sixel support in its Device Attributes reply. The check is
// disabled entirely by CATT_COLOR=no.
func useSixel() bool {
	if !useDarkStyle() {
		return false
	}
	sixelOnce.Do(func() {
		if !isatty.IsTerminal(os.Stdout.Fd()) {
			return
		}
		sixelUsable = sixel.IsSupported()
	})
	return sixelUsable
}

// fallbackSixelWidth is the upper bound, in pixels, for rendered images
// when the terminal's width in pixels is unknown.
const fallbackSixelWidth = 2000

// renderPNG decodes a PNG file and displays it as a sixel graphic. When
// the terminal does not support sixels (or the file is not a valid
// PNG), the raw bytes pass through untouched, just like cat.
//
// Sixel pixels are roughly half as wide as they are tall, so the image
// is doubled in width to keep its aspect ratio on screen. The result is
// capped to the terminal's width in pixels so wide images shrink to
// fit, and the cursor is moved to a fresh line afterwards.
func renderPNG(filename string, data []byte, w io.Writer) error {
	if useSixel() {
		img, err := png.Decode(bytes.NewReader(data))
		if err == nil {
			return renderImageAsSixel(img, w)
		}
	}
	return passthrough(w, data)
}

// renderImageAsSixel scales img to the terminal and emits it as a sixel
// sequence followed by a newline.
func renderImageAsSixel(img image.Image, w io.Writer) error {
	width, height := scaledSize(img.Bounds().Dx(), img.Bounds().Dy())
	out := scaleImage(img, width, height)

	enc := sixel.NewEncoder(w)
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("failed to encode sixel image: %w", err)
	}
	if _, err := fmt.Fprint(w, "\r\n"); err != nil {
		return err
	}
	return nil
}

// scaledSize returns the size to render an image at: twice as wide as
// tall (compensating for sixel aspect ratio), then shrunk if needed so
// it fits the terminal's width in pixels.
func scaledSize(srcW, srcH int) (width, height int) {
	width, height = srcW*2, srcH*2
	if maxW := terminalPixelWidth(); maxW > 0 && width > maxW {
		height = height * maxW / width
		width = maxW
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return width, height
}

// terminalPixelWidth returns the usable width of the terminal in device
// pixels, or 0 when it cannot be determined. It combines the cell size
// reported by the terminal with the window size in characters, so
// margins are respected where the terminal supports it.
func terminalPixelWidth() int {
	cw, _, err := sixel.CellSize()
	if err != nil || cw <= 0 {
		return fallbackSixelWidth
	}
	cols, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || cols <= 0 {
		return fallbackSixelWidth
	}
	return cw * cols
}

// scaleImage resizes img to width x height with nearest-neighbor
// sampling, producing an NRGBA image as preferred by the sixel encoder.
func scaleImage(img image.Image, width, height int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	src := img.Bounds()
	srcW, srcH := src.Dx(), src.Dy()
	for y := 0; y < height; y++ {
		sy := src.Min.Y + y*srcH/height
		for x := 0; x < width; x++ {
			sx := src.Min.X + x*srcW/width
			out.Set(x, y, img.At(sx, sy))
		}
	}
	return out
}

// passthrough writes raw file bytes, ending with a newline like cat.
func passthrough(w io.Writer, data []byte) error {
	if _, err := w.Write(data); err != nil {
		return err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}
