//go:build crashtest

package ui

import (
	"os"

	tea "charm.land/bubbletea/v2"
)

// In builds tagged crashtest (the e2e tests' binary), F12 makes the
// program panic where O12_CRASH_TEST says: "update", "view" or
// "command". Release builds have no such key.
func init() {
	where := os.Getenv("O12_CRASH_TEST")
	crashTest = func(msg tea.Msg) string {
		if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "f12" {
			return where
		}
		return ""
	}
}
