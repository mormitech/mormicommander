// Next and previous file (N and P in the Lister). While viewing a file,
// Total Commander lets you step to the next or previous file of the same
// folder without going back to the panel. The UI normally knows the panel's
// own order and can use NeighborIn with that list; NeighborFile is the
// fallback when the Lister was opened on its own (from the command line or
// another app), and lists the folder itself in natural name order, files
// only, skipping hidden dot files unless the current file is one.

package lister

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// ErrNoNeighbor is returned when there is no next/previous file.
var ErrNoNeighbor = errors.New("lister: no further file in this folder")

// NeighborIn returns the entry delta positions away from current in list
// (delta 1 = next, -1 = previous). It does not wrap around.
func NeighborIn(list []string, current string, delta int) (string, error) {
	for i, p := range list {
		if p == current {
			j := i + delta
			if j < 0 || j >= len(list) {
				return "", ErrNoNeighbor
			}
			return list[j], nil
		}
	}
	return "", ErrNoNeighbor
}

// NeighborFile lists the folder of path and returns the next (delta > 0)
// or previous (delta < 0) regular file in natural name order.
func NeighborFile(path string, delta int) (string, error) {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	showHidden := strings.HasPrefix(base, ".")
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !showHidden && strings.HasPrefix(n, ".") && n != base {
			continue
		}
		if e.Type().IsRegular() || (e.Type()&os.ModeSymlink != 0 && isRegular(filepath.Join(dir, n))) {
			names = append(names, n)
		}
	}
	sort.Slice(names, func(i, j int) bool { return NaturalLess(names[i], names[j]) })
	n, err := NeighborIn(names, base, delta)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, n), nil
}

func isRegular(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// NaturalLess compares names case-insensitively with digit runs compared
// as numbers, so "file2" sorts before "file10".
func NaturalLess(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		ca, cb := ra[i], rb[j]
		if unicode.IsDigit(ca) && unicode.IsDigit(cb) {
			si := i
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			sj := j
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		la, lb := unicode.ToLower(ca), unicode.ToLower(cb)
		if la != lb {
			return la < lb
		}
		i++
		j++
	}
	if len(ra)-i != len(rb)-j {
		return len(ra)-i < len(rb)-j
	}
	return a < b
}
