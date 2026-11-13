// This file says that names on macOS are compared without regard to case by
// default. APFS and HFS+ volumes are case-insensitive unless the user chose
// the rare case-sensitive format, so treating "a.txt" and "A.txt" as the same
// target is the safe choice; callers that know a volume is case-sensitive can
// override it with CaseRuleSensitive.

package multirename

const defaultFoldCase = true
