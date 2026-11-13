// This file checks whether a proposed new name is acceptable to the target
// operating system. Total Commander refuses to start a rename that would
// fail halfway, and so do we: the preview marks every bad name and Execute
// will not run while any are left. Windows has the longest list of rules
// (the characters < > : " / \ | ? * and control characters are forbidden,
// device names like CON or LPT1 are reserved even with an extension, and a
// name may not end in a dot or space). macOS forbids '/' and, because the
// Finder shows ':' as '/', we reject ':' too; Linux and the other Unix
// systems only forbid '/' and the NUL byte. All systems reject empty names,
// "." and "..", and names longer than 255 bytes (255 UTF-16 units on
// Windows). The target system is a parameter so the checks can be tested on
// any machine and so a UI renaming files on a network share could pick a
// stricter set.

package multirename

import (
	"fmt"
	"strings"
	"unicode/utf16"
)

// ProblemKind classifies why an entry cannot be renamed.
type ProblemKind string

const (
	ProblemEmpty        ProblemKind = "empty"         // the new name is empty
	ProblemDotName      ProblemKind = "dot-name"      // "." or ".."
	ProblemInvalidChars ProblemKind = "invalid-chars" // contains forbidden characters
	ProblemReserved     ProblemKind = "reserved"      // a reserved device name on Windows
	ProblemTrailing     ProblemKind = "trailing"      // ends in a dot or space on Windows
	ProblemTooLong      ProblemKind = "too-long"      // longer than the system allows
	ProblemDuplicate    ProblemKind = "duplicate"     // two entries get the same target
	ProblemExists       ProblemKind = "exists"        // a different file already has the name
)

// Problem is one reason an entry cannot be renamed, with a human-readable
// detail for the dialog's tooltip.
type Problem struct {
	Kind   ProblemKind `json:"kind"`
	Detail string      `json:"detail"`
}

// MaxNameLen is the longest file name component allowed by the common file
// systems, in bytes (or UTF-16 code units on Windows).
const MaxNameLen = 255

var windowsReserved = func() map[string]bool {
	m := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}
	for i := 1; i <= 9; i++ {
		m[fmt.Sprintf("COM%d", i)] = true
		m[fmt.Sprintf("LPT%d", i)] = true
	}
	return m
}()

// ValidateName returns the problems the name would have on the operating
// system goos (a runtime.GOOS value). A nil result means the name is fine.
func ValidateName(name, goos string) []Problem {
	var out []Problem
	if name == "" {
		return []Problem{{ProblemEmpty, "the new name is empty"}}
	}
	if name == "." || name == ".." {
		return []Problem{{ProblemDotName, fmt.Sprintf("%q cannot be used as a name", name)}}
	}
	var bad string
	switch goos {
	case "windows":
		bad = `<>:"/\|?*`
	case "darwin", "ios":
		bad = "/:"
	default:
		bad = "/"
	}
	var found []string
	seen := map[rune]bool{}
	for _, r := range name {
		forbidden := r == 0 || strings.ContainsRune(bad, r) || (goos == "windows" && r < 32)
		if forbidden && !seen[r] {
			seen[r] = true
			if r < 32 {
				found = append(found, fmt.Sprintf("U+%04X", r))
			} else {
				found = append(found, string(r))
			}
		}
	}
	if len(found) > 0 {
		out = append(out, Problem{ProblemInvalidChars, "not allowed here: " + strings.Join(found, " ")})
	}
	if goos == "windows" {
		stem := name
		if i := strings.IndexByte(stem, '.'); i >= 0 {
			stem = stem[:i]
		}
		stem = strings.ToUpper(strings.TrimRight(stem, " "))
		if windowsReserved[stem] {
			out = append(out, Problem{ProblemReserved, stem + " is a reserved device name on Windows"})
		}
		if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
			out = append(out, Problem{ProblemTrailing, "Windows names cannot end with a dot or a space"})
		}
		if len(utf16.Encode([]rune(name))) > MaxNameLen {
			out = append(out, Problem{ProblemTooLong, fmt.Sprintf("longer than %d characters", MaxNameLen)})
		}
	} else if len(name) > MaxNameLen {
		out = append(out, Problem{ProblemTooLong, fmt.Sprintf("longer than %d bytes", MaxNameLen)})
	}
	return out
}
