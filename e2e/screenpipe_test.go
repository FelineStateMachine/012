package e2e

// Golden screens for 012 in a pipeline: the table read from standard
// input with the status line saying what quitting sends, and the
// question Quit asks, each on a light terminal too.
var pipeScreens = []screen{
	{name: "pipe-status", args: []string{"--pipe"}, opts: options{piped: true, stdin: lsNUON, env: []string{"TZ=America/Denver"}}, setup: func(s *session) {
		s.waitFor("Ctrl+Q  send the sheet as NUON")
	}},
	{name: "pipe-quit", args: []string{"--pipe"}, opts: options{piped: true, stdin: lsNUON, env: []string{"TZ=America/Denver"}}, setup: func(s *session) {
		s.waitFor("4.2 kB")
		s.keys("<shift+down>", "<shift+down>", "<shift+right>", "<ctrl+q>")
		s.waitFor("Send A1:B3 as NUON?")
	}},
}

func init() {
	for _, sc := range pipeScreens {
		light := sc
		light.name += "-light"
		light.opts.light = true
		screens = append(screens, sc, light)
	}
}
