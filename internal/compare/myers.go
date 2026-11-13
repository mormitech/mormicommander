// This file implements Eugene Myers' O(ND) difference algorithm in its
// linear-space, divide-and-conquer form. It works on sequences of small
// integers (interned lines, words or characters), so the same code serves the
// line diff and the intra-line diff. The result is expressed as two "changed"
// flag arrays, one per input, the representation GNU diff also uses: an
// element whose flag is false is part of the common subsequence, and the
// unchanged elements of both sides pair up in order. Turning the flags into
// edit operations is done by the callers.
//
// Two huge files that share nothing would make the search quadratic, so the
// search carries a cost limit. When a region exceeds it, the search splits at
// the furthest point it has reached instead of the true middle snake. The
// result is still a correct diff (applying it turns one side into the other),
// only possibly not the shortest one, which is the same trade-off GNU diff
// makes with its "too expensive" heuristic.
package compare

import "math"

// diffCtx carries the shared state of one sequence comparison.
type diffCtx struct {
	a, b    []int
	ca, cb  []bool
	maxCost int // <=0: unlimited
	v1, v2  []int
}

// myersDiff marks the elements of a and b that are not part of a (near)
// longest common subsequence. maxCost follows Options.MaxCost semantics.
func myersDiff(a, b []int, maxCost int) (ca, cb []bool) {
	ctx := newDiffCtx(a, b, maxCost)
	ctx.compare(0, len(a), 0, len(b))
	return ctx.ca, ctx.cb
}

func newDiffCtx(a, b []int, maxCost int) *diffCtx {
	ctx := &diffCtx{a: a, b: b, ca: make([]bool, len(a)), cb: make([]bool, len(b))}
	switch {
	case maxCost < 0:
		ctx.maxCost = 0
	case maxCost == 0:
		// Automatic: like GNU diff, allow roughly sqrt(N+M) * 16 rounds but
		// never fewer than 4096, which keeps everyday files exact.
		n := float64(len(a) + len(b))
		ctx.maxCost = int(math.Max(4096, math.Sqrt(n)*16))
	default:
		ctx.maxCost = maxCost
	}
	return ctx
}

// compare diffs a[aLo:aHi] against b[bLo:bHi].
func (c *diffCtx) compare(aLo, aHi, bLo, bHi int) {
	for {
		for aLo < aHi && bLo < bHi && c.a[aLo] == c.b[bLo] {
			aLo++
			bLo++
		}
		for aLo < aHi && bLo < bHi && c.a[aHi-1] == c.b[bHi-1] {
			aHi--
			bHi--
		}
		if aLo == aHi {
			for j := bLo; j < bHi; j++ {
				c.cb[j] = true
			}
			return
		}
		if bLo == bHi {
			for i := aLo; i < aHi; i++ {
				c.ca[i] = true
			}
			return
		}
		x, y, ok := c.bisect(aLo, aHi, bLo, bHi)
		if !ok || (x == aLo && y == bLo) || (x == aHi && y == bHi) {
			for i := aLo; i < aHi; i++ {
				c.ca[i] = true
			}
			for j := bLo; j < bHi; j++ {
				c.cb[j] = true
			}
			return
		}
		// Recurse on the smaller half and loop on the other to bound the
		// stack depth.
		if (x-aLo)+(y-bLo) < (aHi-x)+(bHi-y) {
			c.compare(aLo, x, bLo, y)
			aLo, bLo = x, y
		} else {
			c.compare(x, aHi, y, bHi)
			aHi, bHi = x, y
		}
	}
}

// bisect finds the middle snake of a[aLo:aHi] vs b[bLo:bHi] (both non-empty,
// with no common prefix or suffix) and returns an absolute split point. It
// follows the formulation used by diff-match-patch. ok is false when the two
// ranges share nothing at all.
func (c *diffCtx) bisect(aLo, aHi, bLo, bHi int) (sx, sy int, ok bool) {
	a := c.a[aLo:aHi]
	b := c.b[bLo:bHi]
	n, m := len(a), len(b)
	maxD := (n + m + 1) / 2
	vOff := maxD
	vLen := 2*maxD + 2
	if cap(c.v1) < vLen {
		c.v1 = make([]int, vLen)
		c.v2 = make([]int, vLen)
	}
	v1 := c.v1[:vLen]
	v2 := c.v2[:vLen]
	for i := range v1 {
		v1[i] = -1
		v2[i] = -1
	}
	v1[vOff+1] = 0
	v2[vOff+1] = 0
	delta := n - m
	front := delta%2 != 0
	k1start, k1end, k2start, k2end := 0, 0, 0, 0
	for d := 0; d < maxD; d++ {
		if c.maxCost > 0 && d >= c.maxCost {
			// Too expensive: split at the forward path that got furthest.
			bestX, bestY := -1, -1
			for k := -d + 1; k <= d-1; k += 2 {
				x := v1[vOff+k]
				if x < 0 {
					continue
				}
				y := x - k
				if x > n || y > m || y < 0 {
					continue
				}
				if x+y > bestX+bestY {
					bestX, bestY = x, y
				}
			}
			if bestX <= 0 && bestY <= 0 {
				return 0, 0, false
			}
			return aLo + bestX, bLo + bestY, true
		}
		for k1 := -d + k1start; k1 <= d-k1end; k1 += 2 {
			k1Off := vOff + k1
			var x1 int
			if k1 == -d || (k1 != d && v1[k1Off-1] < v1[k1Off+1]) {
				x1 = v1[k1Off+1]
			} else {
				x1 = v1[k1Off-1] + 1
			}
			y1 := x1 - k1
			for x1 < n && y1 < m && a[x1] == b[y1] {
				x1++
				y1++
			}
			v1[k1Off] = x1
			if x1 > n {
				k1end += 2
			} else if y1 > m {
				k1start += 2
			} else if front {
				k2Off := vOff + delta - k1
				if k2Off >= 0 && k2Off < vLen && v2[k2Off] != -1 {
					x2 := n - v2[k2Off]
					if x1 >= x2 {
						return aLo + x1, bLo + y1, true
					}
				}
			}
		}
		for k2 := -d + k2start; k2 <= d-k2end; k2 += 2 {
			k2Off := vOff + k2
			var x2 int
			if k2 == -d || (k2 != d && v2[k2Off-1] < v2[k2Off+1]) {
				x2 = v2[k2Off+1]
			} else {
				x2 = v2[k2Off-1] + 1
			}
			y2 := x2 - k2
			for x2 < n && y2 < m && a[n-x2-1] == b[m-y2-1] {
				x2++
				y2++
			}
			v2[k2Off] = x2
			if x2 > n {
				k2end += 2
			} else if y2 > m {
				k2start += 2
			} else if !front {
				k1Off := vOff + delta - k2
				if k1Off >= 0 && k1Off < vLen && v1[k1Off] != -1 {
					x1 := v1[k1Off]
					y1 := vOff + x1 - k1Off
					if x1 >= n-x2 {
						return aLo + x1, bLo + y1, true
					}
				}
			}
		}
	}
	return 0, 0, false
}
