// This file groups an op list into hunks, the familiar "@@ -a,b +c,d @@"
// chunks of a unified diff: each difference together with a few lines of
// unchanged context around it, with neighbouring differences merged when
// their contexts touch. Hunks are what a "differences only" view shows, what
// gets written when the user exports the comparison as a patch file, and
// what an external tool or a test can consume. WriteUnified produces text in
// the standard format accepted by patch and git apply.
package compare

import (
	"bufio"
	"fmt"
	"io"
)

// HunkLine is one line of a hunk. Kind is ' ' (context), '-' (left only) or
// '+' (right only). ALine / BLine are the 0-based line numbers on each side,
// or -1 when the line does not exist there.
type HunkLine struct {
	Kind         byte
	Text         string
	ALine, BLine int
}

// Hunk is a group of differences with surrounding context. AStart/BStart are
// 0-based first line indexes, ALen/BLen the number of lines on each side.
type Hunk struct {
	AStart, ALen int
	BStart, BLen int
	Lines        []HunkLine
}

// Header returns the "@@ -l,s +l,s @@" line (1-based, as in diff(1)).
func (h Hunk) Header() string {
	return fmt.Sprintf("@@ -%s +%s @@", hunkRange(h.AStart, h.ALen), hunkRange(h.BStart, h.BLen))
}

func hunkRange(start, n int) string {
	switch n {
	case 0:
		return fmt.Sprintf("%d,0", start) // diff(1) uses the line before
	case 1:
		return fmt.Sprintf("%d", start+1)
	}
	return fmt.Sprintf("%d,%d", start+1, n)
}

// Hunks groups ops into hunks with ctx lines of context (negative = 0).
// Within a change op all left lines are listed before the right lines.
func Hunks(a, b []string, ops []Op, ctx int) []Hunk {
	if ctx < 0 {
		ctx = 0
	}
	var hunks []Hunk
	var cur *Hunk
	flush := func() {
		if cur != nil {
			hunks = append(hunks, *cur)
			cur = nil
		}
	}
	addEq := func(i, j int) {
		cur.Lines = append(cur.Lines, HunkLine{Kind: ' ', Text: a[i], ALine: i, BLine: j})
		cur.ALen++
		cur.BLen++
	}
	for oi, op := range ops {
		if op.Kind == OpEqual {
			if cur == nil {
				continue
			}
			n := op.A2 - op.A1
			last := oi == len(ops)-1
			if !last && n <= 2*ctx {
				// Context of the previous and next diff touch: keep all.
				for k := 0; k < n; k++ {
					addEq(op.A1+k, min(op.B1+k, op.B2-1))
				}
				continue
			}
			for k := 0; k < min(ctx, n); k++ {
				addEq(op.A1+k, min(op.B1+k, op.B2-1))
			}
			flush()
			continue
		}
		if cur == nil {
			// Start a new hunk with leading context from the previous op.
			startA, startB := op.A1, op.B1
			if oi > 0 && ops[oi-1].Kind == OpEqual {
				p := ops[oi-1]
				n := min(ctx, p.A2-p.A1, p.B2-p.B1)
				startA, startB = op.A1-n, op.B1-n
			}
			cur = &Hunk{AStart: startA, BStart: startB}
			for k := 0; k < op.A1-startA; k++ {
				addEq(startA+k, startB+k)
			}
		}
		for i := op.A1; i < op.A2; i++ {
			cur.Lines = append(cur.Lines, HunkLine{Kind: '-', Text: a[i], ALine: i, BLine: -1})
			cur.ALen++
		}
		for j := op.B1; j < op.B2; j++ {
			cur.Lines = append(cur.Lines, HunkLine{Kind: '+', Text: b[j], ALine: -1, BLine: j})
			cur.BLen++
		}
	}
	flush()
	return hunks
}

// WriteUnified writes a unified diff of a and b. nameA/nameB appear in the
// "---" / "+++" header lines. Nothing is written when there are no
// differences. Lines are written with "\n" endings.
func WriteUnified(w io.Writer, nameA, nameB string, a, b []string, ops []Op, ctx int) error {
	hunks := Hunks(a, b, ops, ctx)
	if len(hunks) == 0 {
		return nil
	}
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "--- %s\n+++ %s\n", nameA, nameB)
	for _, h := range hunks {
		fmt.Fprintln(bw, h.Header())
		for _, l := range h.Lines {
			bw.WriteByte(l.Kind)
			bw.WriteString(l.Text)
			bw.WriteByte('\n')
		}
	}
	return bw.Flush()
}
