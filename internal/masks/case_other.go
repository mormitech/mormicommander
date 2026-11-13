// This file sets the defaults for macOS, Linux and every other system except Windows. Masks compare letters
// without regard to case on all platforms, even on case-sensitive Linux file systems, because Total Commander users
// type "*.JPG" and expect it to select "photo.jpg" (spec area-3, mask syntax rule 6). Callers that really need exact
// letters ask for CaseSensitive explicitly. Saved selection files use plain LF line endings here.

//go:build !windows

package masks

// DefaultCaseSensitive is the case rule CaseDefault resolves to on this platform.
const DefaultCaseSensitive = false

// lineEnding ends each line of a saved selection file.
const lineEnding = "\n"
