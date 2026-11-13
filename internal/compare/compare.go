// Package compare is the engine behind mormiCommander's "Compare by content"
// tool, the counterpart of Total Commander's file comparison window (Files >
// Compare by content). The user selects two files, one in each panel, and the
// tool shows them side by side with the differing lines highlighted, the
// differing words or characters inside those lines marked, and buttons to
// jump to the next or previous difference. Lines can be edited, whole blocks
// can be copied from one side to the other, and the result can be saved back
// in the file's original encoding and line endings. Files that are not text
// are compared byte by byte and the first differences are reported with their
// offsets, which is what TC's binary mode does.
//
// This file holds the comparison options that every part of the engine
// shares, and the normalisation that turns a line into the key the line
// differ actually compares, so that "ignore case" and "ignore whitespace"
// behave identically in the line diff, the intra-line diff and the hunk
// output. The algorithms live in the sibling files: myers.go and patience.go
// (sequence diff), diff.go (lines to edit operations, resync, repeated-line
// folding), rows.go (side-by-side rows and difference navigation), inline.go
// (word/character diff), unified.go (hunks and unified diff text),
// binary.go, encoding.go, document.go and session.go (editing and saving).
package compare

import (
	"strings"
	"unicode"
)

// Whitespace selects how differences in blanks and tabs are treated.
type Whitespace int

const (
	// WhitespaceExact compares lines byte for byte (after case folding when
	// IgnoreCase is set).
	WhitespaceExact Whitespace = iota
	// WhitespaceIgnoreChange treats any run of whitespace as a single space
	// and ignores whitespace at the start and end of a line, like
	// "diff -b" plus trailing-blank trimming. Re-indented code compares equal.
	WhitespaceIgnoreChange
	// WhitespaceIgnoreAll removes every whitespace character before
	// comparing, like "diff -w". "a = b" and "a=b" compare equal.
	WhitespaceIgnoreAll
)

// Algorithm selects the line diff algorithm.
type Algorithm int

const (
	// AlgorithmPatience anchors the diff on lines that occur exactly once in
	// both files and runs Myers between the anchors. It produces the most
	// readable result for source code (functions are not interleaved) and is
	// the default.
	AlgorithmPatience Algorithm = iota
	// AlgorithmMyers is the classic minimal O(ND) diff in linear space.
	AlgorithmMyers
)

// InlineMode selects the granularity of the intra-line diff.
type InlineMode int

const (
	// InlineWords marks whole words (letter/digit runs), whitespace runs and
	// single punctuation characters. This is the default and matches how TC
	// highlights changed parts of a line.
	InlineWords InlineMode = iota
	// InlineChars marks individual characters (runes).
	InlineChars
	// InlineNone disables the intra-line diff: a changed line is marked as a
	// whole.
	InlineNone
)

// Options controls a text comparison. The zero value is a sensible default:
// exact comparison, patience algorithm, word-level inline highlighting and
// resynchronisation after a single matching line.
type Options struct {
	// IgnoreCase compares lines case-insensitively (Unicode aware, so "Ő"
	// equals "ő").
	IgnoreCase bool
	// Whitespace selects how blanks and tabs are compared.
	Whitespace Whitespace
	// IgnoreRepeatedLines treats a run of identical consecutive lines as a
	// single line, so a block that appears three times on one side and twice
	// on the other is not reported as a difference. This mirrors TC's
	// "Ignore repeated lines" option and is mainly useful for log files.
	IgnoreRepeatedLines bool
	// ResyncLines is the number of consecutive matching lines needed before
	// the comparison is considered back in sync after a difference (TC's
	// "lines to resync" setting). Shorter runs of matching lines that sit
	// between two differences are folded into one larger difference block,
	// which keeps a heavily edited paragraph together instead of splitting it
	// into many tiny blocks. Values below 1 mean 1 (no folding).
	ResyncLines int
	// Algorithm selects the line diff algorithm.
	Algorithm Algorithm
	// Inline selects the granularity of intra-line highlighting.
	Inline InlineMode
	// MaxCost bounds the work the Myers search may do on one region before
	// it settles for a valid but possibly non-minimal diff, so that two huge
	// and completely different files cannot hang the UI. Zero picks an
	// automatic limit based on the input size; a negative value means no
	// limit.
	MaxCost int
}

func (o Options) resync() int {
	if o.ResyncLines < 1 {
		return 1
	}
	return o.ResyncLines
}

// NormalizeLine returns the key under which a line is compared with the
// given options. Two lines are considered equal exactly when their keys are
// equal. Exposed so that callers (for example folder sync by content) can
// apply the same rules.
func NormalizeLine(s string, o Options) string {
	switch o.Whitespace {
	case WhitespaceIgnoreChange:
		s = collapseSpace(s)
	case WhitespaceIgnoreAll:
		s = removeSpace(s)
	}
	if o.IgnoreCase {
		s = foldCase(s)
	}
	return s
}

// foldCase lower-cases s with simple Unicode case folding. strings.ToLower
// is not enough on its own for characters such as the Kelvin sign, so each
// rune is mapped to the smallest member of its folding orbit, lowered.
func foldCase(s string) string {
	return strings.Map(func(r rune) rune {
		return unicode.ToLower(unicode.ToUpper(r))
	}, s)
}

func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			inSpace = true
			continue
		}
		if inSpace && b.Len() > 0 {
			b.WriteByte(' ')
		}
		inSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

func removeSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// interner maps strings to small integers so that the sequence algorithms
// compare ints instead of strings.
type interner struct {
	ids map[string]int
}

func newInterner() *interner { return &interner{ids: make(map[string]int)} }

func (in *interner) id(s string) int {
	if v, ok := in.ids[s]; ok {
		return v
	}
	v := len(in.ids)
	in.ids[s] = v
	return v
}
