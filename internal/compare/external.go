// This file covers two small helpers around the compare tool. Total
// Commander lets the user replace the built-in comparison with an external
// program (WinMerge, Beyond Compare, meld, FileMerge's opendiff and so on),
// configured as a command line in which %1 and %2 stand for the two files.
// ExternalCommand turns such a template into an argument vector ready for
// exec.Command, quoting-aware and without going through a shell, so file
// names with spaces or shell metacharacters are passed safely. PairByName
// implements "compare files with the same name": given the selected names in
// the two panels it decides which left and right files belong together, so
// the command can open one comparison per pair.
package compare

import (
	"errors"
	"sort"
	"strings"
)

// ExternalCommand splits a command template into arguments and substitutes
// %1 (left file) and %2 (right file); "%%" stands for a literal percent sign.
// Double quotes group words ("C:\Program Files\WinMerge\WinMergeU.exe" %1
// %2). If the template mentions neither %1 nor %2, the two paths are appended
// as the last arguments.
func ExternalCommand(template, left, right string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inQuote, have, used := false, false, false
	for i := 0; i < len(template); i++ {
		c := template[i]
		switch {
		case c == '"':
			inQuote = !inQuote
			have = true
		case !inQuote && (c == ' ' || c == '\t'):
			if have {
				args = append(args, cur.String())
				cur.Reset()
				have = false
			}
		case c == '%' && i+1 < len(template):
			switch template[i+1] {
			case '1':
				cur.WriteString(left)
				used = true
				i++
			case '2':
				cur.WriteString(right)
				used = true
				i++
			case '%':
				cur.WriteByte('%')
				i++
			default:
				cur.WriteByte(c)
			}
			have = true
		default:
			cur.WriteByte(c)
			have = true
		}
	}
	if inQuote {
		return nil, errors.New("compare: unbalanced quote in external command")
	}
	if have {
		args = append(args, cur.String())
	}
	if len(args) == 0 {
		return nil, errors.New("compare: empty external command")
	}
	if !used {
		args = append(args, left, right)
	}
	return args, nil
}

// Pair is a left/right file pair to compare.
type Pair struct {
	Left, Right string
}

// PairByName matches names (base names or relative paths) from the left and
// right panel. Exact matches come first; when caseInsensitive is set (the
// default on Windows and macOS file systems) names differing only in case
// also match. The result is sorted by the left name. Names without a partner
// are returned in onlyLeft / onlyRight.
func PairByName(left, right []string, caseInsensitive bool) (pairs []Pair, onlyLeft, onlyRight []string) {
	usedR := make(map[int]bool)
	exact := make(map[string]int, len(right))
	folded := make(map[string][]int, len(right))
	for i, r := range right {
		if _, ok := exact[r]; !ok {
			exact[r] = i
		}
		k := strings.ToLower(r)
		folded[k] = append(folded[k], i)
	}
	var pending []string
	for _, l := range left {
		if i, ok := exact[l]; ok && !usedR[i] {
			usedR[i] = true
			pairs = append(pairs, Pair{l, right[i]})
			continue
		}
		pending = append(pending, l)
	}
	for _, l := range pending {
		matched := false
		if caseInsensitive {
			for _, i := range folded[strings.ToLower(l)] {
				if !usedR[i] {
					usedR[i] = true
					pairs = append(pairs, Pair{l, right[i]})
					matched = true
					break
				}
			}
		}
		if !matched {
			onlyLeft = append(onlyLeft, l)
		}
	}
	for i, r := range right {
		if !usedR[i] {
			onlyRight = append(onlyRight, r)
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Left < pairs[j].Left })
	return pairs, onlyLeft, onlyRight
}
