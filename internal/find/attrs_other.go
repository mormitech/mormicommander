//go:build !windows && !darwin

// This file covers Linux and the BSDs for the attribute filter. They have no hidden
// or system attribute, so hidden means "dotfile" (handled in attrs.go) and the
// read-only and executable bits come straight from the permission bits.

package find

import "io/fs"

// sysAttrs returns what the permission bits say.
func sysAttrs(name string, info fs.FileInfo) Attr { return modeAttrs(info) }
