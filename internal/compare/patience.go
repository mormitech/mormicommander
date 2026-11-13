// This file implements the patience diff algorithm (Bram Cohen's, as used by
// Bazaar and "git diff --patience"). Plain Myers finds a shortest edit
// script, but for source code the shortest script often matches up unrelated
// lines such as lone braces or blank lines and so interleaves two different
// functions. Patience first looks for lines that occur exactly once in each
// file, takes the longest increasing run of such lines as fixed anchors, and
// only then diffs the gaps between the anchors, recursively. Gaps without any
// unique line fall back to Myers. The output uses the same changed-flag
// representation as myers.go.
package compare

import "sort"

// patienceDiff marks the elements of a and b that are not matched.
func patienceDiff(a, b []int, maxCost int) (ca, cb []bool) {
	ctx := newDiffCtx(a, b, maxCost)
	ctx.patience(0, len(a), 0, len(b), 0)
	return ctx.ca, ctx.cb
}

// patience diffs a[aLo:aHi] against b[bLo:bHi].
func (c *diffCtx) patience(aLo, aHi, bLo, bHi, depth int) {
	for aLo < aHi && bLo < bHi && c.a[aLo] == c.b[bLo] {
		aLo++
		bLo++
	}
	for aLo < aHi && bLo < bHi && c.a[aHi-1] == c.b[bHi-1] {
		aHi--
		bHi--
	}
	if aLo == aHi || bLo == bHi || depth > 64 {
		c.compare(aLo, aHi, bLo, bHi)
		return
	}
	anchors := c.uniqueLCS(aLo, aHi, bLo, bHi)
	if len(anchors) == 0 {
		c.compare(aLo, aHi, bLo, bHi)
		return
	}
	pa, pb := aLo, bLo
	for _, an := range anchors {
		c.patience(pa, an[0], pb, an[1], depth+1)
		pa, pb = an[0]+1, an[1]+1
	}
	c.patience(pa, aHi, pb, bHi, depth+1)
}

// uniqueLCS returns the pairs (i, j) of positions holding a value that is
// unique within both ranges, reduced to the longest chain that increases in
// both i and j.
func (c *diffCtx) uniqueLCS(aLo, aHi, bLo, bHi int) [][2]int {
	type occ struct {
		countA, countB int
		posA, posB     int
	}
	m := make(map[int]*occ)
	for i := aLo; i < aHi; i++ {
		o := m[c.a[i]]
		if o == nil {
			o = &occ{}
			m[c.a[i]] = o
		}
		o.countA++
		o.posA = i
	}
	for j := bLo; j < bHi; j++ {
		o := m[c.b[j]]
		if o == nil {
			continue
		}
		o.countB++
		o.posB = j
	}
	var pairs [][2]int
	for _, o := range m {
		if o.countA == 1 && o.countB == 1 {
			pairs = append(pairs, [2]int{o.posA, o.posB})
		}
	}
	if len(pairs) == 0 {
		return nil
	}
	sort.Slice(pairs, func(x, y int) bool { return pairs[x][0] < pairs[y][0] })

	// Longest increasing subsequence on the b positions (patience sorting
	// with back pointers).
	tails := make([]int, 0, len(pairs)) // indexes into pairs
	prev := make([]int, len(pairs))
	for idx, p := range pairs {
		k := sort.Search(len(tails), func(t int) bool { return pairs[tails[t]][1] >= p[1] })
		if k > 0 {
			prev[idx] = tails[k-1]
		} else {
			prev[idx] = -1
		}
		if k == len(tails) {
			tails = append(tails, idx)
		} else {
			tails[k] = idx
		}
	}
	out := make([][2]int, len(tails))
	for k, idx := len(tails)-1, tails[len(tails)-1]; k >= 0; k-- {
		out[k] = pairs[idx]
		idx = prev[idx]
	}
	return out
}
