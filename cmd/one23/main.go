// Command one23 is a Lotus 1-2-3 style spreadsheet for the terminal.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	tea "charm.land/bubbletea/v2"

	"one23/internal/sheet"
	"one23/internal/ui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "one23:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: one23 [file" + sheet.FileExt + "]")
	}
	s, name := sheet.New(), ""
	if len(args) == 1 {
		name = args[0]
		f, err := os.Open(name)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			// Start a new worksheet that will be saved under this name.
		case err != nil:
			return err
		default:
			defer f.Close()
			if s, err = sheet.Read(f); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	_, err := tea.NewProgram(ui.New(s, name)).Run()
	return err
}
