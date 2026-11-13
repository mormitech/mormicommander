// This file turns the text of a directory listing into entries the panel
// can show. FTP never standardised the LIST output, so servers send whatever
// their operating system's "dir" or "ls" prints; Total Commander copes by
// trying a series of parsers on every line, and so does this file: the
// machine-readable MLSD format first, then UNIX ls, DOS/IIS, VMS, EPLF and
// OS/400. Lines that no parser understands are returned separately, and if
// nothing at all could be parsed the lines are taken as bare file names (the
// last-resort fallback for exotic servers). Timestamps from LIST are in the
// server's local time and are corrected by the user's time offset; MLSD
// timestamps are UTC by definition and are used as they are.
package ftp

import (
	"io/fs"
	"strconv"
	"strings"
	"time"
)

// EntryType classifies a listing entry.
type EntryType int

const (
	TypeFile EntryType = iota
	TypeDir
	TypeLink
	TypeOther // devices, pipes, sockets, unknown objects
)

// ListFormat names the dialect a line was parsed as.
type ListFormat int

const (
	FormatUnknown ListFormat = iota
	FormatMLSD
	FormatUnix
	FormatDOS
	FormatVMS
	FormatEPLF
	FormatOS400
	FormatNameOnly
)

// String returns a short name of the format.
func (f ListFormat) String() string {
	return [...]string{"unknown", "mlsd", "unix", "dos", "vms", "eplf", "os400", "names"}[f]
}

// Entry is one item of a remote folder.
type Entry struct {
	Name    string
	Type    EntryType
	Size    int64     // bytes; -1 when unknown
	ModTime time.Time // zero when unknown
	// Mode holds Unix permission bits plus fs.ModeDir/fs.ModeSymlink;
	// HasMode tells whether the server reported permissions at all.
	Mode    fs.FileMode
	HasMode bool
	Owner   string
	Group   string
	Target  string            // symbolic link target, when the server shows it
	Perm    string            // raw permission text (ls "rwxr-xr-x", MLSD perm fact, VMS protection)
	Facts   map[string]string // MLSD facts, lower-case keys
	Format  ListFormat
	Raw     string // the original line
}

// IsDir reports whether the entry is a folder.
func (e Entry) IsDir() bool { return e.Type == TypeDir }

// ParseOptions control how listing timestamps are interpreted.
type ParseOptions struct {
	Location *time.Location // zone of LIST timestamps; default time.Local
	Offset   time.Duration  // user correction added to LIST timestamps
	Now      time.Time      // reference for year-less UNIX dates; default time.Now()
}

func (o ParseOptions) loc() *time.Location {
	if o.Location == nil {
		return time.Local
	}
	return o.Location
}

func (o ParseOptions) now() time.Time {
	if o.Now.IsZero() {
		return time.Now()
	}
	return o.Now
}

// ParseList parses all lines of a listing. "." and ".." are dropped. The
// second result holds lines that no parser understood (headers such as
// "total 12" are silently skipped and not reported).
func ParseList(lines []string, opt ParseOptions) ([]Entry, []string) {
	lines = joinVMSContinuations(lines)
	var out []Entry
	var bad []string
	for _, l := range lines {
		if isListNoise(l) {
			continue
		}
		e, ok := ParseLine(l, opt)
		if !ok {
			bad = append(bad, l)
			continue
		}
		if e.Name == "" || e.Name == "." || e.Name == ".." {
			continue
		}
		out = append(out, e)
	}
	if len(out) == 0 && len(bad) > 0 {
		for _, l := range bad {
			name := strings.TrimSpace(l)
			out = append(out, Entry{Name: name, Type: TypeFile, Size: -1, Format: FormatNameOnly, Raw: l})
		}
		bad = nil
	}
	return out, bad
}

// ParseLine parses one listing line with the first parser that accepts it.
func ParseLine(line string, opt ParseOptions) (Entry, bool) {
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return Entry{}, false
	}
	for _, p := range []func(string, ParseOptions) (Entry, bool){
		parseMLSx, parseEPLF, parseUnix, parseDOS, parseVMS, parseOS400,
	} {
		if e, ok := p(line, opt); ok {
			e.Raw = line
			return e, true
		}
	}
	return Entry{}, false
}

func isListNoise(l string) bool {
	t := strings.TrimSpace(l)
	lt := strings.ToLower(t)
	switch {
	case t == "":
		return true
	case strings.HasPrefix(lt, "total ") && isDigits(strings.TrimSpace(t[6:])):
		return true
	case strings.HasPrefix(lt, "total of "): // VMS trailer
		return true
	case strings.HasPrefix(lt, "directory ") && strings.Contains(t, "[") && strings.HasSuffix(t, "]"): // VMS header
		return true
	}
	return false
}

// token is a whitespace-separated word plus where it ends in the line, so
// a name containing spaces can be cut out of the original text.
type token struct {
	s     string
	start int
	end   int
}

func tokenize(line string) []token {
	var toks []token
	i := 0
	for i < len(line) {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) {
			break
		}
		j := i
		for j < len(line) && line[j] != ' ' && line[j] != '\t' {
			j++
		}
		toks = append(toks, token{line[i:j], i, j})
		i = j
	}
	return toks
}

// restAfter returns the line text after token t, minus the separating
// whitespace.
func restAfter(line string, t token) string {
	return strings.TrimLeft(line[t.end:], " \t")
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func atoi(s string) (int, bool) {
	if !isDigits(s) {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// parseSize accepts "1234" and "1,234" / "1.234" thousands separators.
func parseSize(s string) (int64, bool) {
	s = strings.NewReplacer(",", "", ".", "").Replace(s)
	if !isDigits(s) {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

// parseClock accepts "15:04", "15:04:05", "15:04:05.99", "3:04PM", "03:04 PM".
func parseClock(s, ampm string) (h, m, sec int, ok bool) {
	s = strings.ToUpper(s)
	if ampm == "" {
		if strings.HasSuffix(s, "AM") || strings.HasSuffix(s, "PM") {
			ampm = s[len(s)-2:]
			s = s[:len(s)-2]
		}
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, false
	}
	if h, ok = atoi(parts[0]); !ok || h > 23 {
		return 0, 0, 0, false
	}
	if m, ok = atoi(parts[1]); !ok || m > 59 {
		return 0, 0, 0, false
	}
	if len(parts) == 3 {
		secPart, _, _ := strings.Cut(parts[2], ".")
		if sec, ok = atoi(secPart); !ok || sec > 60 {
			return 0, 0, 0, false
		}
	}
	switch strings.ToUpper(ampm) {
	case "":
	case "AM":
		if h > 12 {
			return 0, 0, 0, false
		}
		if h == 12 {
			h = 0
		}
	case "PM":
		if h > 12 {
			return 0, 0, 0, false
		}
		if h < 12 {
			h += 12
		}
	default:
		return 0, 0, 0, false
	}
	return h, m, sec, true
}

func validDate(y, mo, d int) bool {
	if mo < 1 || mo > 12 || d < 1 || d > 31 || y < 1 {
		return false
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	return t.Day() == d
}

func expandYear(y int) int {
	if y < 100 {
		if y < 70 {
			return 2000 + y
		}
		return 1900 + y
	}
	return y
}

var monthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
	// common German/French/Spanish/Dutch abbreviations seen on localised servers
	"mär": 3, "mrz": 3, "mai": 5, "okt": 10, "dez": 12, "fév": 2, "avr": 4,
	"jui": 7, "aoû": 8, "déc": 12, "ene": 1, "abr": 4, "ago": 8, "dic": 12,
	"mei": 5, "maa": 3,
}

func monthOf(s string) (int, bool) {
	s = strings.ToLower(strings.TrimSuffix(s, "."))
	if len(s) > 3 {
		// "June", "Sept": use the first three letters
		r := []rune(s)
		if len(r) > 3 {
			s = string(r[:3])
		}
	}
	m, ok := monthNames[s]
	return m, ok
}

func (o ParseOptions) listTime(y, mo, d, h, mi, s int) time.Time {
	return time.Date(y, time.Month(mo), d, h, mi, s, 0, o.loc()).Add(o.Offset)
}
