//go:build !windows

// On macOS and Linux paths are compared exactly (apart from a trailing
// slash): Linux file systems are case-sensitive, and although the default
// macOS volume is not, treating "Foo" and "foo" as two history entries is
// harmless while merging two real folders on a case-sensitive volume would be
// wrong. The home folder is written back as $HOME.

package config

import "strings"

const homeVarSpelling = "$HOME"

func pathsEqual(a, b string) bool {
	trim := func(s string) string {
		if len(s) > 1 {
			return strings.TrimRight(s, "/")
		}
		return s
	}
	return trim(a) == trim(b)
}
