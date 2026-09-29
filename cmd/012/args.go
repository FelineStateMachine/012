package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// exitError is an error with the exit status it calls for. A nil err
// exits with code saying nothing, as diff(1) does for "files differ".
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit status %d", e.code)
	}
	return e.err.Error()
}

func (e *exitError) Unwrap() error { return e.err }

// exitCode is the status err asks for, 1 unless it says otherwise, and
// whether there is anything to print.
func exitCode(err error) (code int, show bool) {
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code, ee.err != nil
	}
	return 1, true
}

// usageError is a command used wrongly: status 2, as most tools use for
// usage, with the command's usage line.
func usageError(why, usage string) error {
	switch {
	case usage == "":
		usage = why
	case why != "":
		usage = why + "\n" + usage
	}
	return &exitError{code: 2, err: errors.New(usage)}
}

// cliArgs are a subcommand's arguments: its flags, by name without the
// dashes, and the rest in order.
type cliArgs struct {
	flags map[string]string
	pos   []string
}

func (a cliArgs) has(name string) bool { _, ok := a.flags[name]; return ok }

// parseArgs splits args into flags and the rest. Flags start with --
// and may come anywhere; valued ones take the next argument or =value.
// Only flags the command knows are flags, so an argument such as -5 or
// -A1 is a value; after --, everything is.
func parseArgs(args []string, valued, bools []string) (cliArgs, error) {
	out := cliArgs{flags: map[string]string{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			out.pos = append(out.pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "--") || arg == "--" {
			out.pos = append(out.pos, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg[2:], "=")
		switch {
		case slices.Contains(bools, name):
			if hasValue {
				return out, fmt.Errorf("--%s takes no value", name)
			}
			out.flags[name] = ""
		case slices.Contains(valued, name):
			if !hasValue {
				if i+1 == len(args) {
					return out, fmt.Errorf("--%s needs a value", name)
				}
				i++
				value = args[i]
			}
			out.flags[name] = value
		default:
			return out, fmt.Errorf("unknown flag --%s", name)
		}
	}
	return out, nil
}
