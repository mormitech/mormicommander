// This file parses UNIX "ls -l" style listing lines, by far the most common
// LIST format (vsftpd, ProFTPD, Pure-FTPd, most NAS boxes, and IIS in its
// UNIX mode). The parser does not rely on fixed columns, because servers
// drop the group or link-count column, print device numbers instead of a
// size, use ISO dates (ls --time-style) or localised month names. Instead it
// recognises the permission string at the start, finds the date by looking
// for a month name followed by a day and a time or year, takes the number in
// front of it as the size, and cuts the name out of the original text so
// names with spaces survive. Dates without a year are placed in the last
// twelve months, just like ls does when it prints them.
package ftp

import (
	"io/fs"
	"strings"
	"time"
)

func isPermString(s string) bool {
	if len(s) < 10 {
		return false
	}
	if !strings.ContainsRune("-dlbcpsDn?", rune(s[0])) {
		return false
	}
	for i := 1; i < 10; i++ {
		if !strings.ContainsRune("rwxsStTlL-", rune(s[i])) {
			return false
		}
	}
	if len(s) > 10 {
		rest := s[10:]
		if rest != "+" && rest != "." && rest != "@" {
			return false
		}
	}
	return true
}

// permMode converts "rwxr-sr-t" style bits into an fs.FileMode.
func permMode(p string) fs.FileMode {
	var m fs.FileMode
	bit := func(i int, c byte, v fs.FileMode) {
		if p[i] == c {
			m |= v
		}
	}
	bit(1, 'r', 0o400)
	bit(2, 'w', 0o200)
	bit(4, 'r', 0o040)
	bit(5, 'w', 0o020)
	bit(7, 'r', 0o004)
	bit(8, 'w', 0o002)
	exec := func(i int, set fs.FileMode, x fs.FileMode, lower, upper byte) {
		switch p[i] {
		case 'x':
			m |= x
		case lower:
			m |= x | set
		case upper:
			m |= set
		}
	}
	exec(3, fs.ModeSetuid, 0o100, 's', 'S')
	exec(6, fs.ModeSetgid, 0o010, 's', 'S')
	exec(9, fs.ModeSticky, 0o001, 't', 'T')
	return m
}

func parseUnix(line string, opt ParseOptions) (Entry, bool) {
	toks := tokenize(line)
	if len(toks) < 4 || !isPermString(toks[0].s) {
		return Entry{}, false
	}
	e := Entry{Format: FormatUnix, Size: -1, HasMode: true, Perm: toks[0].s[:10]}
	switch toks[0].s[0] {
	case 'd', 'D':
		e.Type = TypeDir
	case 'l':
		e.Type = TypeLink
	case '-':
		e.Type = TypeFile
	default:
		e.Type = TypeOther
	}
	e.Mode = permMode(e.Perm)
	switch e.Type {
	case TypeDir:
		e.Mode |= fs.ModeDir
	case TypeLink:
		e.Mode |= fs.ModeSymlink
	}

	// Find the date: "Mon DD HH:MM|YYYY", "DD Mon HH:MM|YYYY" or ISO.
	dateAt, last := -1, -1
	now := opt.now().In(opt.loc())
	for i := 2; i < len(toks)-1 && dateAt < 0; i++ {
		s := toks[i].s
		if y, mo, d, ok := isoDate(s); ok {
			h, mi, sec := 0, 0, 0
			last = i
			if i+1 < len(toks)-1 {
				if hh, mm, ss, ok := parseClock(toks[i+1].s, ""); ok {
					h, mi, sec = hh, mm, ss
					last = i + 1
				}
			}
			if last == len(toks)-1 {
				continue // nothing left for a name
			}
			e.ModTime = opt.listTime(y, mo, d, h, mi, sec)
			dateAt = i
			break
		}
		if i+2 >= len(toks) {
			break
		}
		var mo, d int
		var ok bool
		if mo, ok = monthOf(s); ok {
			d, ok = atoi(strings.TrimSuffix(toks[i+1].s, "."))
		} else if d, ok = atoi(strings.TrimSuffix(s, ".")); ok {
			mo, ok = monthOf(toks[i+1].s)
		}
		if !ok || d < 1 || d > 31 {
			continue
		}
		ty := toks[i+2].s
		if y, ok := atoi(ty); ok && len(ty) == 4 {
			if !validDate(y, mo, d) {
				continue
			}
			e.ModTime = opt.listTime(y, mo, d, 0, 0, 0)
		} else if h, mi, sec, ok := parseClock(ty, ""); ok {
			y := now.Year()
			if !validDate(y, mo, d) && !validDate(y-1, mo, d) {
				continue
			}
			t := opt.listTime(y, mo, d, h, mi, sec)
			if t.Sub(now) > 48*time.Hour || !validDate(y, mo, d) {
				t = opt.listTime(y-1, mo, d, h, mi, sec)
			}
			e.ModTime = t
		} else {
			continue
		}
		dateAt, last = i, i+2
	}
	if dateAt < 0 || last >= len(toks)-1 && restAfter(line, toks[last]) == "" {
		return Entry{}, false
	}

	// The size is the number just before the date (for devices "maj, min").
	sizeAt := dateAt - 1
	if n, ok := parseSize(toks[sizeAt].s); ok && !strings.Contains(toks[sizeAt].s, ",") {
		e.Size = n
	} else if e.Type != TypeOther {
		return Entry{}, false
	}
	// Columns between the permissions and the size: [links] owner [group].
	mid := toks[1:sizeAt]
	if e.Type == TypeOther && len(mid) > 0 && strings.HasSuffix(mid[len(mid)-1].s, ",") {
		mid = mid[:len(mid)-1] // device major number
		e.Size = 0
	}
	// Drop the link count: always with three columns, and with two when
	// only the first one is numeric ("1 ftp" = links + owner).
	if len(mid) >= 3 || len(mid) == 2 && isDigits(mid[0].s) && !isDigits(mid[1].s) {
		mid = mid[1:]
	}
	if len(mid) > 0 {
		e.Owner = mid[0].s
	}
	if len(mid) > 1 {
		e.Group = mid[1].s
	}

	// ls separates the name from the time or year with exactly one space
	// (the columns before it are padded, the name never is), so only that
	// one separator is removed: a name that itself starts with spaces must
	// survive, or " a" and "a" would collapse into the same entry.
	name := nameAfter(line, toks[last])
	if e.Type == TypeLink {
		if n, target, ok := strings.Cut(name, " -> "); ok {
			name, e.Target = n, target
		}
	}
	e.Name = name
	return e, true
}

// nameAfter returns the text after token t minus the single blank that
// separates it from the name. Leading blanks beyond that one belong to the
// name itself.
func nameAfter(line string, t token) string {
	rest := line[t.end:]
	if rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
		rest = rest[1:]
	}
	return rest
}

// isoDate parses "2006-01-02".
func isoDate(s string) (y, mo, d int, ok bool) {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return 0, 0, 0, false
	}
	y, ok1 := atoi(s[:4])
	mo, ok2 := atoi(s[5:7])
	d, ok3 := atoi(s[8:])
	if !ok1 || !ok2 || !ok3 || !validDate(y, mo, d) {
		return 0, 0, 0, false
	}
	return y, mo, d, true
}
