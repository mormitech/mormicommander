package main

// Quick search and quick filter in a panel (F-066..F-070, F-068, F-780): the page sends the names of a panel once
// (QuickLoad) and then asks, keystroke by keystroke, which of them match the typed text (QuickMatch). Matching is
// internal/masks' quick filter; this file adds Total Commander's quick search options on top: "match the beginning"
// (rep = rep*), the extension rule (te.tx finds Test.txt unless "match the end" is on), wildcards on or off, accents
// ignored (á matches a), and folders only / files only.

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"mormicommander/internal/masks"
)

// FindQuickOptions are the quick search options (Configuration > Quick search).
type FindQuickOptions struct {
	Filter        bool   `json:"filter"`        // quick filter: the text may match anywhere and folders count too
	Beginning     bool   `json:"beginning"`     // the name must begin with the text
	MatchEnd      bool   `json:"matchEnd"`      // "te.tx" needs the name "te" exactly, not "te*"
	Wildcards     bool   `json:"wildcards"`     // * and ? are wildcards
	IgnoreAccents bool   `json:"ignoreAccents"` // á matches a
	Kind          string `json:"kind"`          // quick search: "" (files and folders), "dirs" or "files"
	FilterDirs    bool   `json:"filterDirs"`    // quick filter: folders are filtered too (else they always stay)
}

// findQuickList is the names of one panel. A folder's name ends in "/".
type findQuickList struct {
	names  []string
	dirs   []bool
	folded []string // the names without accents, made when first needed
}

// QuickLoad keeps the names of a panel for QuickMatch (key: the panel, e.g. "left"). Folder names end in "/".
func (f *Find) QuickLoad(key string, names []string) {
	l := &findQuickList{names: make([]string, len(names)), dirs: make([]bool, len(names))}
	for i, n := range names {
		if strings.HasSuffix(n, "/") && n != "/" {
			l.names[i], l.dirs[i] = strings.TrimSuffix(n, "/"), true
		} else {
			l.names[i] = n
		}
	}
	f.mu.Lock()
	f.quick[key] = l
	f.mu.Unlock()
}

// QuickMatch lists the indexes of the loaded names that match the text.
func (f *Find) QuickMatch(key, text string, o FindQuickOptions) []int {
	f.mu.Lock()
	l := f.quick[key]
	if l != nil && o.IgnoreAccents && l.folded == nil {
		l.folded = make([]string, len(l.names))
		for i, n := range l.names {
			l.folded[i] = findFold(n)
		}
	}
	f.mu.Unlock()
	out := []int{}
	if l == nil {
		return out
	}
	q := masks.NewQuickFilter(findQuickPattern(text, o), masks.QuickFilterOptions{
		Case: masks.CaseInsensitive, AnyPart: o.Filter || !o.Beginning, KeepFolders: o.Filter && !o.FilterDirs,
	})
	for i, n := range l.names {
		if n == ".." {
			continue
		}
		if !o.Filter && (o.Kind == "dirs" && !l.dirs[i] || o.Kind == "files" && l.dirs[i]) {
			continue
		}
		if o.IgnoreAccents {
			n = l.folded[i]
		}
		if q.Match(n, l.dirs[i]) {
			out = append(out, i)
		}
	}
	return out
}

// findQuickPattern turns typed text into a quick filter text.
func findQuickPattern(text string, o FindQuickOptions) string {
	if o.IgnoreAccents {
		text = findFold(text)
	}
	wild := strings.ContainsAny(text, "*?[")
	if !o.Wildcards && wild {
		r := strings.NewReplacer("[", "[[]", "*", "[*]", "?", "[?]")
		text = r.Replace(text)
		wild = false
	}
	// the extension rule: "te.tx" is "te*.tx" (the name part and the extension part each match their beginning)
	if !o.Filter && !o.MatchEnd && !wild {
		if i := strings.IndexByte(text, '.'); i > 0 {
			text = text[:i] + "*" + text[i:]
		}
	}
	return text
}

// findFold drops accents: "Árvíztűrő" → "Arvizturo".
func findFold(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return norm.NFC.String(b.String())
}
