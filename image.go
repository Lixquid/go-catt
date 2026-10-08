// Image rendering: display images as kitty graphics or sixel graphics
// on terminals that support them, falling back to a plain passthrough
// like cat otherwise. PNG, JPEG, and GIF files are recognized; animated
// GIFs are rendered as their first frame, since graphics output is
// static.
package main

import (
	"bytes"
	"fmt"
	"image"
	// Register the decoders used by image.Decode.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mattn/go-isatty"
	"github.com/mattn/go-sixel"
	"golang.org/x/term"
)

var imageExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
}

func isImage(name string) bool {
	return imageExtensions[strings.ToLower(filepath.Ext(name))]
}

// Sixel support probing talks to the terminal, so it is done at most
// once per process and the answer is cached.
var (
	sixelOnce   sync.Once
	sixelUsable bool
)

// useSixel reports whether image files should be rendered as sixel
// graphics. It requires stdout to be a terminal; the terminal must then
// advertise sixel support in its Device Attributes reply.
func useSixel() bool {
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

// imageProtocol selects the graphics format used to draw image files.
type imageProtocol int

const (
	// protocolAuto detects kitty graphics first, then falls back to
	// sixel. This is the default.
	protocolAuto imageProtocol = iota
	// protocolKitty forces the kitty graphics protocol.
	protocolKitty
	// protocolSixel forces sixel graphics.
	protocolSixel
	// protocolNone passes image bytes through untouched, like cat.
	protocolNone
)

// configuredImageProtocol returns the image protocol requested through
// the environment. CATT_IMAGE_PROTOCOL accepts auto, kitty, sixel, or
// none; CATT_KITTY=yes is accepted as a shorthand that forces the kitty
// protocol. Anything unrecognized leaves the choice to detection.
func configuredImageProtocol() imageProtocol {
	for _, name := range []string{"CATT_IMAGE_PROTOCOL", "CATT_IMAGE", "CATT_GRAPHICS"} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
		case "kitty":
			return protocolKitty
		case "sixel":
			return protocolSixel
		case "none", "off", "no", "raw":
			return protocolNone
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CATT_KITTY"))) {
	case "yes", "y", "true", "1", "on", "force", "kitty":
		return protocolKitty
	}
	return protocolAuto
}

// kittyDisabled reports whether CATT_KITTY=no was set, in which case
// automatic detection skips the kitty protocol and may still use sixel.
func kittyDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CATT_KITTY"))) {
	case "no", "n", "false", "0", "off":
		return true
	}
	return false
}

// renderImage decodes an image file and displays it using the selected
// graphics protocol. When no terminal graphics are available (or the
// file is not a valid image), the raw bytes pass through untouched,
// just like cat.
//
// Sixel pixels are roughly half as wide as they are tall, so sixel
// images are doubled in width to keep their aspect ratio on screen.
// Kitty pixels are square, so they are drawn at their native size. Both
// are capped to the terminal's width in pixels so wide images shrink to
// fit, and the cursor is moved to a fresh line afterwards.
func renderImage(filename string, data []byte, w io.Writer) error {
	proto := configuredImageProtocol()
	if proto == protocolNone {
		return passthrough(w, data)
	}

	// Automatic rendering only applies to styled terminal output, so
	// CATT_COLOR=no forces a plain passthrough. An explicit protocol
	// choice is always attempted.
	if proto == protocolAuto && !useDarkStyle() {
		return passthrough(w, data)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return passthrough(w, data)
	}

	switch proto {
	case protocolKitty:
		return renderImageAsKitty(img, w)
	case protocolSixel:
		return renderImageAsSixel(img, w)
	default:
		if !kittyDisabled() && useKitty() {
			return renderImageAsKitty(img, w)
		}
		if useSixel() {
			return renderImageAsSixel(img, w)
		}
		return passthrough(w, data)
	}
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
// pixels, or fallbackSixelWidth when it cannot be determined. It combines the cell size
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
