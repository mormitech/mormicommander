// This file turns two lists of lines into a list of edit operations, the
// central result of a text comparison. Lines are first normalised according
// to the options (case, whitespace) and interned to integers, then diffed
// with the selected algorithm. Two of Total Commander's options are applied
// around that core: "ignore repeated lines" folds runs of identical lines
// into one before diffing and expands them again afterwards, and the resync
// setting merges differences that are separated by fewer matching lines than
// required, so that one heavily edited paragraph shows as one block.
package compare

// OpKind classifies an edit operation.
type OpKind int

const (
	// OpEqual: the lines match (under the options). With IgnoreRepeatedLines
	// an equal op may cover a different number of lines on each side.
	OpEqual OpKind = iota
	// OpDelete: lines exist only on the left (A) side.
	OpDelete
	// OpInsert: lines exist only on the right (B) side.
	OpInsert
	// OpChange: lines on the left were replaced by lines on the right.
	OpChange
)

func (k OpKind) String() string {
	switch k {
	case OpEqual:
		return "equal"
	case OpDelete:
		return "delete"
	case OpInsert:
		return "insert"
	case OpChange:
		return "change"
	}
	return "?"
}

// Op is one edit operation. The ranges are half-open line index ranges:
// A[A1:A2] on the left and B[B1:B2] on the right. For a delete B1 == B2 marks
// the insertion point on the right, for an insert A1 == A2 marks it on the
// left. Consecutive ops cover both files completely and without gaps.
type Op struct {
	Kind   OpKind
	A1, A2 int
	B1, B2 int
}

// IsDiff reports whether the op is a difference (not OpEqual).
func (o Op) IsDiff() bool { return o.Kind != OpEqual }

// DiffLines compares two lists of lines and returns the edit operations that
// turn a into b.
func DiffLines(a, b []string, opts Options) []Op {
	in := newInterner()
	ka := make([]int, len(a))
	kb := make([]int, len(b))
	for i, s := range a {
		ka[i] = in.id(NormalizeLine(s, opts))
	}
	for i, s := range b {
		kb[i] = in.id(NormalizeLine(s, opts))
	}
	return diffKeys(ka, kb, opts)
}

// diffKeys runs the configured algorithm on interned keys and applies the
// repeated-line folding and resync post-processing.
func diffKeys(ka, kb []int, opts Options) []Op {
	var startA, startB []int
	if opts.IgnoreRepeatedLines {
		ka, startA = foldRuns(ka)
		kb, startB = foldRuns(kb)
	}
	var ca, cb []bool
	if opts.Algorithm == AlgorithmMyers {
		ca, cb = myersDiff(ka, kb, opts.MaxCost)
	} else {
		ca, cb = patienceDiff(ka, kb, opts.MaxCost)
	}
	ops := opsFromFlags(ca, cb)
	if opts.IgnoreRepeatedLines {
		ops = expandRuns(ops, startA, startB)
	}
	return applyResync(ops, opts.resync())
}

// foldRuns collapses runs of equal values. start[i] is the original index of
// folded element i; start[len(out)] is the original length.
func foldRuns(k []int) (out, start []int) {
	for i, v := range k {
		if i > 0 && k[i-1] == v {
			continue
		}
		out = append(out, v)
		start = append(start, i)
	}
	start = append(start, len(k))
	return out, start
}

// expandRuns maps ops on folded sequences back to original line numbers.
// An equal op is split into one op per folded run, so that each run of
// repeated lines starts level with its partner run in the side-by-side view
// (consecutive equal ops are therefore possible in this mode).
func expandRuns(ops []Op, startA, startB []int) []Op {
	out := make([]Op, 0, len(ops))
	for _, op := range ops {
		if op.Kind != OpEqual {
			out = append(out, Op{Kind: op.Kind,
				A1: startA[op.A1], A2: startA[op.A2],
				B1: startB[op.B1], B2: startB[op.B2]})
			continue
		}
		for k := 0; k < op.A2-op.A1; k++ {
			i, j := op.A1+k, op.B1+k
			out = append(out, Op{Kind: OpEqual,
				A1: startA[i], A2: startA[i+1],
				B1: startB[j], B2: startB[j+1]})
		}
	}
	return out
}

// opsFromFlags converts changed flags into a gap-free op list.
func opsFromFlags(ca, cb []bool) []Op {
	var ops []Op
	i, j := 0, 0
	n, m := len(ca), len(cb)
	for i < n || j < m {
		if i < n && j < m && !ca[i] && !cb[j] {
			i0, j0 := i, j
			for i < n && j < m && !ca[i] && !cb[j] {
				i++
				j++
			}
			ops = append(ops, Op{Kind: OpEqual, A1: i0, A2: i, B1: j0, B2: j})
			continue
		}
		i0, j0 := i, j
		for i < n && ca[i] {
			i++
		}
		for j < m && cb[j] {
			j++
		}
		if i == i0 && j == j0 {
			// Defensive: unmatched tails (cannot happen with a valid diff).
			if i < n {
				i = n
			}
			if j < m {
				j = m
			}
		}
		ops = append(ops, newDiffOp(i0, i, j0, j))
	}
	return ops
}

func newDiffOp(a1, a2, b1, b2 int) Op {
	k := OpChange
	switch {
	case a1 == a2:
		k = OpInsert
	case b1 == b2:
		k = OpDelete
	}
	return Op{Kind: k, A1: a1, A2: a2, B1: b1, B2: b2}
}

// applyResync merges an equal op shorter than n lines (on both sides) that
// sits between two differences into a single change op.
func applyResync(ops []Op, n int) []Op {
	if n <= 1 || len(ops) < 3 {
		return ops
	}
	out := make([]Op, 0, len(ops))
	for _, op := range ops {
		if len(out) > 0 && op.IsDiff() {
			last := out[len(out)-1]
			// Pattern: diff, short equal, diff -> merge all three.
			if last.Kind == OpEqual && len(out) >= 2 && out[len(out)-2].IsDiff() &&
				last.A2-last.A1 < n && last.B2-last.B1 < n {
				prev := out[len(out)-2]
				out = out[:len(out)-2]
				out = append(out, newDiffOp(prev.A1, op.A2, prev.B1, op.B2))
				continue
			}
		}
		out = append(out, op)
	}
	return out
}

// Stats summarises an op list.
type Stats struct {
	Equal, Deleted, Inserted, ChangedA, ChangedB int
	// Blocks is the number of difference blocks.
	Blocks int
}

// Identical reports whether no differences were found.
func (s Stats) Identical() bool { return s.Blocks == 0 }

// Summarize counts lines per op kind.
func Summarize(ops []Op) Stats {
	var s Stats
	for _, op := range ops {
		switch op.Kind {
		case OpEqual:
			s.Equal += op.A2 - op.A1
		case OpDelete:
			s.Deleted += op.A2 - op.A1
			s.Blocks++
		case OpInsert:
			s.Inserted += op.B2 - op.B1
			s.Blocks++
		case OpChange:
			s.ChangedA += op.A2 - op.A1
			s.ChangedB += op.B2 - op.B1
			s.Blocks++
		}
	}
	return s
}
