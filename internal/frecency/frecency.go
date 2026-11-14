// Package frecency ranks folders by how often and how recently they were used, for the frecency jump (F-867, board
// issue #27). It has two sources: the app's own frequently used folders (visit counts that fade, kept in folders.ini
// by internal/config), and, when the user has zoxide installed, zoxide's database, which is only read, never written
// or copied: the jump reads it again each time it opens, so nothing of it is stored by the app.
//
// zoxide (0.8 and later) keeps its folders in "db.zo", a bincode file: a little-endian u32 format version (3), then a
// u64 count and that many entries, each a u64-length UTF-8 path, an f64 rank and a u64 last-access time in Unix
// seconds. Older zoxide versions and z.sh kept a text file with one "path|rank|time" line per folder; that form is
// read too. zoxide ranks a folder by its rank times a factor for how recently it was used (4 within the hour, 2 within
// the day, 0.5 within the week, 0.25 after that); Score does the same.
package frecency

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Entry is one folder of zoxide's database.
type Entry struct {
	Path         string
	Rank         float64
	LastAccessed int64 // Unix seconds
}

// Item is one folder of the merged, ranked list.
type Item struct {
	Path   string  `json:"path"`
	Score  float64 `json:"score"`
	Source string  `json:"source"` // "app", "zoxide" or "both"
}

// zoxideVersion is the only binary format version this reader knows.
const zoxideVersion = 3

// maxEntries bounds what a damaged file can make the reader allocate.
const maxEntries = 1 << 20

// ErrFormat says the file is neither of the forms zoxide has used.
var ErrFormat = errors.New("The zoxide database is in a form this version does not know.")

// ParseZoxide reads zoxide's database: the binary db.zo, or the older text form. An empty file is an empty database.
func ParseZoxide(b []byte) ([]Entry, error) {
	if len(b) == 0 {
		return nil, nil
	}
	if len(b) >= 4 && binary.LittleEndian.Uint32(b) == zoxideVersion {
		return parseBinary(b[4:])
	}
	return parseText(b)
}

func parseBinary(b []byte) ([]Entry, error) {
	r := bytes.NewReader(b)
	var n uint64
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil || n > maxEntries {
		return nil, ErrFormat
	}
	out := make([]Entry, 0, n)
	for i := uint64(0); i < n; i++ {
		var l uint64
		if err := binary.Read(r, binary.LittleEndian, &l); err != nil || l > uint64(r.Len()) {
			return nil, ErrFormat
		}
		p := make([]byte, l)
		if _, err := r.Read(p); err != nil && l > 0 {
			return nil, ErrFormat
		}
		var rank float64
		var at uint64
		if err := binary.Read(r, binary.LittleEndian, &rank); err != nil {
			return nil, ErrFormat
		}
		if err := binary.Read(r, binary.LittleEndian, &at); err != nil {
			return nil, ErrFormat
		}
		if !utf8.Valid(p) {
			continue
		}
		if e, ok := clean(string(p), rank, int64(at)); ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// parseText reads "path|rank|time" lines; the path may itself hold "|", so the two numbers are taken from the right.
func parseText(b []byte) ([]Entry, error) {
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return nil, ErrFormat
	}
	var out []Entry
	bad := 0
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		i := strings.LastIndexByte(line, '|')
		j := -1
		if i > 0 {
			j = strings.LastIndexByte(line[:i], '|')
		}
		if j <= 0 {
			bad++
			continue
		}
		rank, err1 := strconv.ParseFloat(line[j+1:i], 64)
		at, err2 := strconv.ParseInt(line[i+1:], 10, 64)
		if err1 != nil || err2 != nil {
			bad++
			continue
		}
		if e, ok := clean(line[:j], rank, at); ok {
			out = append(out, e)
		}
	}
	if len(out) == 0 && bad > 0 {
		return nil, ErrFormat
	}
	return out, nil
}

// clean drops entries that cannot be used: no path, a path with line breaks, or a rank that is not a positive number.
func clean(p string, rank float64, at int64) (Entry, bool) {
	if p == "" || strings.ContainsAny(p, "\r\n\x00") || math.IsNaN(rank) || math.IsInf(rank, 0) || rank <= 0 {
		return Entry{}, false
	}
	return Entry{Path: p, Rank: rank, LastAccessed: at}, true
}

// Score is zoxide's frecency of an entry at the time now.
func Score(e Entry, now time.Time) float64 {
	age := now.Unix() - e.LastAccessed
	switch {
	case age < 3600:
		return e.Rank * 4
	case age < 86400:
		return e.Rank * 2
	case age < 7*86400:
		return e.Rank / 2
	}
	return e.Rank / 4
}

// ZoxideDBPath says where zoxide keeps its database on the platform goos, the way zoxide itself finds it:
// $_ZO_DATA_DIR, else the platform's local data folder plus "zoxide". It returns "" when no folder can be worked out.
func ZoxideDBPath(goos string, getenv func(string) string, home string) string {
	join := filepath.Join
	if goos == "windows" {
		join = func(p ...string) string { return strings.Join(p, `\`) }
	}
	if d := getenv("_ZO_DATA_DIR"); d != "" {
		return join(d, "db.zo")
	}
	switch goos {
	case "windows":
		if d := getenv("LOCALAPPDATA"); d != "" {
			return join(d, "zoxide", "db.zo")
		}
		return ""
	case "darwin":
		if home == "" {
			return ""
		}
		return join(home, "Library", "Application Support", "zoxide", "db.zo")
	}
	if d := getenv("XDG_DATA_HOME"); d != "" && filepath.IsAbs(d) {
		return join(d, "zoxide", "db.zo")
	}
	if home == "" {
		return ""
	}
	return join(home, ".local", "share", "zoxide", "db.zo")
}

// Merge puts the app's own scores (folder → score) and zoxide's entries in one list, best first. key says when two
// spellings are the same folder (the platform's rules for case and trailing separators); a folder in both lists gets
// the sum of its scores and the app's spelling. Ties are broken by path so the order is stable.
func Merge(own map[string]float64, zox []Entry, now time.Time, key func(string) string) []Item {
	byKey := map[string]*Item{}
	var order []*Item
	add := func(p string, s float64, src string) {
		if p == "" || s <= 0 || math.IsNaN(s) || math.IsInf(s, 0) {
			return
		}
		k := key(p)
		if it := byKey[k]; it != nil {
			it.Score += s
			if it.Source != src {
				it.Source = "both"
			}
			return
		}
		it := &Item{Path: p, Score: s, Source: src}
		byKey[k] = it
		order = append(order, it)
	}
	// the app's folders first, in a fixed order, so their spelling wins over zoxide's
	names := make([]string, 0, len(own))
	for p := range own {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		add(p, own[p], "app")
	}
	for _, e := range zox {
		add(e.Path, Score(e, now), "zoxide")
	}
	out := make([]Item, len(order))
	for i, it := range order {
		out[i] = *it
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Path < out[j].Path
	})
	return out
}
