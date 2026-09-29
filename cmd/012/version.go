package main

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/FelineStateMachine/012/internal/headless"
)

// version is the release this binary was built as, set by make dist
// with -ldflags "-X main.version=v1.2.3". Empty in other builds.
var version string

// buildVersion is what 012 version prints: the stamped release, else the
// module version go install records (v1.2.3, or (devel) in a checkout),
// with the VCS revision when the build has one.
func buildVersion() string {
	v, rev := version, ""
	if info, ok := debug.ReadBuildInfo(); ok {
		if v == "" {
			v = info.Main.Version
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				rev = s.Value
			}
		}
	}
	if v == "" {
		v = "(devel)"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if rev != "" {
		v += " (" + rev + ")"
	}
	return v
}

// versionResult is what 012 version --format json writes.
type versionResult struct {
	Version string `json:"version"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func runVersion(args []string, e env) error {
	a, err := parseArgs(args, []string{"format"}, nil)
	if err != nil || len(a.pos) > 0 {
		return usageError("", "usage: 012 version [--format text|json|nuon]")
	}
	format, err := resultFormat(a)
	if err != nil {
		return usageError(err.Error(), "usage: 012 version [--format text|json|nuon]")
	}
	if format != "text" {
		return headless.Encode(e.stdout, format, versionResult{buildVersion(), runtime.Version(), runtime.GOOS, runtime.GOARCH})
	}
	_, err = fmt.Fprintf(e.stdout, "012 %s %s %s/%s\n", buildVersion(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return err
}
