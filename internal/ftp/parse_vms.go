// This file parses OpenVMS listings, which still turn up on old
// institutional servers:
//
//	Directory DISK$USER:[SMITH]
//
//	LOGIN.COM;3          2/3      12-JAN-2020 10:15:22  [GROUP,SMITH]  (RWED,RWED,RE,)
//	PROJECTS.DIR;1       1/3       2-FEB-2021 08:00     [GROUP,SMITH]  (RWE,RWE,RE,E)
//
// Names carry a ";version" suffix that is stripped, folders end in ".DIR",
// sizes are counted in 512-byte blocks, and a long name may push the rest
// of its entry onto the next line, which joinVMSContinuations glues back
// together before parsing. Header and "Total of" lines are skipped.
package ftp

import (
	"io/fs"
	"strings"
)

func isVMSName(s string) bool {
	i := strings.LastIndexByte(s, ';')
	return i > 0 && isDigits(s[i+1:])
}

// joinVMSContinuations merges a VMS entry whose name sits alone on one line
// with the indented details on the following line.
func joinVMSContinuations(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		f := strings.Fields(l)
		if len(f) == 1 && isVMSName(f[0]) && i+1 < len(lines) {
			next := lines[i+1]
			if strings.HasPrefix(next, " ") || strings.HasPrefix(next, "\t") {
				out = append(out, strings.TrimSpace(l)+" "+strings.TrimSpace(next))
				i++
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

func parseVMS(line string, opt ParseOptions) (Entry, bool) {
	toks := tokenize(line)
	if len(toks) < 3 || !isVMSName(toks[0].s) {
		return Entry{}, false
	}
	name := toks[0].s[:strings.LastIndexByte(toks[0].s, ';')]
	e := Entry{Format: FormatVMS, Size: -1, Type: TypeFile}
	if strings.HasSuffix(strings.ToUpper(name), ".DIR") {
		e.Type = TypeDir
		e.Mode = fs.ModeDir
		name = name[:len(name)-4]
	}
	e.Name = name

	i := 1
	used, _, _ := strings.Cut(toks[i].s, "/")
	if n, ok := parseSize(used); ok {
		e.Size = n * 512
		i++
	}
	if i < len(toks) {
		parts := strings.Split(toks[i].s, "-")
		if len(parts) == 3 {
			d, ok1 := atoi(parts[0])
			mo, ok2 := monthOf(parts[1])
			y, ok3 := atoi(parts[2])
			if ok1 && ok2 && ok3 && validDate(expandYear(y), mo, d) {
				y = expandYear(y)
				h, mi, sec := 0, 0, 0
				i++
				if i < len(toks) {
					if hh, mm, ss, ok := parseClock(toks[i].s, ""); ok {
						h, mi, sec = hh, mm, ss
						i++
					}
				}
				e.ModTime = opt.listTime(y, mo, d, h, mi, sec)
			}
		}
	}
	if e.ModTime.IsZero() && e.Size < 0 {
		return Entry{}, false // a bare "NAME;1" without details is not enough
	}
	for ; i < len(toks); i++ {
		s := toks[i].s
		switch {
		case strings.HasPrefix(s, "["):
			owner := strings.Trim(s, "[]")
			if g, u, ok := strings.Cut(owner, ","); ok {
				e.Group, e.Owner = g, u
			} else {
				e.Owner = owner
			}
		case strings.HasPrefix(s, "("):
			e.Perm = s
		}
	}
	return e, true
}
