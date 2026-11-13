// This file parses MLSD and MLST lines (RFC 3659), the only listing format
// that was designed for programs rather than people:
//
//	type=file;size=1234;modify=20240131120000;unix.mode=0644;perm=adfrw; name.txt
//
// Facts are "name=value" pairs separated by semicolons, followed by one
// space and the file name, which may contain anything except a line break.
// Times are UTC with optional fractions of a second. Besides the standard
// facts the parser understands the common UNIX extensions (unix.mode,
// unix.owner/ownername, unix.group/groupname) and the "OS.unix=slink:target"
// way some servers mark symbolic links.
package ftp

import (
	"io/fs"
	"strconv"
	"strings"
	"time"
)

func parseMLSx(line string, opt ParseOptions) (Entry, bool) {
	line = strings.TrimLeft(line, " ") // MLST replies indent the fact line
	sp := strings.IndexByte(line, ' ')
	if sp <= 0 {
		return Entry{}, false
	}
	factText := line[:sp]
	if !strings.Contains(factText, "=") || !strings.Contains(factText, ";") {
		return Entry{}, false
	}
	facts := map[string]string{}
	for _, f := range strings.Split(factText, ";") {
		if f == "" {
			continue
		}
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			return Entry{}, false
		}
		facts[strings.ToLower(k)] = v
	}
	e := Entry{Name: line[sp+1:], Format: FormatMLSD, Size: -1, Facts: facts}
	typ := strings.ToLower(facts["type"])
	switch {
	case typ == "file":
		e.Type = TypeFile
	case typ == "dir":
		e.Type = TypeDir
	case typ == "cdir":
		e.Type, e.Name = TypeDir, "."
	case typ == "pdir":
		e.Type, e.Name = TypeDir, ".."
	case strings.HasPrefix(typ, "os.unix=slink") || strings.HasPrefix(typ, "os.unix=symlink") || typ == "link":
		e.Type = TypeLink
		if _, target, ok := strings.Cut(facts["type"], ":"); ok {
			e.Target = target
		}
	case typ == "":
		e.Type = TypeFile
	default:
		e.Type = TypeOther
	}
	if s, ok := facts["size"]; ok {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			e.Size = n
		}
	} else if s, ok := facts["sizd"]; ok {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			e.Size = n
		}
	}
	if s, ok := facts["modify"]; ok {
		if t, ok := parseFactTime(s); ok {
			e.ModTime = t
		}
	}
	if s, ok := facts["unix.mode"]; ok {
		if n, err := strconv.ParseUint(s, 8, 32); err == nil {
			e.Mode = unixMode(uint32(n))
			e.HasMode = true
		}
	}
	switch e.Type {
	case TypeDir:
		e.Mode |= fs.ModeDir
	case TypeLink:
		e.Mode |= fs.ModeSymlink
	}
	e.Owner = firstFact(facts, "unix.ownername", "unix.owner", "unix.uid")
	e.Group = firstFact(facts, "unix.groupname", "unix.group", "unix.gid")
	e.Perm = facts["perm"]
	return e, true
}

func firstFact(f map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := f[k]; v != "" {
			return v
		}
	}
	return ""
}

// unixMode converts a numeric chmod value into an fs.FileMode.
func unixMode(n uint32) fs.FileMode {
	m := fs.FileMode(n & 0o777)
	if n&0o4000 != 0 {
		m |= fs.ModeSetuid
	}
	if n&0o2000 != 0 {
		m |= fs.ModeSetgid
	}
	if n&0o1000 != 0 {
		m |= fs.ModeSticky
	}
	return m
}

// parseFactTime parses "YYYYMMDDHHMMSS[.sss]" (MLSD modify, MDTM) as UTC.
func parseFactTime(s string) (time.Time, bool) {
	main, frac, _ := strings.Cut(s, ".")
	if len(main) != 14 || !isDigits(main) {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("20060102150405", main, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	if frac != "" && isDigits(frac) {
		for len(frac) < 9 {
			frac += "0"
		}
		ns, _ := strconv.Atoi(frac[:9])
		t = t.Add(time.Duration(ns))
	}
	return t, true
}
