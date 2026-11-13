// Windows file names are case-insensitive and both slash kinds separate
// folders, so two configured paths are the same folder when they differ only
// in case or in the slash used. Histories use this to avoid duplicates, and
// Contract uses it to recognise the program and profile folders. The profile
// folder is written back as %USERPROFILE%, the variable Windows users know.

package config

import "strings"

const homeVarSpelling = "%USERPROFILE%"

func pathsEqual(a, b string) bool {
	norm := func(s string) string {
		return strings.TrimRight(strings.ReplaceAll(s, "/", `\`), `\`)
	}
	return strings.EqualFold(norm(a), norm(b))
}
