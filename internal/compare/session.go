// This file is the state behind one open compare window. A Session owns the
// two documents, the current options and the computed ops, rows and
// difference blocks, and it implements the editing features of Total
// Commander's compare tool: editing a line in place, inserting and deleting
// lines, copying a difference block (or any range of rows) from one side to
// the other, unlimited undo and redo, reloading a side in another encoding,
// and saving each side back to disk. Every edit is recorded as a "splice"
// (replace n lines at a position with some new lines), which makes undo
// simply the inverse splice, and after every change the comparison is
// recomputed so the highlighting always matches the text. The UI keeps only
// a scroll position and a current row; everything else is asked from here.
package compare

import (
	"errors"
	"fmt"
	"slices"
)

// Side selects the left (A) or right (B) document.
type Side int

const (
	Left  Side = 0
	Right Side = 1
)

// Other returns the opposite side.
func (s Side) Other() Side { return 1 - s }

func (s Side) String() string {
	if s == Left {
		return "left"
	}
	return "right"
}

// ErrModified is returned when an operation would discard unsaved edits.
var ErrModified = errors.New("compare: document has unsaved changes")

// splice replaces Removed lines at At with Inserted lines on Side.
type splice struct {
	id       int
	side     Side
	at       int
	removed  []Line
	inserted []Line
}

// Session is the model of one compare window.
type Session struct {
	docs [2]*Document
	det  [2]Detection
	opts Options

	ops    []Op
	rows   []Row
	blocks []Block

	undo, redo []splice
	nextID     int
	saved      [2]int // id of the newest applied splice per side at last save
}

// NewSession compares two documents.
func NewSession(left, right *Document, opts Options) *Session {
	s := &Session{docs: [2]*Document{left, right}, opts: opts}
	s.Recompute()
	return s
}

// OpenFiles loads two files (detecting their encodings with dopts) and
// compares them. Check Binary() afterwards: when either file looks binary the
// UI should switch to CompareBinaryFiles instead of showing text.
func OpenFiles(pathA, pathB string, opts Options, dopts DetectOptions) (*Session, error) {
	a, detA, err := LoadFile(pathA, EncAuto, dopts)
	if err != nil {
		return nil, err
	}
	b, detB, err := LoadFile(pathB, EncAuto, dopts)
	if err != nil {
		return nil, err
	}
	s := NewSession(a, b, opts)
	s.det = [2]Detection{detA, detB}
	return s, nil
}

// Binary reports whether either side was detected as binary data.
func (s *Session) Binary() bool { return s.det[0].Binary || s.det[1].Binary }

// Detection returns the encoding detection result of a side (zero value for
// documents not loaded through OpenFiles).
func (s *Session) Detection(side Side) Detection { return s.det[side] }

// Doc returns the document of a side.
func (s *Session) Doc(side Side) *Document { return s.docs[side] }

// Options returns the current options.
func (s *Session) Options() Options { return s.opts }

// SetOptions changes the options and recomputes.
func (s *Session) SetOptions(o Options) {
	s.opts = o
	s.Recompute()
}

// Recompute re-runs the comparison on the current text.
func (s *Session) Recompute() {
	s.ops = DiffLines(s.docs[0].Texts(), s.docs[1].Texts(), s.opts)
	s.rows = BuildRows(s.ops)
	s.blocks = Blocks(s.rows)
}

// Ops returns the edit operations.
func (s *Session) Ops() []Op { return s.ops }

// Rows returns the side-by-side rows.
func (s *Session) Rows() []Row { return s.rows }

// Blocks returns the difference blocks.
func (s *Session) Blocks() []Block { return s.blocks }

// Stats summarises the differences.
func (s *Session) Stats() Stats { return Summarize(s.ops) }

// Identical reports whether the documents compare equal under the options.
func (s *Session) Identical() bool { return len(s.blocks) == 0 }

// LineText returns the text of a row's cell on a side ("" for padding).
func (s *Session) LineText(row int, side Side) string {
	r := s.rows[row]
	idx := r.Left
	if side == Right {
		idx = r.Right
	}
	if idx < 0 {
		return ""
	}
	return s.docs[side].Lines[idx].Text
}

// InlineSpans returns the highlighted spans of a row. For a change row the
// intra-line diff is computed; other rows get one span covering the line
// (changed for delete/insert rows, unchanged for equal rows).
func (s *Session) InlineSpans(row int) (left, right []Span) {
	r := s.rows[row]
	lt, rt := s.LineText(row, Left), s.LineText(row, Right)
	switch r.Kind {
	case RowChange:
		return InlineDiff(lt, rt, s.opts)
	case RowEqual:
		return wholeSpan(lt, false), wholeSpan(rt, false)
	}
	return wholeSpan(lt, true), wholeSpan(rt, true)
}

// apply performs a splice on the documents (without recording it).
func (s *Session) apply(sp splice) {
	d := s.docs[sp.side]
	d.Lines = slices.Replace(d.Lines, sp.at, sp.at+len(sp.removed), sp.inserted...)
}

// edit records and applies a splice, clears redo and recomputes.
func (s *Session) edit(side Side, at, n int, inserted []Line) error {
	d := s.docs[side]
	if at < 0 || n < 0 || at+n > len(d.Lines) {
		return fmt.Errorf("compare: line range %d+%d out of bounds (0..%d)", at, n, len(d.Lines))
	}
	if n == 0 && len(inserted) == 0 {
		return nil
	}
	s.nextID++
	sp := splice{
		id:       s.nextID,
		side:     side,
		at:       at,
		removed:  slices.Clone(d.Lines[at : at+n]),
		inserted: s.fixEOLs(side, at, n, inserted),
	}
	s.apply(sp)
	s.undo = append(s.undo, sp)
	s.redo = nil
	s.Recompute()
	return nil
}

// fixEOLs gives new lines proper terminators: every line gets the
// document's dominant ending, except that a block placed at the very end of
// a document whose last line had no terminator keeps that property. (When
// only deleting an unterminated last line, the line before it keeps its
// terminator, so the file then ends with a newline; that is harmless and
// avoids touching lines outside the edit.)
func (s *Session) fixEOLs(side Side, at, n int, in []Line) []Line {
	d := s.docs[side]
	eol := d.DefaultEOL()
	out := make([]Line, len(in))
	for i, l := range in {
		out[i] = Line{Text: l.Text, EOL: eol}
	}
	atEnd := at+n == len(d.Lines)
	if len(out) > 0 && atEnd {
		hadFinal := len(d.Lines) == 0 || d.Lines[len(d.Lines)-1].EOL != ""
		if !hadFinal {
			out[len(out)-1].EOL = ""
		}
	}
	return out
}

// SetLine replaces the text of one line.
func (s *Session) SetLine(side Side, idx int, text string) error {
	if idx < 0 || idx >= len(s.docs[side].Lines) {
		return fmt.Errorf("compare: line %d out of range", idx)
	}
	old := s.docs[side].Lines[idx]
	if old.Text == text {
		return nil
	}
	if err := s.edit(side, idx, 1, []Line{{Text: text}}); err != nil {
		return err
	}
	// Keep the original terminator of an edited line.
	s.docs[side].Lines[idx].EOL = old.EOL
	s.undo[len(s.undo)-1].inserted[0].EOL = old.EOL
	return nil
}

// InsertLines inserts lines before index at (at == len inserts at the end).
func (s *Session) InsertLines(side Side, at int, texts ...string) error {
	lines := make([]Line, len(texts))
	for i, t := range texts {
		lines[i] = Line{Text: t}
	}
	return s.edit(side, at, 0, lines)
}

// DeleteLines removes n lines starting at at.
func (s *Session) DeleteLines(side Side, at, n int) error {
	return s.edit(side, at, n, nil)
}

// CopyBlock copies difference block number b from side from to the other
// side, making that block identical.
func (s *Session) CopyBlock(b int, from Side) error {
	if b < 0 || b >= len(s.blocks) {
		return fmt.Errorf("compare: block %d out of range", b)
	}
	return s.CopyRows(s.blocks[b].First, s.blocks[b].Last, from)
}

// CopyAllBlocks copies every difference from side from to the other side as
// a single undoable edit, which makes the target a copy of the source text
// (keeping the target's encoding and line endings).
func (s *Session) CopyAllBlocks(from Side) error {
	to := from.Other()
	src := s.docs[from].Lines
	return s.edit(to, 0, len(s.docs[to].Lines), slices.Clone(src))
}

// CopyRows copies the lines shown in rows first..last (inclusive) from side
// from over the corresponding lines of the other side. Lines missing on the
// source side are deleted on the target side, so after the copy the selected
// rows compare equal.
func (s *Session) CopyRows(first, last int, from Side) error {
	if first < 0 || last >= len(s.rows) || first > last {
		return fmt.Errorf("compare: row range %d..%d out of range", first, last)
	}
	to := from.Other()
	cell := func(r Row, side Side) int {
		if side == Left {
			return r.Left
		}
		return r.Right
	}
	var src []Line
	tLo, tHi := -1, -1
	for _, r := range s.rows[first : last+1] {
		if i := cell(r, from); i >= 0 {
			src = append(src, s.docs[from].Lines[i])
		}
		if j := cell(r, to); j >= 0 {
			if tLo < 0 {
				tLo = j
			}
			tHi = j + 1
		}
	}
	if tLo < 0 {
		// No target lines in the range: insert before the next target line.
		tLo = len(s.docs[to].Lines)
		for _, r := range s.rows[last+1:] {
			if j := cell(r, to); j >= 0 {
				tLo = j
				break
			}
		}
		tHi = tLo
	}
	return s.edit(to, tLo, tHi-tLo, src)
}

// CanUndo / CanRedo report whether undo or redo is possible.
func (s *Session) CanUndo() bool { return len(s.undo) > 0 }
func (s *Session) CanRedo() bool { return len(s.redo) > 0 }

// Undo reverts the most recent edit. It returns the side that changed and
// false when there was nothing to undo.
func (s *Session) Undo() (Side, bool) {
	if len(s.undo) == 0 {
		return Left, false
	}
	sp := s.undo[len(s.undo)-1]
	s.undo = s.undo[:len(s.undo)-1]
	s.apply(splice{side: sp.side, at: sp.at, removed: sp.inserted, inserted: sp.removed})
	s.redo = append(s.redo, sp)
	s.Recompute()
	return sp.side, true
}

// Redo re-applies the most recently undone edit.
func (s *Session) Redo() (Side, bool) {
	if len(s.redo) == 0 {
		return Left, false
	}
	sp := s.redo[len(s.redo)-1]
	s.redo = s.redo[:len(s.redo)-1]
	s.apply(sp)
	s.undo = append(s.undo, sp)
	s.Recompute()
	return sp.side, true
}

func (s *Session) topID(side Side) int {
	for i := len(s.undo) - 1; i >= 0; i-- {
		if s.undo[i].side == side {
			return s.undo[i].id
		}
	}
	return 0
}

// Modified reports whether a side differs from its last saved (or loaded)
// state. Undoing back to the saved state clears the flag again.
func (s *Session) Modified(side Side) bool {
	return s.topID(side) != s.saved[side]
}

// Save writes a side to path (empty = the path it was loaded from) in the
// document's encoding.
func (s *Session) Save(side Side, path string) error {
	d := s.docs[side]
	if path == "" {
		path = d.Path
	}
	if path == "" {
		return errors.New("compare: no file name to save to")
	}
	if err := d.SaveFile(path); err != nil {
		return err
	}
	d.Path = path
	s.saved[side] = s.topID(side)
	return nil
}

// ReloadSide reads a side's file again, optionally in a different encoding
// (EncAuto = detect). Unsaved edits are refused unless force is set. The undo
// history of that side is dropped because it no longer matches the text; the
// other side's history stays usable since splices on different sides are
// independent.
func (s *Session) ReloadSide(side Side, e Encoding, dopts DetectOptions, force bool) error {
	if !force && s.Modified(side) {
		return ErrModified
	}
	path := s.docs[side].Path
	if path == "" {
		return errors.New("compare: document has no file to reload")
	}
	d, det, err := LoadFile(path, e, dopts)
	if err != nil {
		return err
	}
	s.docs[side] = d
	s.det[side] = det
	keep := func(list []splice) []splice {
		return slices.DeleteFunc(list, func(sp splice) bool { return sp.side == side })
	}
	s.undo, s.redo = keep(s.undo), keep(s.redo)
	s.saved[side] = 0
	s.Recompute()
	return nil
}
