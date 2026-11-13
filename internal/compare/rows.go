// This file lays an op list out as the rows of a side-by-side view, the way
// Total Commander's compare window shows two files next to each other with
// their line numbers, padding one side with empty rows where the other has
// extra lines so that matching lines stay level and both sides scroll
// together. It also groups the rows into difference blocks and answers the
// "next difference" / "previous difference" navigation questions, so the UI
// only has to keep a current row index.
package compare

// RowKind classifies a row of the side-by-side view.
type RowKind int

const (
	// RowEqual: same line on both sides (or one side padded inside a folded
	// run of repeated lines).
	RowEqual RowKind = iota
	// RowDelete: line only on the left; the right cell is empty.
	RowDelete
	// RowInsert: line only on the right; the left cell is empty.
	RowInsert
	// RowChange: the left line was changed into the right line; inline
	// highlighting applies.
	RowChange
)

// Row is one visual row. Left and Right are 0-based line indexes, or -1 for
// an empty padding cell. Op is the index of the op the row belongs to.
type Row struct {
	Kind        RowKind
	Left, Right int
	Op          int
}

// BuildRows expands ops into side-by-side rows. Within a change op the
// lines are paired in order; surplus lines on the longer side become delete
// or insert rows.
func BuildRows(ops []Op) []Row {
	var rows []Row
	for oi, op := range ops {
		na, nb := op.A2-op.A1, op.B2-op.B1
		n := max(na, nb)
		for k := 0; k < n; k++ {
			l, r := -1, -1
			if k < na {
				l = op.A1 + k
			}
			if k < nb {
				r = op.B1 + k
			}
			kind := RowEqual
			if op.Kind != OpEqual {
				switch {
				case l >= 0 && r >= 0:
					kind = RowChange
				case l >= 0:
					kind = RowDelete
				default:
					kind = RowInsert
				}
			}
			rows = append(rows, Row{Kind: kind, Left: l, Right: r, Op: oi})
		}
	}
	return rows
}

// Block is one difference block in row coordinates: rows First..Last
// inclusive, belonging to op index Op.
type Block struct {
	First, Last int
	Op          int
}

// Blocks returns the difference blocks of a row list in order.
func Blocks(rows []Row) []Block {
	var out []Block
	for i, r := range rows {
		if r.Kind == RowEqual {
			continue
		}
		if len(out) > 0 && out[len(out)-1].Op == r.Op && out[len(out)-1].Last == i-1 {
			out[len(out)-1].Last = i
			continue
		}
		out = append(out, Block{First: i, Last: i, Op: r.Op})
	}
	return out
}

// NextDiff returns the index of the first block that starts after row, or -1
// if there is none. Pass row = -1 to get the first block.
func NextDiff(blocks []Block, row int) int {
	for i, b := range blocks {
		if b.First > row {
			return i
		}
	}
	return -1
}

// PrevDiff returns the index of the last block that ends before row (so from
// inside a block it moves to the block before it), or -1 if there is none.
// Pass row = len(rows) to get the last block.
func PrevDiff(blocks []Block, row int) int {
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Last < row {
			return i
		}
	}
	return -1
}

// BlockAt returns the index of the block containing row, or -1.
func BlockAt(blocks []Block, row int) int {
	for i, b := range blocks {
		if row >= b.First && row <= b.Last {
			return i
		}
	}
	return -1
}

// RowOfLine returns the row showing the given line of one side (0 = left,
// 1 = right), or -1. Used for "go to line" and to keep the cursor in place
// after a recompute.
func RowOfLine(rows []Row, side Side, line int) int {
	for i, r := range rows {
		if (side == Left && r.Left == line) || (side == Right && r.Right == line) {
			return i
		}
	}
	return -1
}
