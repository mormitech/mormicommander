//go:build !windows && !darwin

// Linux and other Unix details of the comparison. Their usual file systems keep case, so CaseAuto matches names
// exactly and "a.txt" and "A.txt" are two files. A file is hidden when its name starts with a dot, the only hiding
// convention these systems have.
package dircompare

import "io/fs"

const defaultCaseInsensitive = false

func isHidden(name string, _ fs.FileInfo) bool {
	return len(name) > 1 && name[0] == '.'
}
