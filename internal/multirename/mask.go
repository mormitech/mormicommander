// This file parses and renders rename masks. A mask is ordinary text mixed
// with bracketed placeholders, exactly as in Total Commander: [N] is the old
// name without extension, [E] the extension, [P] and [G] the parent and
// grandparent folder names, [C] the counter, [Y] [y] [M] [D] [h] [m] [s] [d]
// [t] the parts of the modification date and time, [U] [L] [F] [n] switch the
// case of everything that follows, [=field] inserts a named field such as the
// file size, and [[] and []] produce literal brackets. Name-like placeholders
// accept a character range: [N1] is the first character, [N2-5] characters 2
// to 5, [N2,3] three characters starting at 2, [N2-] everything from 2 on,
// [N-8,5] five characters starting at the 8th from the end, [N-8-5] or
// [N-8--5] the 8th-last to the 5th-last, [N2--5] from 2 to the 5th-last and
// [N-5-] the last five. The mask is compiled once into a token list so the
// live preview can re-render hundreds of names cheaply on every keystroke.

package multirename

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// tokenKind says what a single piece of a compiled mask stands for.
type tokenKind int

const (
	tLiteral tokenKind = iota
	tName
	tExt
	tParent
	tGrand
	tCounter
	tYear4
	tYear2
	tMonth
	tDay
	tHour
	tMinute
	tSecond
	tDate
	tTime
	tCaseUpper
	tCaseLower
	tCaseWords
	tCaseKeep
	tField
)

// rangeMode tells how the end of a character range is given.
type rangeMode int

const (
	rangeSingle rangeMode = iota // [N3]: just one character
	rangeToEnd                   // [N3-]: up to the last character
	rangeEnd                     // [N3-5] or [N3--2]: up to an explicit position
	rangeCount                   // [N3,4]: a number of characters
)

// charRange is a parsed character range. Positions are 1-based like in Total
// Commander; a negative position counts from the end, so -1 is the last
// character.
type charRange struct {
	start int
	mode  rangeMode
	end   int
	count int
}

// apply cuts the range out of s, counting in characters (runes) rather than
// bytes so accented names behave as the user expects. Positions outside the
// string are clamped instead of failing, which matches how the preview in
// Total Commander simply shows less text for short names.
func (r charRange) apply(s string) string {
	rs := []rune(s)
	n := len(rs)
	pos := func(k int) int {
		if k > 0 {
			return k - 1
		}
		if k == 0 {
			return 0
		}
		return n + k
	}
	st := pos(r.start)
	if st < 0 {
		st = 0
	}
	var en int
	switch r.mode {
	case rangeSingle:
		en = st + 1
	case rangeToEnd:
		en = n
	case rangeEnd:
		en = pos(r.end) + 1
	case rangeCount:
		en = st + r.count
	}
	if st > n {
		st = n
	}
	if en > n {
		en = n
	}
	if en < st {
		en = st
	}
	return string(rs[st:en])
}

// counterSpec holds the inline overrides of a [C...] placeholder. A nil
// pointer means "use the value from the dialog's counter fields".
type counterSpec struct {
	start  *int
	alpha  bool // the start was given as letters, e.g. [Ca]
	upper  bool // ...and in upper case, e.g. [CA]
	step   *int
	digits *int
	auto   bool // [C:a]: width taken from the largest value
}

// token is one compiled piece of a mask.
type token struct {
	kind  tokenKind
	text  string     // literal text, or the field name for tField
	rng   *charRange // optional range for name-like placeholders
	ctr   *counterSpec
	field string
}

// Mask is a compiled rename mask.
type Mask struct {
	src    string
	tokens []token
}

// String returns the mask text the Mask was compiled from.
func (m *Mask) String() string { return m.src }

// MaskError reports a problem in a mask, with the byte offset of the
// offending placeholder so the dialog can point at it.
type MaskError struct {
	Mask   string
	Offset int
	Msg    string
}

func (e *MaskError) Error() string {
	return fmt.Sprintf("mask %q at position %d: %s", e.Mask, e.Offset+1, e.Msg)
}

// CompileMask parses a mask. An empty mask is valid and produces an empty
// string, which is how Total Commander behaves when the extension field is
// cleared to drop all extensions.
func CompileMask(src string) (*Mask, error) {
	m := &Mask{src: src}
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			m.tokens = append(m.tokens, token{kind: tLiteral, text: lit.String()})
			lit.Reset()
		}
	}
	i := 0
	for i < len(src) {
		c := src[i]
		if c != '[' {
			lit.WriteByte(c)
			i++
			continue
		}
		if strings.HasPrefix(src[i:], "[[]") {
			lit.WriteByte('[')
			i += 3
			continue
		}
		if strings.HasPrefix(src[i:], "[]]") {
			lit.WriteByte(']')
			i += 3
			continue
		}
		end := strings.IndexByte(src[i+1:], ']')
		if end < 0 {
			return nil, &MaskError{Mask: src, Offset: i, Msg: "missing closing bracket"}
		}
		body := src[i+1 : i+1+end]
		tok, err := parsePlaceholder(body)
		if err != nil {
			return nil, &MaskError{Mask: src, Offset: i, Msg: err.Error()}
		}
		flush()
		m.tokens = append(m.tokens, tok)
		i += end + 2
	}
	flush()
	return m, nil
}

// parsePlaceholder turns the text between brackets into a token.
func parsePlaceholder(body string) (token, error) {
	if body == "" {
		return token{}, fmt.Errorf("empty placeholder []")
	}
	code, rest := body[0], body[1:]
	switch code {
	case 'N', 'E', 'P', 'G':
		kind := map[byte]tokenKind{'N': tName, 'E': tExt, 'P': tParent, 'G': tGrand}[code]
		if rest == "" {
			return token{kind: kind}, nil
		}
		r, err := parseRange(rest)
		if err != nil {
			return token{}, err
		}
		return token{kind: kind, rng: &r}, nil
	case 'C':
		cs, err := parseCounter(rest)
		if err != nil {
			return token{}, err
		}
		return token{kind: tCounter, ctr: cs}, nil
	case '=':
		name := strings.TrimSpace(rest)
		if name == "" {
			return token{}, fmt.Errorf("field name missing after [=")
		}
		return token{kind: tField, field: name}, nil
	}
	simple := map[string]tokenKind{
		"Y": tYear4, "y": tYear2, "M": tMonth, "D": tDay,
		"h": tHour, "m": tMinute, "s": tSecond, "d": tDate, "t": tTime,
		"U": tCaseUpper, "L": tCaseLower, "F": tCaseWords, "n": tCaseKeep,
	}
	if k, ok := simple[body]; ok {
		return token{kind: k}, nil
	}
	return token{}, fmt.Errorf("unknown placeholder [%s]", body)
}

// parseSigned reads an optionally negative decimal number from the start of
// s and returns it with the remaining text.
func parseSigned(s string) (int, string, bool) {
	j := 0
	if j < len(s) && s[j] == '-' {
		j++
	}
	k := j
	for k < len(s) && s[k] >= '0' && s[k] <= '9' {
		k++
	}
	if k == j {
		return 0, s, false
	}
	v, err := strconv.Atoi(s[:k])
	if err != nil {
		return 0, s, false
	}
	return v, s[k:], true
}

// parseRange parses the part of [N...] after the letter.
func parseRange(spec string) (charRange, error) {
	start, rest, ok := parseSigned(spec)
	if !ok {
		return charRange{}, fmt.Errorf("bad character range %q", spec)
	}
	r := charRange{start: start}
	switch {
	case rest == "":
		r.mode = rangeSingle
	case rest == "-":
		r.mode = rangeToEnd
	case rest[0] == '-':
		end, tail, ok := parseSigned(rest[1:])
		if !ok || tail != "" {
			return charRange{}, fmt.Errorf("bad character range %q", spec)
		}
		// [N-8-5] means "8th-last to 5th-last": when the start already
		// counts from the end, an unsigned end does too.
		if start < 0 && end > 0 {
			end = -end
		}
		r.mode, r.end = rangeEnd, end
	case rest[0] == ',':
		cnt, tail, ok := parseSigned(rest[1:])
		if !ok || tail != "" || cnt < 0 {
			return charRange{}, fmt.Errorf("bad character count in %q", spec)
		}
		r.mode, r.count = rangeCount, cnt
	default:
		return charRange{}, fmt.Errorf("bad character range %q", spec)
	}
	return r, nil
}

// parseCounter parses the part of [C...] after the letter, for example
// "10+5:3" (start 10, step 5, three digits), "+2", ":4", "a" (letters) or
// ":a" (automatic width).
func parseCounter(spec string) (*counterSpec, error) {
	cs := &counterSpec{}
	s := spec
	if s != "" && s[0] != '+' && s[0] != ':' {
		if r, _ := utf8.DecodeRuneInString(s); unicode.IsLetter(r) {
			j := 0
			for j < len(s) && isASCIILetter(s[j]) {
				j++
			}
			word := s[:j]
			if word != strings.ToLower(word) && word != strings.ToUpper(word) {
				return nil, fmt.Errorf("mixed-case letter counter %q", word)
			}
			v := alphaToNum(strings.ToLower(word))
			if v == 0 {
				return nil, fmt.Errorf("bad letter counter %q", spec)
			}
			cs.start, cs.alpha, cs.upper = &v, true, word == strings.ToUpper(word)
			s = s[j:]
		} else {
			v, rest, ok := parseSigned(s)
			if !ok {
				return nil, fmt.Errorf("bad counter %q", spec)
			}
			cs.start = &v
			s = rest
		}
	}
	if strings.HasPrefix(s, "+") {
		v, rest, ok := parseSigned(s[1:])
		if !ok {
			return nil, fmt.Errorf("bad counter step in %q", spec)
		}
		cs.step = &v
		s = rest
	}
	if strings.HasPrefix(s, ":") {
		if s[1:] == "a" || s[1:] == "A" {
			cs.auto = true
			s = ""
		} else {
			v, rest, ok := parseSigned(s[1:])
			if !ok || v < 0 {
				return nil, fmt.Errorf("bad counter width in %q", spec)
			}
			cs.digits = &v
			s = rest
		}
	}
	if s != "" {
		return nil, fmt.Errorf("bad counter %q", spec)
	}
	return cs, nil
}

func isASCIILetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// caseMode is the inline case state switched by [U], [L], [F] and [n].
type caseMode int

const (
	inlineKeep caseMode = iota
	inlineUpper
	inlineLower
	inlineWords
)

// segment is a rendered piece of text together with the inline case mode
// that was active when it was produced.
type segment struct {
	text string
	mode caseMode
}

// render produces the text for one item. The warnings slice collects soft
// problems such as an unknown [=field], which render as empty text rather
// than stopping the whole preview.
func (m *Mask) render(ctx *renderCtx) (string, []string) {
	var segs []segment
	var warns []string
	mode := inlineKeep
	for _, t := range m.tokens {
		var s string
		switch t.kind {
		case tLiteral:
			s = t.text
		case tName:
			s = ctx.name
		case tExt:
			s = ctx.ext
		case tParent:
			s = ctx.parent
		case tGrand:
			s = ctx.grand
		case tCounter:
			s = ctx.counter(t.ctr)
		case tYear4:
			s = fmt.Sprintf("%04d", ctx.item.ModTime.Year())
		case tYear2:
			s = fmt.Sprintf("%02d", ctx.item.ModTime.Year()%100)
		case tMonth:
			s = fmt.Sprintf("%02d", int(ctx.item.ModTime.Month()))
		case tDay:
			s = fmt.Sprintf("%02d", ctx.item.ModTime.Day())
		case tHour:
			s = fmt.Sprintf("%02d", ctx.item.ModTime.Hour())
		case tMinute:
			s = fmt.Sprintf("%02d", ctx.item.ModTime.Minute())
		case tSecond:
			s = fmt.Sprintf("%02d", ctx.item.ModTime.Second())
		case tDate:
			s = ctx.item.ModTime.Format(ctx.dateFormat())
		case tTime:
			s = ctx.item.ModTime.Format(ctx.timeFormat())
		case tCaseUpper:
			mode = inlineUpper
			continue
		case tCaseLower:
			mode = inlineLower
			continue
		case tCaseWords:
			mode = inlineWords
			continue
		case tCaseKeep:
			mode = inlineKeep
			continue
		case tField:
			v, ok := lookupField(t.field, ctx)
			if !ok {
				warns = append(warns, fmt.Sprintf("unknown field [=%s]", t.field))
			}
			s = v
		}
		if t.rng != nil {
			s = t.rng.apply(s)
		}
		segs = append(segs, segment{text: s, mode: mode})
	}
	return joinSegments(segs), warns
}

// joinSegments applies each segment's case mode and concatenates them. The
// last rune of the text so far is passed along so that [F] knows whether a
// segment starts in the middle of a word.
func joinSegments(segs []segment) string {
	var b strings.Builder
	var prev rune = ' '
	for _, sg := range segs {
		var out string
		switch sg.mode {
		case inlineUpper:
			out = strings.ToUpper(sg.text)
		case inlineLower:
			out = strings.ToLower(sg.text)
		case inlineWords:
			out = titleWords(sg.text, prev)
		default:
			out = sg.text
		}
		b.WriteString(out)
		if r, size := utf8.DecodeLastRuneInString(out); size > 0 {
			prev = r
		}
	}
	return b.String()
}
