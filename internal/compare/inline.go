// This file computes the intra-line difference shown inside a changed row:
// when a line was edited rather than replaced, Total Commander highlights
// only the words or characters that actually differ, so a one-letter typo fix
// does not light up the whole line. The two lines are split into tokens
// (words, whitespace runs and punctuation, or single characters), the token
// sequences are diffed with the same Myers code as the line diff, and the
// result is returned as byte-offset spans for each side that the UI can paint
// directly. Case and whitespace options are honoured, so with "ignore case"
// a change from "Foo" to "foo" is not highlighted. Tiny islands of unchanged
// text between two changes (a single space, a single character) are folded
// into the change, which reads much better than a confetti of fragments.
package compare

import (
	"unicode"
	"unicode/utf8"
)

// Span is a byte range [Start, End) of a line, marked as changed or not.
// The spans of one line are contiguous and cover it completely.
type Span struct {
	Start, End int
	Changed    bool
}

type token struct {
	start, end int
	space      bool
}

// InlineDiff returns the spans of a (left line) and b (right line).
func InlineDiff(a, b string, opts Options) (sa, sb []Span) {
	if opts.Inline == InlineNone {
		return wholeSpan(a, NormalizeLine(a, opts) != NormalizeLine(b, opts)),
			wholeSpan(b, NormalizeLine(a, opts) != NormalizeLine(b, opts))
	}
	ta := tokenize(a, opts.Inline)
	tb := tokenize(b, opts.Inline)
	in := newInterner()
	ka := tokenKeys(a, ta, opts, in)
	kb := tokenKeys(b, tb, opts, in)
	ca, cb := myersDiff(ka, kb, 0)
	ops := opsFromFlags(ca, cb)
	ops = absorbIslands(ops, a, ta, opts.Inline)
	return spansFromOps(ta, ops, true, opts), spansFromOps(tb, ops, false, opts)
}

func wholeSpan(s string, changed bool) []Span {
	if s == "" {
		return nil
	}
	return []Span{{Start: 0, End: len(s), Changed: changed}}
}

func tokenize(s string, mode InlineMode) []token {
	var out []token
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		start := i
		i += w
		switch {
		case mode == InlineChars:
			out = append(out, token{start, i, unicode.IsSpace(r)})
		case isWordRune(r):
			for i < len(s) {
				r2, w2 := utf8.DecodeRuneInString(s[i:])
				if !isWordRune(r2) {
					break
				}
				i += w2
			}
			out = append(out, token{start, i, false})
		case unicode.IsSpace(r):
			for i < len(s) {
				r2, w2 := utf8.DecodeRuneInString(s[i:])
				if !unicode.IsSpace(r2) {
					break
				}
				i += w2
			}
			out = append(out, token{start, i, true})
		default:
			out = append(out, token{start, i, false})
		}
	}
	return out
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r)
}

func tokenKeys(s string, toks []token, opts Options, in *interner) []int {
	keys := make([]int, len(toks))
	for i, t := range toks {
		txt := s[t.start:t.end]
		if t.space && opts.Whitespace != WhitespaceExact {
			txt = " "
		} else if opts.IgnoreCase {
			txt = foldCase(txt)
		}
		keys[i] = in.id(txt)
	}
	return keys
}

// absorbIslands merges an equal run that sits between two differences into
// one difference when it is too small to be meaningful: whitespace only, or
// (in character mode) a single character.
func absorbIslands(ops []Op, a string, ta []token, mode InlineMode) []Op {
	if len(ops) < 3 {
		return ops
	}
	small := func(op Op) bool {
		if op.A2-op.A1 == 0 {
			return false
		}
		allSpace := true
		for _, t := range ta[op.A1:op.A2] {
			if !t.space {
				allSpace = false
				break
			}
		}
		if allSpace {
			return true
		}
		if mode == InlineChars && op.A2-op.A1 == 1 {
			return true
		}
		return false
	}
	out := make([]Op, 0, len(ops))
	for _, op := range ops {
		if op.IsDiff() && len(out) >= 2 {
			last := out[len(out)-1]
			prev := out[len(out)-2]
			if last.Kind == OpEqual && prev.IsDiff() && small(last) {
				out = out[:len(out)-2]
				out = append(out, newDiffOp(prev.A1, op.A2, prev.B1, op.B2))
				continue
			}
		}
		out = append(out, op)
	}
	return out
}

func spansFromOps(toks []token, ops []Op, left bool, opts Options) []Span {
	var spans []Span
	add := func(start, end int, changed bool) {
		if start >= end {
			return
		}
		if n := len(spans); n > 0 && spans[n-1].Changed == changed && spans[n-1].End == start {
			spans[n-1].End = end
			return
		}
		spans = append(spans, Span{Start: start, End: end, Changed: changed})
	}
	for _, op := range ops {
		lo, hi := op.B1, op.B2
		if left {
			lo, hi = op.A1, op.A2
		}
		for _, t := range toks[lo:hi] {
			changed := op.IsDiff()
			if changed && t.space && opts.Whitespace != WhitespaceExact {
				changed = false
			}
			add(t.start, t.end, changed)
		}
	}
	return spans
}
