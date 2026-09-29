package e2e

import (
	"path/filepath"
	"testing"
)

// Golden screens for following files: a log followed in its own sheet,
// the tab's live mark and a formula over the rows, the question of how
// many rows to keep, and a linked file that isn't there; the log and the
// missing file on a light terminal too.

// appLog is a web server's log as a CSV file.
const appLog = `time,level,path,ms
09:14:02,info,/,12
09:14:03,info,/login,48
09:14:05,warn,/search,310
09:14:07,info,/cart,22
09:14:08,error,/pay,1204
09:14:10,info,/,9
09:14:11,info,/orders,57
09:14:13,warn,/search,288
09:14:14,info,/login,41
09:14:16,info,/cart,19
09:14:17,info,/,11
`

func writeAppLog(t *testing.T, dir string) { writeText(t, filepath.Join(dir, "app.csv"), appLog) }

// followLog follows app.csv in a new sheet, keeping its last 8 rows, and
// adds up their times beside it.
func followLog(s *session) {
	s.keys("Requests", "<enter>")
	s.keys("<ctrl+k>", "Import", "<enter>")
	s.waitFor("Type to filter, or a path")
	s.keys("app.csv", "<enter>")
	s.waitFor("Follow the file")
	s.keys("follow", "<enter>")
	s.waitFor("Follow app.csv:")
	s.keys("l")
	s.waitFor("Keep the last how many rows:")
	s.keys("8", "<enter>")
	s.waitFor("/orders")
	s.keys("<f5>", "F1", "<enter>", "Slowest", "<tab>", "=MAX(D:D)", "<enter>")
	s.keys("<f5>", "F2", "<enter>", "Errors", "<tab>", `=COUNTIF(B:B,"error")`, "<enter>")
	s.waitFor("1204")
}

var followScreens = []screen{
	{name: "follow-log", files: writeAppLog, setup: followLog},
	{name: "follow-ask", files: writeAppLog, setup: func(s *session) { s.linkTableAsk("app.csv") }},
	{name: "follow-missing", files: func(t *testing.T, dir string) {
		writeText(t, filepath.Join(dir, "gone.012"),
			`{"version": 2, "cells": {"D1": "Nightly run"}, "regions": [{"name": "nightly", "at": "A1", "path": "nightly.csv"}]}`)
	}, setup: func(s *session) {
		s.keys("<ctrl+o>", "gone", "<enter>")
		s.waitFor("#REF!")
		s.waitFor("! nightly.csv: the file isn't there")
	}},
}

// linkTableAsk links name at the active cell through the palette, up to
// the question of how many rows to keep.
func (s *session) linkTableAsk(name string) {
	s.t.Helper()
	s.keys("<ctrl+k>", "Link a table")
	s.waitFor("Link a table")
	s.keys("<enter>")
	s.waitFor("Type to filter, or a path")
	s.keys(name, "<enter>")
	s.waitFor("Follow " + name + ":")
}

func init() {
	for _, sc := range followScreens {
		screens = append(screens, sc)
		if sc.name != "follow-ask" {
			light := sc
			light.name += "-light"
			light.opts.light = true
			screens = append(screens, light)
		}
	}
}
