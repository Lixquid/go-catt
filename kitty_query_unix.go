//go:build unix

package main

import (
	"bytes"
	"os"
	"syscall"
	"time"

	"golang.org/x/term"
)

// queryTerminal writes query to the controlling terminal and reads the
// reply until terminator appears or timeout elapses. Using /dev/tty
// keeps the probe from consuming or disturbing the standard streams.
func queryTerminal(query string, terminator byte, timeout time.Duration) ([]byte, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fd := int(f.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	defer term.Restore(fd, oldState)
	if err := syscall.SetNonblock(fd, true); err != nil {
		return nil, err
	}

	if _, err := f.Write([]byte(query)); err != nil {
		return nil, err
	}

	if err := f.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	var resp []byte
	var buf [64]byte
	for {
		n, err := f.Read(buf[:])
		if n > 0 {
			resp = append(resp, buf[:n]...)
			if bytes.IndexByte(resp, terminator) != -1 {
				return resp, nil
			}
		}
		if err != nil {
			return resp, err
		}
	}
}
