// This file holds the parsers for two rarer listing formats that Total
// Commander also understands. EPLF ("Easily Parsed LIST Format", used by
// publicfile and some embedded servers) starts each line with "+", lists
// comma-separated facts and puts the name after a tab. OS/400 (IBM i)
// listings show owner, size, date, time, an object type such as *DIR or
// *STMF, and the name, with folders marked by a trailing slash.
package ftp

import (
	"io/fs"
	"strconv"
	"strings"
	"time"
)

func parseEPLF(line string, opt ParseOptions) (Entry, bool) {
	if !strings.HasPrefix(line, "+") {
		return Entry{}, false
	}
	facts, name, ok := strings.Cut(line[1:], "\t")
	if !ok || name == "" {
		return Entry{}, false
	}
	e := Entry{Name: name, Format: FormatEPLF, Size: -1, Type: TypeFile}
	for _, f := range strings.Split(facts, ",") {
		if f == "" {
			continue
		}
		switch f[0] {
		case '/':
			e.Type = TypeDir
			e.Mode |= fs.ModeDir
		case 'r':
			e.Type = TypeFile
		case 's':
			if n, err := strconv.ParseInt(f[1:], 10, 64); err == nil {
				e.Size = n
			}
		case 'm':
			if n, err := strconv.ParseInt(f[1:], 10, 64); err == nil {
				e.ModTime = time.Unix(n, 0).UTC()
			}
		case 'u':
			if strings.HasPrefix(f, "up") {
				if n, err := strconv.ParseUint(f[2:], 8, 32); err == nil {
					e.Mode |= unixMode(uint32(n))
					e.HasMode = true
				}
			}
		}
	}
	return e, true
}

func parseOS400(line string, opt ParseOptions) (Entry, bool) {
	toks := tokenize(line)
	if len(toks) < 6 || !strings.HasPrefix(toks[4].s, "*") {
		return Entry{}, false
	}
	size, ok := parseSize(toks[1].s)
	if !ok {
		return Entry{}, false
	}
	parts := strings.FieldsFunc(toks[2].s, func(r rune) bool { return r == '/' || r == '.' })
	if len(parts) != 3 {
		return Entry{}, false
	}
	a, ok1 := atoi(parts[0])
	b, ok2 := atoi(parts[1])
	c, ok3 := atoi(parts[2])
	if !ok1 || !ok2 || !ok3 {
		return Entry{}, false
	}
	mo, d := a, b
	if strings.Contains(toks[2].s, ".") {
		mo, d = b, a
	}
	y := expandYear(c)
	if !validDate(y, mo, d) {
		return Entry{}, false
	}
	h, mi, sec, ok := parseClock(toks[3].s, "")
	if !ok {
		return Entry{}, false
	}
	e := Entry{Format: FormatOS400, Size: size, Owner: toks[0].s, Type: TypeFile}
	e.ModTime = opt.listTime(y, mo, d, h, mi, sec)
	name := restAfter(line, toks[4])
	obj := strings.ToUpper(toks[4].s)
	if obj == "*DIR" || obj == "*LIB" || strings.HasSuffix(name, "/") {
		e.Type = TypeDir
		e.Mode = fs.ModeDir
	}
	e.Name = strings.TrimSuffix(name, "/")
	e.Perm = toks[4].s
	return e, e.Name != ""
}
