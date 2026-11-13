// This file parses DOS/Windows style listing lines, the default output of
// Microsoft IIS and of many Windows FTP servers:
//
//	01-02-06  03:04PM       <DIR>          Folder name
//	01-02-2006  15:04            1,234 file.txt
//
// The date comes first (month-day-year with two or four digit years, or the
// ISO and European variants some servers print), then a 12- or 24-hour time,
// then either a <DIR>-style marker or the size, and the rest of the line is
// the name, spaces included. DOS listings carry no permissions or owners.
package ftp

import (
	"io/fs"
	"strings"
)

func parseDOSDate(s string) (y, mo, d int, ok bool) {
	if y, mo, d, ok := isoDate(s); ok {
		return y, mo, d, true
	}
	sep := ""
	for _, c := range []string{"-", "/", "."} {
		if strings.Count(s, c) == 2 {
			sep = c
			break
		}
	}
	if sep == "" {
		return 0, 0, 0, false
	}
	parts := strings.Split(s, sep)
	a, ok1 := atoi(parts[0])
	b, ok2 := atoi(parts[1])
	c, ok3 := atoi(parts[2])
	if !ok1 || !ok2 || !ok3 || len(parts[2]) != 2 && len(parts[2]) != 4 {
		return 0, 0, 0, false
	}
	y = expandYear(c)
	if sep == "." {
		mo, d = b, a // European DD.MM.YYYY
	} else {
		mo, d = a, b // US MM-DD-YY
		if mo > 12 && d <= 12 {
			mo, d = d, mo
		}
	}
	if !validDate(y, mo, d) {
		return 0, 0, 0, false
	}
	return y, mo, d, true
}

func parseDOS(line string, opt ParseOptions) (Entry, bool) {
	toks := tokenize(line)
	if len(toks) < 4 {
		return Entry{}, false
	}
	y, mo, d, ok := parseDOSDate(toks[0].s)
	if !ok {
		return Entry{}, false
	}
	i := 1
	ampm := ""
	if len(toks) > 4 {
		if u := strings.ToUpper(toks[2].s); u == "AM" || u == "PM" {
			ampm = u
		}
	}
	h, mi, sec, ok := parseClock(toks[i].s, ampm)
	if !ok {
		return Entry{}, false
	}
	i++
	if ampm != "" {
		i++
	}
	if i >= len(toks)-1 {
		return Entry{}, false
	}
	e := Entry{Format: FormatDOS, Size: -1}
	e.ModTime = opt.listTime(y, mo, d, h, mi, sec)
	switch m := strings.ToUpper(toks[i].s); m {
	case "<DIR>":
		e.Type = TypeDir
		e.Mode = fs.ModeDir
	case "<JUNCTION>", "<SYMLINKD>", "<SYMLINK>":
		e.Type = TypeLink
		e.Mode = fs.ModeSymlink
	default:
		n, ok := parseSize(toks[i].s)
		if !ok {
			return Entry{}, false
		}
		e.Type = TypeFile
		e.Size = n
	}
	name := restAfter(line, toks[i])
	if e.Type == TypeLink && strings.HasSuffix(name, "]") {
		if j := strings.LastIndex(name, " ["); j > 0 {
			e.Target = name[j+2 : len(name)-1]
			name = name[:j]
		}
	}
	e.Name = name
	return e, true
}
