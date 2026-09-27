package config

import (
	"fmt"
	"strings"
)

// DocsMarker divides docs/config.md: the introduction above it is
// written by hand, the reference below it is Reference's output.
const DocsMarker = "<!-- Generated from internal/config/registry.go by `go test ./internal/config -update-docs`. Don't edit below. -->"

// Reference is the option reference in Markdown, one section per group.
func Reference() string {
	var b strings.Builder
	overview(&b)
	for _, g := range Groups {
		fmt.Fprintf(&b, "\n### %s\n", g)
		for _, o := range Options {
			if o.Group != g {
				continue
			}
			fmt.Fprintf(&b, "\n#### `%s`\n\n%s\n\n", o.Name, o.Desc)
			b.WriteString("| | |\n|---|---|\n")
			fmt.Fprintf(&b, "| Type | %s |\n", o.typeDoc())
			def := "(empty)"
			if o.Default != "" {
				def = "`" + o.Default + "`"
			}
			fmt.Fprintf(&b, "| Default | %s |\n", def)
			if len(o.Env) > 0 {
				fmt.Fprintf(&b, "| Environment | `%s` |\n", strings.Join(o.Env, "`, `"))
			}
			if o.Flag != "" {
				fmt.Fprintf(&b, "| Flag | `%s` |\n", o.Flag)
			}
			if o.Repeat {
				b.WriteString("| Repeatable | yes |\n")
			}
			reload := "restart 012"
			if o.Live {
				reload = "File > Settings > Reload config"
			}
			fmt.Fprintf(&b, "| Applies | %s |\n", reload)
		}
	}
	return b.String()
}

func (o *Option) typeDoc() string {
	if o.Kind == Enum {
		return "one of `" + strings.Join(o.Values, "`, `") + "`"
	}
	return o.Kind.String()
}

// overview is a table of every option, its default and where else it's
// set, linking to its section.
func overview(b *strings.Builder) {
	b.WriteString("\n| Option | Default | Environment, flag |\n|---|---|---|\n")
	for _, g := range Groups {
		for _, o := range Options {
			if o.Group != g {
				continue
			}
			def := ""
			if o.Default != "" {
				def = "`" + o.Default + "`"
			}
			var via []string
			for _, e := range o.Env {
				via = append(via, "`"+e+"`")
			}
			if o.Flag != "" {
				via = append(via, "`"+o.Flag+"`")
			}
			fmt.Fprintf(b, "| [`%s`](#%s) | %s | %s |\n", o.Name, o.Name, def, strings.Join(via, ", "))
		}
	}
}
