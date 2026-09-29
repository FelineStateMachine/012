// Package module holds 012.nu, the nushell module that gives 012 the
// command `sheet`, so nushell users needn't type `^012` (a bare 012 is a
// number to nushell). `012 nu --module` prints it and `012 nu
// --install-module` installs it (see docs/nushell/README.md).
package module

import _ "embed"

// Source is the text of 012.nu.
//
//go:embed 012.nu
var Source string

// Name is the module's file name, and so its name in nushell.
const Name = "012.nu"
