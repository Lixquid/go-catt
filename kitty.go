// Kitty graphics protocol support. The terminal is probed with the
// protocol's documented query, and images are sent as base64-encoded
// RGBA pixel data split into escape sequences no larger than the
// protocol allows.
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

// kittyQuery asks a terminal whether it speaks the kitty graphics
// protocol: a 1x1 RGB image with the query action. It is followed by a
// Primary Device Attributes request so a terminal that does not answer
// the query still produces a reply to read.
const kittyQuery = "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\\x1b[c"

// kittyQueryTimeout bounds how long detection waits for the terminal.
const kittyQueryTimeout = time.Second

// kittyChunkSize is the largest base64 payload carried by a single
// graphics escape sequence. The protocol wants chunks to be a multiple
// of four bytes, which 4096 is.
const kittyChunkSize = 4096

// kitty support probing talks to the terminal, so it is done at most
// once per process and the answer is cached.
var (
	kittyOnce   sync.Once
	kittyUsable bool
)

// useKitty reports whether image files should be drawn with the kitty
// graphics protocol. It requires stdout to be a terminal and the
// terminal to answer the protocol's support query.
func useKitty() bool {
	kittyOnce.Do(func() {
		if !isatty.IsTerminal(os.Stdout.Fd()) {
			return
		}
		kittyUsable = kittySupported()
	})
	return kittyUsable
}

// kittySupported reports whether the controlling terminal answered the
// kitty graphics protocol support query. A terminal that understands
// the protocol replies with an APC containing an OK response.
func kittySupported() bool {
	resp, err := queryTerminal(kittyQuery, 'c', kittyQueryTimeout)
	if err != nil {
		return false
	}
	return bytes.Contains(resp, []byte("_G")) && bytes.Contains(resp, []byte(";OK"))
}

// renderImageAsKitty scales img to the terminal and emits it using the
// kitty graphics protocol followed by a newline.
func renderImageAsKitty(img image.Image, w io.Writer) error {
	width, height := kittyScaledSize(img.Bounds().Dx(), img.Bounds().Dy())
	out := scaleImage(img, width, height)
	return encodeKitty(out, w)
}

// kittyScaledSize returns the size to render an image at under the
// kitty protocol: its native size, shrunk to fit the terminal's width
// in pixels. Kitty pixels are square, so unlike sixels no width
// doubling is applied.
func kittyScaledSize(srcW, srcH int) (width, height int) {
	width, height = srcW, srcH
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

// encodeKitty writes img as straight RGBA pixel data using the kitty
// graphics protocol. The base64 payload is split across as many escape
// sequences as needed; only the first carries the image metadata.
func encodeKitty(img *image.NRGBA, w io.Writer) error {
	width, height := img.Bounds().Dx(), img.Bounds().Dy()

	// Flatten to the tightly packed, row-major RGBA bytes the protocol
	// expects, since in-memory NRGBA rows may be padded.
	raw := make([]byte, 0, width*height*4)
	for y := 0; y < height; y++ {
		start := y * img.Stride
		raw = append(raw, img.Pix[start:start+width*4]...)
	}
	encoded := base64.RawStdEncoding.EncodeToString(raw)

	for i := 0; i < len(encoded); i += kittyChunkSize {
		end := i + kittyChunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		more := 1
		if end == len(encoded) {
			more = 0
		}

		// Only the first chunk carries the image metadata; later
		// chunks just mark whether another follows.
		control := "m=" + strconv.Itoa(more)
		if i == 0 {
			// a=T transmits and displays the image, f=32 is RGBA,
			// and q=2 suppresses any acknowledgement from the
			// terminal so it does not pollute the output.
			control = fmt.Sprintf("a=T,f=32,s=%d,v=%d,q=2,", width, height) + control
		}
		if _, err := fmt.Fprintf(w, "\x1b_G%s;%s\x1b\\", control, encoded[i:end]); err != nil {
			return err
		}
	}

	_, err := fmt.Fprint(w, "\r\n")
	return err
}
