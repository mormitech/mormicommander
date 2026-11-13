// On Windows the command lines we build follow the Windows rules: double
// quotes only, backslash as the path separator, drive letters and UNC
// shares. This file only picks that style as the default; the rules
// themselves live in quote.go and paths.go so they can be tested anywhere.
package params

// DefaultStyle is the quoting and path style of the operating system the
// program was built for.
const DefaultStyle = StyleWindows
