//go:build !unix && !windows

package main

import (
	"errors"
	"time"
)

// queryTerminal is unsupported on platforms without a terminal
// interface, so support detection always fails.
func queryTerminal(query string, terminator byte, timeout time.Duration) ([]byte, error) {
	return nil, errors.New("catt: terminal query is not supported on this platform")
}
