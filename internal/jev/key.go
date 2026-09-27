package jev

import "github.com/FelineStateMachine/012/internal/sheet"

// Key identifies a question by its content; see sheet.RemoteCall.Key.
func Key(c sheet.RemoteCall) string { return c.Key() }
