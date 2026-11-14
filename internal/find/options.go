// This file defines Options, everything the Find Files dialog can ask for, in one
// plain struct that the UI fills from its tabs (General, Advanced) and that is saved
// as-is for search templates (Load/Save tab, F-537). Every field has a JSON name so a
// saved search is readable and survives new fields being added later. The zero value
// of each filter means "not used", with one exception explained at Levels: the depth
// default is "all subfolders", so callers should start from DefaultOptions.

package find

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"time"

	"mormicommander/internal/rxflags"
)

// Unlimited is the Levels value for "search all subfolders".
const Unlimited = -1

// Kind restricts the results to files, folders or both (F-535).
type Kind uint8

const (
	KindAll   Kind = iota // files and folders
	KindFiles             // files only
	KindDirs              // folders only
)

// Options is one Find Files search.
type Options struct {
	// SearchFor is the "Search for" field: wildcard masks with ";" and "|" exclusions
	// (see mask.go), or a regular expression when NameRegex is set. Empty means "*".
	SearchFor string `json:"searchFor,omitempty"`
	// NameRegex treats SearchFor as a regular expression over the file name (F-530).
	// It is case-insensitive unless NameRegexCase is set, matching Total Commander.
	NameRegex     bool `json:"nameRegex,omitempty"`
	NameRegexCase bool `json:"nameRegexCase,omitempty"`

	// SearchIn holds the start folders ("Search in", F-526). ParseSearchIn splits the
	// TC-style "c:\a;d:\b" string. A start path that is a file is tested by itself.
	SearchIn []string `json:"searchIn,omitempty"`
	// Levels is how many levels of subfolders to enter below each start folder:
	// Unlimited (-1) for all, 0 for the start folder only, n for n levels (F-527).
	Levels int `json:"levels"`
	// FollowLinks enters symbolic links to folders and reports link targets' sizes
	// and dates. Loops (a link to an ancestor) are detected and skipped.
	FollowLinks bool `json:"followLinks,omitempty"`
	// Kind limits results to files or folders (F-535).
	Kind Kind `json:"kind,omitempty"`
	// SkipHidden leaves out hidden files and does not enter hidden folders, for users
	// who switched off "Show hidden files".
	SkipHidden bool `json:"skipHidden,omitempty"`
	// ExcludeDirs are extra folder patterns to skip, in addition to "|name/" entries
	// in SearchFor (F-837). Each is a wildcard mask for the folder name, or for the
	// relative path if it contains a separator.
	ExcludeDirs []string `json:"excludeDirs,omitempty"`
	// SearchArchives looks inside archives through the Archives handler (F-528).
	SearchArchives bool `json:"searchArchives,omitempty"`

	// Content is the "Find text" group (F-529).
	Content ContentOptions `json:"content,omitzero"`
	// Size is the size filter (F-532).
	Size SizeFilter `json:"size,omitzero"`
	// Date is the date and age filter (F-531).
	Date DateFilter `json:"date,omitzero"`
	// AttrSet must all be set and AttrClear must all be clear (F-533).
	AttrSet   Attr `json:"attrSet,omitempty"`
	AttrClear Attr `json:"attrClear,omitempty"`
	// PermMask and PermValue filter Unix permissions: mode&PermMask == PermValue.
	// For example mask 0o002, value 0o002 finds world-writable files.
	PermMask  fs.FileMode `json:"permMask,omitempty"`
	PermValue fs.FileMode `json:"permValue,omitempty"`

	// Duplicates turns the search into "Find duplicate files" (F-534).
	Duplicates DupOptions `json:"duplicates,omitzero"`

	// Within limits the search to these paths instead of walking SearchIn: this is
	// "Search in found files" (F-544). Folders in the list are tested as entries but
	// not entered, exactly like the result list they came from.
	Within []string `json:"within,omitempty"`

	// FS is the file system to search; nil means the local one (OSFS).
	FS FS `json:"-"`
	// Archives opens archives for SearchArchives; nil disables archive search.
	Archives ArchiveHandler `json:"-"`
	// Now is the clock for age filters; nil means time.Now. Tests set it.
	Now func() time.Time `json:"-"`
}

// DefaultOptions returns the dialog's defaults: all names, all subfolders.
func DefaultOptions() Options {
	return Options{SearchFor: "*", Levels: Unlimited}
}

// ParseSearchIn splits a "Search in" string like `c:\a;"d:\with;semicolon";e:\` into
// folders. Quotes protect semicolons; empty parts are dropped.
func ParseSearchIn(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ';' && !inQuote:
			if t := strings.TrimSpace(cur.String()); t != "" {
				out = append(out, t)
			}
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if t := strings.TrimSpace(cur.String()); t != "" {
		out = append(out, t)
	}
	return out
}

// SizeOp compares a file size.
type SizeOp string

const (
	SizeAny     SizeOp = ""  // no size filter
	SizeEqual   SizeOp = "=" // equal (within the unit, see SizeFilter)
	SizeGreater SizeOp = ">" // strictly greater
	SizeLess    SizeOp = "<" // strictly smaller
)

// SizeUnit scales SizeFilter.Value. Units are binary (1 KB = 1024 bytes) as in TC.
type SizeUnit string

const (
	Bytes SizeUnit = "B"
	KB    SizeUnit = "KB"
	MB    SizeUnit = "MB"
	GB    SizeUnit = "GB"
	TB    SizeUnit = "TB"
)

func (u SizeUnit) factor() (int64, error) {
	switch strings.ToUpper(string(u)) {
	case "", "B":
		return 1, nil
	case "KB", "K":
		return 1 << 10, nil
	case "MB", "M":
		return 1 << 20, nil
	case "GB", "G":
		return 1 << 30, nil
	case "TB", "T":
		return 1 << 40, nil
	}
	return 0, fmt.Errorf("find: unknown size unit %q", u)
}

// SizeFilter is "File size: [op] [value] [unit]". With a unit bigger than bytes,
// SizeEqual matches the whole unit: "= 3 KB" means 3072..4095 bytes, which is what a
// user who reads "3 KB" in a file list expects. Folders never pass an active size
// filter, because Total Commander does not compute folder sizes while searching.
type SizeFilter struct {
	Op    SizeOp   `json:"op,omitempty"`
	Value int64    `json:"value,omitempty"`
	Unit  SizeUnit `json:"unit,omitempty"`
}

// Active reports whether the filter is set.
func (s SizeFilter) Active() bool { return s.Op != SizeAny }

// Match tests a size in bytes. It never multiplies Value by the unit, which
// could overflow int64 for a large value in TB and flip the comparison;
// instead the size is divided by the unit (rounding down) and the quotient
// and remainder are compared with Value. The results are the same as
// "lo := Value*unit" for every value that does not overflow.
func (s SizeFilter) Match(size int64) bool {
	f, err := s.Unit.factor()
	if err != nil {
		return false
	}
	q, r := size/f, size%f
	if r < 0 { // round towards minus infinity so r is always in [0, f)
		q, r = q-1, r+f
	}
	switch s.Op {
	case SizeAny:
		return true
	case SizeEqual:
		return q == s.Value
	case SizeGreater:
		return q > s.Value || (q == s.Value && r > 0)
	case SizeLess:
		return q < s.Value
	}
	return false
}

// AgeUnit is the unit of "Not older than n …".
type AgeUnit string

const (
	Minutes AgeUnit = "minutes"
	Hours   AgeUnit = "hours"
	Days    AgeUnit = "days"
	Weeks   AgeUnit = "weeks"
	Months  AgeUnit = "months"
	Years   AgeUnit = "years"
)

// Age is a span like "3 days". N == 0 means not set.
type Age struct {
	N    int     `json:"n,omitempty"`
	Unit AgeUnit `json:"unit,omitempty"`
}

// Before returns now minus the age, using calendar months and years.
func (a Age) Before(now time.Time) (time.Time, error) {
	switch a.Unit {
	case Minutes:
		return now.Add(-time.Duration(a.N) * time.Minute), nil
	case Hours:
		return now.Add(-time.Duration(a.N) * time.Hour), nil
	case Days, "":
		return now.AddDate(0, 0, -a.N), nil
	case Weeks:
		return now.AddDate(0, 0, -7*a.N), nil
	case Months:
		return now.AddDate(0, -a.N, 0), nil
	case Years:
		return now.AddDate(-a.N, 0, 0), nil
	}
	return time.Time{}, fmt.Errorf("find: unknown age unit %q", a.Unit)
}

// DateFilter is the Advanced tab's date group. From and To are inclusive bounds on
// the modification time (zero means open); the UI passes the end of the "to" day.
// NotOlderThan keeps files changed within the span; OlderThan keeps the rest.
type DateFilter struct {
	From         time.Time `json:"from,omitzero"`
	To           time.Time `json:"to,omitzero"`
	NotOlderThan Age       `json:"notOlderThan,omitzero"`
	OlderThan    Age       `json:"olderThan,omitzero"`
}

// Active reports whether any date condition is set.
func (d DateFilter) Active() bool {
	return !d.From.IsZero() || !d.To.IsZero() || d.NotOlderThan.N != 0 || d.OlderThan.N != 0
}

// dateMatcher is a DateFilter with ages turned into fixed cutoffs at search start, so
// every file in one search is judged against the same moment.
type dateMatcher struct {
	from, to time.Time
}

func compileDate(d DateFilter, now time.Time) (dateMatcher, error) {
	m := dateMatcher{from: d.From, to: d.To}
	if d.NotOlderThan.N != 0 {
		t, err := d.NotOlderThan.Before(now)
		if err != nil {
			return m, err
		}
		if m.from.IsZero() || t.After(m.from) {
			m.from = t
		}
	}
	if d.OlderThan.N != 0 {
		t, err := d.OlderThan.Before(now)
		if err != nil {
			return m, err
		}
		if m.to.IsZero() || t.Before(m.to) {
			m.to = t
		}
	}
	return m, nil
}

func (m dateMatcher) match(t time.Time) bool {
	if !m.from.IsZero() && t.Before(m.from) {
		return false
	}
	if !m.to.IsZero() && t.After(m.to) {
		return false
	}
	return true
}

// compileNameRegex builds the file name regular expression.
func compileNameRegex(expr string, caseSensitive bool) (*regexp.Regexp, error) {
	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}
	re, err := rxflags.Compile(flags, expr)
	if err != nil {
		return nil, fmt.Errorf("find: bad file name regular expression: %w", err)
	}
	return re, nil
}
