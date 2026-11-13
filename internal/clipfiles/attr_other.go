//go:build !windows

// This file holds the non-Windows forms of two small platform hooks used by the text formatters. Outside Windows the
// attribute column of cm_CopyDetailsToClip shows the Unix permission string (for example "-rw-r--r--"), because that
// is what the panel shows there, and network names need no translation: a mounted share already has a normal path.

package clipfiles

import "io/fs"

// attrString returns the attribute text for the details column.
func attrString(fi fs.FileInfo) string { return fi.Mode().String() }

// uncName returns the network name for path; outside Windows that is the path itself.
func uncName(path string) string { return path }
