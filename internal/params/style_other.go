//go:build !windows

// On macOS and Linux the command lines we build follow POSIX shell rules:
// single and double quotes, backslash escapes and "/" as the path separator.
// This file only picks that style as the default; the rules themselves live
// in quote.go and paths.go so they can be tested on any machine.
package params

// DefaultStyle is the quoting and path style of the operating system the
// program was built for.
const DefaultStyle = StylePOSIX
