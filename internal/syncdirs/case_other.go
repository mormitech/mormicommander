//go:build !windows && !darwin

// On Linux and the other Unix systems file names are case-sensitive, so
// CaseAuto treats "A.txt" and "a.txt" as two different files.
package syncdirs

const defaultCaseInsensitive = false
