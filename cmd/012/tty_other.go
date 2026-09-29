//go:build !windows

package main

import "os"

// openTTY opens the terminal 012 was started from, whatever standard
// input and output are.
func openTTY() (in, out *os.File, err error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	return tty, tty, nil
}
