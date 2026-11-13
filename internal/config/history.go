// Histories: the folder history of each panel (Alt+Down, F-660), the command
// line history, and the input histories of dialogs such as Find files
// ("Search for"), multi-rename masks and selection masks. Total Commander
// stores each one in its own section as numbered keys starting at 0, newest
// first: [LeftHistory] 0=c:\newest, 1=..., [Command line history] 0=...,
// [SearchName] 0=*.txt. A history is a most-recently-used list with a maximum
// size (F-662) in which re-using an entry moves it to the top instead of
// adding a copy; folders are compared with the platform's rules (case does not
// matter on Windows). Frequent keeps the "frequently used folders" list
// (F-661): a visit count per folder that slowly fades, so folders used a lot
// recently rank above folders used a lot long ago.

package config

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// TC section names of the histories.
const (
	SectionLeftHistory    = "LeftHistory"
	SectionRightHistory   = "RightHistory"
	SectionCmdLineHistory = "Command line history"
	SectionSearchName     = "SearchName"
	SectionSearchText     = "SearchText"
	SectionSearchIn       = "SearchIn"
	SectionSelection      = "Selection"
	SectionRenameTemplate = "RenameTemplates"
	SectionMkDirHistory   = "MkDirHistory"
	SectionFrequentDirs   = "FrequentDirs"
)

// DefaultHistorySize is TC's default number of kept entries.
const DefaultHistorySize = 30

// History is a most-recently-used list, newest first.
type History struct {
	Max      int  // maximum entries; <= 0 means DefaultHistorySize
	IsPath   bool // compare entries as paths (platform rules)
	FoldCase bool // compare non-path entries without regard to case
	items    []string
}

// NewHistory makes an empty history.
func NewHistory(max int, isPath bool) *History { return &History{Max: max, IsPath: isPath} }

func (h *History) max() int {
	if h.Max <= 0 {
		return DefaultHistorySize
	}
	return h.Max
}

func (h *History) same(a, b string) bool {
	switch {
	case h.IsPath:
		return pathsEqual(a, b)
	case h.FoldCase:
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Add puts s at the top, removing an older equal entry, and trims the list.
// Empty strings are ignored.
func (h *History) Add(s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	out := make([]string, 0, len(h.items)+1)
	out = append(out, s)
	for _, x := range h.items {
		if !h.same(x, s) {
			out = append(out, x)
		}
	}
	if len(out) > h.max() {
		out = out[:h.max()]
	}
	h.items = out
}

// Remove deletes all entries equal to s.
func (h *History) Remove(s string) bool {
	out := h.items[:0]
	removed := false
	for _, x := range h.items {
		if h.same(x, s) {
			removed = true
			continue
		}
		out = append(out, x)
	}
	h.items = out
	return removed
}

// Items returns a copy of the entries, newest first.
func (h *History) Items() []string { return append([]string(nil), h.items...) }

// Len returns the number of entries.
func (h *History) Len() int { return len(h.items) }

// Clear removes all entries (F-662).
func (h *History) Clear() { h.items = nil }

// SetMax changes the size limit and trims the list.
func (h *History) SetMax(n int) {
	h.Max = n
	if len(h.items) > h.max() {
		h.items = h.items[:h.max()]
	}
}

// Load reads numbered keys 0, 1, 2... from a section (stopping at the first
// gap), dropping duplicates and extra entries.
func (h *History) Load(f *File, section string) {
	m := f.Map(section)
	h.items = nil
	for i := 0; ; i++ {
		v, ok := m[strconv.Itoa(i)]
		if !ok {
			break
		}
		if strings.TrimSpace(v) == "" {
			continue
		}
		dup := false
		for _, x := range h.items {
			if h.same(x, v) {
				dup = true
				break
			}
		}
		if !dup && len(h.items) < h.max() {
			h.items = append(h.items, v)
		}
	}
}

// Save writes the entries as 0=..., 1=... into a section.
func (h *History) Save(f *File, section string) {
	kvs := make([]KV, len(h.items))
	for i, x := range h.items {
		kvs[i] = KV{strconv.Itoa(i), x}
	}
	f.ReplaceSection(section, kvs)
}

// Frequent counts folder visits for the "frequently used folders" list.
type Frequent struct {
	Max   int     // folders kept; <= 0 means 50
	Decay float64 // factor applied to all scores on every visit (0 < d <= 1); 0 means 0.98
	score map[string]float64
	names map[string]string // key -> spelling as last visited
}

// NewFrequent makes an empty list.
func NewFrequent() *Frequent { return &Frequent{} }

func (q *Frequent) key(p string) string {
	if pathsEqual("A", "a") {
		return strings.ToLower(strings.TrimRight(p, `/\`))
	}
	if len(p) > 1 {
		return strings.TrimRight(p, "/")
	}
	return p
}

// Visit records that folder was opened.
func (q *Frequent) Visit(folder string) {
	if folder == "" {
		return
	}
	if q.score == nil {
		q.score, q.names = map[string]float64{}, map[string]string{}
	}
	d := q.Decay
	if d <= 0 || d > 1 {
		d = 0.98
	}
	for k := range q.score {
		q.score[k] *= d
	}
	k := q.key(folder)
	q.score[k]++
	q.names[k] = folder
	max := q.Max
	if max <= 0 {
		max = 50
	}
	if len(q.score) > max {
		top := q.Top(max)
		keep := map[string]bool{}
		for _, t := range top {
			keep[q.key(t)] = true
		}
		for k := range q.score {
			if !keep[k] {
				delete(q.score, k)
				delete(q.names, k)
			}
		}
	}
}

// Top returns up to n folders, highest score first (ties by name).
func (q *Frequent) Top(n int) []string {
	type kv struct {
		k string
		s float64
	}
	var all []kv
	for k, s := range q.score {
		all = append(all, kv{k, s})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].s != all[j].s {
			return all[i].s > all[j].s
		}
		return all[i].k < all[j].k
	})
	var out []string
	for i := 0; i < len(all) && i < n; i++ {
		out = append(out, q.names[all[i].k])
	}
	return out
}

// Forget removes a folder.
func (q *Frequent) Forget(folder string) {
	k := q.key(folder)
	delete(q.score, k)
	delete(q.names, k)
}

// Clear removes everything.
func (q *Frequent) Clear() { q.score, q.names = nil, nil }

// Load reads "N=score|folder" entries.
func (q *Frequent) Load(f *File, section string) {
	q.Clear()
	for _, kv := range f.Entries(section) {
		sc, folder, ok := strings.Cut(kv.Value, "|")
		if !ok || folder == "" {
			continue
		}
		s, err := strconv.ParseFloat(sc, 64)
		if err != nil || math.IsNaN(s) || s < 0 {
			continue
		}
		if q.score == nil {
			q.score, q.names = map[string]float64{}, map[string]string{}
		}
		k := q.key(folder)
		q.score[k] += s
		q.names[k] = folder
	}
}

// Save writes the folders, best first.
func (q *Frequent) Save(f *File, section string) {
	var kvs []KV
	for i, folder := range q.Top(len(q.score)) {
		kvs = append(kvs, KV{strconv.Itoa(i), fmt.Sprintf("%.3f|%s", q.score[q.key(folder)], folder)})
	}
	f.ReplaceSection(section, kvs)
}
