// This file says that names on Linux and the other Unix systems are compared
// exactly. Their native file systems are case-sensitive, so "a.txt" and
// "A.txt" are two different files and may both be targets of one rename.

//go:build !windows && !darwin

package multirename

const defaultFoldCase = false
