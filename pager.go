// Pager support: when output goes to a terminal and would be larger
// than the terminal viewport, it is piped through a pager instead.
// The pager is taken from the PAGER environment variable and defaults
// to less. CATT_PAGE=yes forces pagination on (even when piped or when
// the output fits), CATT_PAGE=no forces it off.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

// defaultPager is used when PAGER is unset or empty.
const defaultPager = "less -R"

// pagerArgv returns the pager command and its arguments. PAGER may
// contain arguments (e.g. "less -FX"), so it is split on whitespace.
func pagerArgv() []string {
	fields := strings.Fields(os.Getenv("PAGER"))
	if len(fields) == 0 {
		fields = strings.Fields(defaultPager)
	}
	return fields
}

// pageModeFromEnv interprets CATT_PAGE the same way CATT_COLOR is
// interpreted: yes forces pagination on, no forces it off, and an
// unset or unrecognized value leaves the automatic behaviour.
func pageModeFromEnv() (forceOn, forceOff bool) {
	switch strings.ToLower(os.Getenv("CATT_PAGE")) {
	case "yes", "y", "true", "1":
		return true, false
	case "no", "n", "false", "0":
		return false, true
	}
	return false, false
}

// pagerWriter decides where rendered output goes. Output is buffered
// until it either finishes (and is written straight to the underlying
// writer) or is known to exceed the terminal viewport (and is handed
// to a pager). This keeps memory bounded to roughly one screenful.
type pagerWriter struct {
	w        io.Writer // the real output (usually os.Stdout)
	rows     int       // terminal viewport height, 0 if unknown
	cols     int       // terminal viewport width, 0 if unknown
	disabled bool      // write everything straight to w
	force    bool      // always use the pager, even for short output

	buf  bytes.Buffer // pending output while the decision is open
	row  int          // display rows buffered so far
	col  int          // display columns used by the current line
	cmd  *exec.Cmd    // running pager, nil until started
	pipe io.WriteCloser
}

// newPagerWriter wraps an output writer with the pagination decision.
// Pagination happens only when the output is a terminal and the
// viewport size is known, unless CATT_PAGE forces it on.
func newPagerWriter(w io.Writer) *pagerWriter {
	pw := &pagerWriter{w: w}

	forceOn, forceOff := pageModeFromEnv()
	pw.force = forceOn

	if forceOff {
		pw.disabled = true
		return pw
	}

	f, ok := w.(*os.File)
	if !ok || !isatty.IsTerminal(f.Fd()) {
		if forceOn {
			return pw // still page, into the pager's own full screen
		}
		pw.disabled = true
		return pw
	}

	rows, cols, err := term.GetSize(int(f.Fd()))
	if err != nil || rows <= 0 || cols <= 0 {
		if forceOn {
			return pw
		}
		pw.disabled = true
		return pw
	}
	pw.rows, pw.cols = rows, cols
	return pw
}

// Write implements io.Writer.
func (pw *pagerWriter) Write(p []byte) (int, error) {
	if pw.cmd != nil {
		n, err := pw.pipe.Write(p)
		if err != nil && errors.Is(err, syscall.EPIPE) {
			// The user quit the pager early; drop the rest quietly.
			return n, nil
		}
		return n, err
	}
	if pw.disabled {
		return pw.w.Write(p)
	}

	pw.buf.Write(p)
	pw.countRows(p)
	if pw.force || pw.row > pw.rows {
		if err := pw.startPager(); err != nil {
			return 0, err
		}
		_, err := pw.pipe.Write(pw.buf.Bytes())
		pw.buf.Reset()
		return len(p), err
	}
	return len(p), nil
}

// startPager launches the pager and re-buffers the pending output into
// it. Nothing has been written to the real output yet, so the switch is
// invisible to the terminal.
func (pw *pagerWriter) startPager() error {
	argv := pagerArgv()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout = pw.w
	cmd.Stderr = os.Stderr
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("could not create pipe to %s: %w", argv[0], err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start pager %s: %w", argv[0], err)
	}
	pw.cmd, pw.pipe = cmd, pipe
	return nil
}

// countRows updates the buffered display-row estimate for p, taking
// line wrapping at the terminal width into account. ANSI styling is
// measured with ansi.StringWidth so escape sequences do not count as
// visible columns.
func (pw *pagerWriter) countRows(p []byte) {
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			pw.col += ansi.StringWidth(string(p))
			return
		}
		pw.col += ansi.StringWidth(string(p[:i]))
		pw.row += pw.wrappedRows(pw.col)
		pw.col = 0
		p = p[i+1:]
	}
}

// wrappedRows returns the number of terminal rows a line of the given
// display width occupies: one for an empty line, otherwise the line
// width divided by the terminal width, rounded up.
func (pw *pagerWriter) wrappedRows(width int) int {
	if width == 0 || pw.cols <= 0 {
		return 1
	}
	return (width + pw.cols - 1) / pw.cols
}

// reset drops the buffered output of one file so the next file makes
// its own pagination decision. If the pager is already running, its
// session continues and the call does nothing.
func (pw *pagerWriter) reset() {
	if pw.cmd != nil {
		return
	}
	pw.buf.Reset()
	pw.row, pw.col = 0, 0
}

// finish flushes any buffered output (or waits for the pager to exit).
func (pw *pagerWriter) finish() error {
	if pw.cmd != nil {
		closeErr := pw.pipe.Close()
		waitErr := pw.cmd.Wait()
		if closeErr != nil && errors.Is(closeErr, syscall.EPIPE) {
			closeErr = nil
		}
		if waitErr != nil {
			return fmt.Errorf("pager failed: %w", waitErr)
		}
		return closeErr
	}
	if pw.disabled {
		return nil
	}
	_, err := pw.w.Write(pw.buf.Bytes())
	pw.buf.Reset()
	return err
}

// raw returns the underlying writer, bypassing the pager. It is used
// for sixel image output, whose escape sequences have no meaningful
// line count and which manages the terminal itself.
func (pw *pagerWriter) raw() io.Writer {
	return pw.w
}

// unwrapPager returns the writer under a pagerWriter, so output that
// must not be paged (sixel images) can reach the terminal directly.
func unwrapPager(w io.Writer) io.Writer {
	if pw, ok := w.(*pagerWriter); ok {
		return pw.raw()
	}
	return w
}
