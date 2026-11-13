// This file sets the default case rule for macOS and any non-Unix system other than Windows. APFS and HFS+ are
// case-insensitive by default, so "readme.*" must also select "README.md", exactly as Finder would find it.

//go:build !windows && (!unix || darwin)

package masks

// DefaultCaseSensitive is the case rule CaseDefault resolves to on this platform.
const DefaultCaseSensitive = false

// lineEnding ends each line of a saved selection file.
const lineEnding = "\n"
