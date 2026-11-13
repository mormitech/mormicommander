package fileops

// Names in the copy dialog work like Total Commander's: the target field may hold a folder, a folder plus a new name
// ("x/b.txt"), or a folder plus a mask ("x/*.bak") that renames every copy; the "Only files of this type" field takes
// patterns with "|" exclusions; and a copy that would land on an existing name can be given a free one, "file(2).txt".
// These helpers are pure functions so the UI can use them for previews too.

import (
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// Filter is a parsed "Only files of this type" field. The zero value takes everything.
type Filter struct {
	Include    []string // lower-case patterns; empty means all
	Exclude    []string // lower-case patterns for files (and folders)
	ExcludeDir []string // lower-case patterns for folders only (written with a trailing / or \)
}

// ParseFilter reads "*.txt;*.md|old*;vendor/". Patterns are separated by ";" or spaces; a pattern with spaces in it can
// be written in double quotes. Matching ignores case, like TC. "*.*" means every file, with or without an extension.
func ParseFilter(s string) Filter {
	var f Filter
	inc, exc, _ := strings.Cut(s, "|")
	f.Include = splitPatterns(inc)
	for _, p := range splitPatterns(exc) {
		if strings.HasSuffix(p, "/") || strings.HasSuffix(p, `\`) {
			f.ExcludeDir = append(f.ExcludeDir, strings.TrimRight(p, `/\`))
		} else {
			f.Exclude = append(f.Exclude, p)
		}
	}
	return f
}

func splitPatterns(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	flush := func() {
		if p := strings.TrimSpace(cur.String()); p != "" {
			if p == "*.*" {
				p = "*"
			}
			out = append(out, strings.ToLower(p))
		}
		cur.Reset()
	}
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case !quoted && (r == ';' || r == ' ' || r == '\t'):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// Empty says whether the filter takes everything.
func (f Filter) Empty() bool { return len(f.Include)+len(f.Exclude)+len(f.ExcludeDir) == 0 }

// MatchFile says whether a file (or link) with this name is taken.
func (f Filter) MatchFile(name string) bool {
	n := strings.ToLower(name)
	if matchAny(f.Exclude, n) {
		return false
	}
	return len(f.Include) == 0 || matchAny(f.Include, n)
}

// MatchDir says whether the walk goes into a folder with this name. Include patterns don't apply to folders: TC looks
// for matching files in every folder.
func (f Filter) MatchDir(name string) bool {
	n := strings.ToLower(name)
	return !matchAny(f.ExcludeDir, n) && !matchAny(f.Exclude, n)
}

func matchAny(pats []string, name string) bool {
	for _, p := range pats {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// splitExt splits "report.final.txt" into "report.final" and "txt". A leading dot is part of the name (".bashrc" has
// no extension), like in TC.
func splitExt(name string) (base, ext string, hasDot bool) {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return name, "", false
	}
	return name[:i], name[i+1:], true
}

// ApplyMask renames name by a TC rename mask: "*.bak" keeps the name and replaces the extension, "new_*.*" puts a
// prefix in front, "????.txt" keeps the first four characters. A "*" stands for the (rest of the) name part or
// extension part, each "?" for its next character. A mask without wildcards is the new name itself.
func ApplyMask(mask, name string) string {
	if mask == "" {
		return name
	}
	if !strings.ContainsAny(mask, "*?") {
		return mask
	}
	mb, me, mdot := splitExt(mask)
	nb, ne, _ := splitExt(name)
	if !mdot {
		if mask == "*" {
			return name
		}
		return fill(mask, name)
	}
	out := fill(mb, nb)
	ext := fill(me, ne)
	if ext == "" {
		return out
	}
	return out + "." + ext
}

// fill replaces * by the rest of src and each ? by the next character of src (nothing past its end).
func fill(mask, src string) string {
	var b strings.Builder
	rs := []rune(src)
	pos := 0
	for _, r := range mask {
		switch r {
		case '*':
			if pos < len(rs) {
				b.WriteString(string(rs[pos:]))
			}
			pos = len(rs)
		case '?':
			if pos < len(rs) {
				b.WriteRune(rs[pos])
			}
			pos++
		default:
			b.WriteRune(r) // literal characters don't use up characters of src: "new_*" is "new_" plus the whole name
		}
	}
	return b.String()
}

// UniqueName returns a name like name that is free in dir: "file.txt", then "file(2).txt", "file(3).txt" and so on
// (TC's auto-rename). Folders and names without an extension get the number at the end.
func UniqueName(dir, name string) string {
	return uniqueName(dir, name, func(p string) bool { _, err := lstat(p); return err == nil })
}

func uniqueName(dir, name string, taken func(string) bool) string {
	if !taken(filepath.Join(dir, name)) {
		return name
	}
	base, ext, dot := splitExt(name)
	for i := 2; ; i++ {
		n := base + "(" + strconv.Itoa(i) + ")"
		if dot {
			n += "." + ext
		}
		if !taken(filepath.Join(dir, n)) {
			return n
		}
	}
}

// ParseTarget splits what the user typed in the copy/move dialog into the target folder and a new name or mask, the
// way TC reads it: a path ending in a separator, or naming an existing folder, is the folder; a last part with * or ?
// is a mask; for one source a last part that isn't an existing folder is the new name; for several sources it is a
// folder that will be made.
func ParseTarget(input string, sources int) (dir, rename string) {
	if input == "" {
		return "", ""
	}
	if strings.HasSuffix(input, "/") || strings.HasSuffix(input, string(filepath.Separator)) {
		return filepath.Clean(input), ""
	}
	clean := filepath.Clean(input)
	if fi, err := os.Stat(lp(clean)); err == nil && fi.IsDir() {
		return clean, ""
	}
	base := filepath.Base(clean)
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return clean, "" // never hand back a "name" that is really a step up or the folder itself
	}
	if strings.ContainsAny(base, "*?") || sources == 1 {
		return filepath.Dir(clean), base
	}
	return clean, ""
}
