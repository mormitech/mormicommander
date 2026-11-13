// This file implements the "Remove accents" option, an addition of ours that
// Total Commander does not have. Hungarian users in particular often need to
// turn 'árvíztűrő tükörfúrógép' into plain 'arvizturo tukorfurogep' before
// copying files to systems or web servers that mangle non-ASCII names. We
// map precomposed Latin letters to their base letters with a table (so the
// standard library is enough and no Unicode normalisation package is pulled
// in) and drop combining marks, which covers names that macOS stores in
// decomposed form. Letters that are not accented variants of a Latin base,
// such as Greek or Cyrillic, are left untouched.

package multirename

import (
	"strings"
	"unicode"
)

// accentGroups lists, for each replacement, the characters that turn into
// it. Multi-letter replacements handle ligatures and letters like ß.
var accentGroups = map[string]string{
	"a":  "àáâãäåāăąǎǟǡǻȁȃȧạảấầẩẫậắằẳẵặ",
	"A":  "ÀÁÂÃÄÅĀĂĄǍǞǠǺȀȂȦẠẢẤẦẨẪẬẮẰẲẴẶ",
	"c":  "çćĉċč",
	"C":  "ÇĆĈĊČ",
	"d":  "ďđḍḏ",
	"D":  "ĎĐḌḎ",
	"e":  "èéêëēĕėęěȅȇȩẹẻẽếềểễệ",
	"E":  "ÈÉÊËĒĔĖĘĚȄȆȨẸẺẼẾỀỂỄỆ",
	"g":  "ĝğġģǧǵ",
	"G":  "ĜĞĠĢǦǴ",
	"h":  "ĥħḥ",
	"H":  "ĤĦḤ",
	"i":  "ìíîïĩīĭįıǐȉȋịỉ",
	"I":  "ÌÍÎÏĨĪĬĮİǏȈȊỊỈ",
	"j":  "ĵǰ",
	"J":  "Ĵ",
	"k":  "ķǩ",
	"K":  "ĶǨ",
	"l":  "ĺļľŀł",
	"L":  "ĹĻĽĿŁ",
	"n":  "ñńņňŉǹṇ",
	"N":  "ÑŃŅŇǸṆ",
	"o":  "òóôõöøōŏőơǒǫǭǿȍȏȫȭȯȱọỏốồổỗộớờởỡợ",
	"O":  "ÒÓÔÕÖØŌŎŐƠǑǪǬǾȌȎȪȬȮȰỌỎỐỒỔỖỘỚỜỞỠỢ",
	"r":  "ŕŗřȑȓṛ",
	"R":  "ŔŖŘȐȒṚ",
	"s":  "śŝşšșṣ",
	"S":  "ŚŜŞŠȘṢ",
	"t":  "ţťŧțṭ",
	"T":  "ŢŤŦȚṬ",
	"u":  "ùúûüũūŭůűųưǔǖǘǚǜȕȗụủứừửữự",
	"U":  "ÙÚÛÜŨŪŬŮŰŲƯǓǕǗǙǛȔȖỤỦỨỪỬỮỰ",
	"w":  "ŵẁẃẅ",
	"W":  "ŴẀẂẄ",
	"y":  "ýÿŷȳỳỵỷỹ",
	"Y":  "ÝŸŶȲỲỴỶỸ",
	"z":  "źżžẓ",
	"Z":  "ŹŻŽẒ",
	"ss": "ß",
	"SS": "ẞ",
	"ae": "æǣǽ",
	"AE": "ÆǢǼ",
	"oe": "œ",
	"OE": "Œ",
	"th": "þ",
	"TH": "Þ",
	"dh": "ð",
	"DH": "Ð",
	"ij": "ĳ",
	"IJ": "Ĳ",
}

var accentMap = func() map[rune]string {
	m := make(map[rune]string)
	for base, chars := range accentGroups {
		for _, r := range chars {
			m[r] = base
		}
	}
	return m
}()

// RemoveAccents replaces accented Latin letters with their plain base letters
// and drops combining marks.
func RemoveAccents(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if rep, ok := accentMap[r]; ok {
			b.WriteString(rep)
			continue
		}
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
