// This file implements the panel quick filter (Ctrl+S, and Ctrl+Shift+S to reuse the last text). While the user
// types, the panel shows only the entries that match. Total Commander's quick search options decide how plain text
// is read: "match beginning" treats "rep" as "rep*", "any part" treats it as "*rep*". When the text already contains
// wildcards ("*" or "?") or a set, the same rule wraps it, so "r*t" in beginning mode means "r*t*". The ".." entry
// always stays visible so the user can still leave the folder, and folders can optionally stay visible as well so
// navigation keeps working while a filter is on.

package masks

import "strings"

// QuickFilterOptions mirror Configuration > Quick search.
type QuickFilterOptions struct {
	Case        CaseRule
	AnyPart     bool // match anywhere in the name instead of only at the beginning
	KeepFolders bool // folders are always shown, only files are filtered
}

// QuickFilter is a compiled quick-filter text. The zero value and an empty text match everything.
type QuickFilter struct {
	text  string
	opt   QuickFilterOptions
	masks []*Mask
}

// NewQuickFilter compiles text. Several words separated by ";" are alternatives, like a mask list; spaces are part of
// the text, because people filter for "my doc" more often than they want two masks.
func NewQuickFilter(text string, opt QuickFilterOptions) *QuickFilter {
	q := &QuickFilter{text: text, opt: opt}
	cs := opt.Case.Sensitive()
	for _, w := range strings.Split(text, ";") {
		if w == "" {
			continue
		}
		p := w
		if !strings.HasSuffix(p, "*") {
			p += "*"
		}
		if opt.AnyPart && !strings.HasPrefix(p, "*") {
			p = "*" + p
		}
		m := CompileMask(p, cs)
		m.dirOnly = false // a trailing slash in a quick filter is just a character the user typed
		q.masks = append(q.masks, m)
	}
	return q
}

// Text returns the filter text as typed, so the panel can show it and Ctrl+Shift+S can reuse it.
func (q *QuickFilter) Text() string {
	if q == nil {
		return ""
	}
	return q.text
}

// Active says whether the filter hides anything at all.
func (q *QuickFilter) Active() bool { return q != nil && len(q.masks) > 0 }

// Match says whether an entry stays visible.
func (q *QuickFilter) Match(name string, isDir bool) bool {
	if !q.Active() || name == ".." || (isDir && q.opt.KeepFolders) {
		return true
	}
	for _, m := range q.masks {
		if m.Match(name) {
			return true
		}
	}
	return false
}

// Filter returns the indexes of the entries that stay visible, in order.
func (q *QuickFilter) Filter(entries []Entry) []int {
	out := make([]int, 0, len(entries))
	for i, e := range entries {
		if q.Match(e.Name, e.IsDir) {
			out = append(out, i)
		}
	}
	return out
}
