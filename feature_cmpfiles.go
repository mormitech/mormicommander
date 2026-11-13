package main

// Compare by content (cm_CompareFilesByContent, FEATURES.md section 26) for the page: a thin layer over
// internal/compare. A session holds the two documents of one compare window; every call that changes something (an
// option, an edit, a copied block, undo, an encoding) returns the whole new view, so the page only draws. Binary files,
// and text files too big to show as text, are compared byte by byte; the page then reads the bytes it shows with Bytes.
// An external compare program (F-563) is started with ExternalStart instead of the window.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"mormicommander/internal/compare"
)

// cmpTextLimit is the largest file shown as text; bigger files are compared byte by byte (spec area 4, 5.3).
const cmpTextLimit = 50 << 20

// CompareFiles is what the page calls for the compare window (window.go.main.CompareFiles).
type CompareFiles struct {
	app  *App
	mu   sync.Mutex
	next int
	sess map[int]*cmpSession
}

type cmpSession struct {
	mu          sync.Mutex
	left, right string
	opts        CmpOptions
	s           *compare.Session      // text mode
	bin         *compare.BinaryResult // binary mode
	note        string                // why the files are shown as binary, when they were not asked to be
}

func init() {
	registerFeature("cmpfiles", func(a *App) []any { return []any{&CompareFiles{app: a, sess: map[int]*cmpSession{}}} })
}

// CmpOptions are the options of the compare window.
type CmpOptions struct {
	IgnoreCase     bool   `json:"ignoreCase"`
	Whitespace     string `json:"whitespace"` // "exact", "change" (ignore repeated and outer blanks) or "all"
	IgnoreRepeated bool   `json:"ignoreRepeated"`
	ResyncLines    int    `json:"resyncLines"`
	Binary         bool   `json:"binary"` // compare byte by byte even when both files are text
	ANSI           string `json:"ansi"`   // the code page assumed for text that is not UTF-8 ("" = Windows-1252)
}

func (o CmpOptions) engine() compare.Options {
	e := compare.Options{IgnoreCase: o.IgnoreCase, IgnoreRepeatedLines: o.IgnoreRepeated, ResyncLines: o.ResyncLines}
	switch o.Whitespace {
	case "change":
		e.Whitespace = compare.WhitespaceIgnoreChange
	case "all":
		e.Whitespace = compare.WhitespaceIgnoreAll
	}
	return e
}

func (o CmpOptions) detect() compare.DetectOptions {
	d := compare.DetectOptions{}
	if o.ANSI != "" {
		if e, err := compare.ParseEncoding(o.ANSI); err == nil {
			d.ANSI = e
		}
	}
	return d
}

// CmpRow is one row of the side-by-side view. L and R are 0-based line numbers (-1 for an empty cell); SA and SB are
// the changed parts of a changed line as [start, end) pairs in UTF-16 units, the way the page indexes strings.
type CmpRow struct {
	K  int      `json:"k"` // 0 equal, 1 only left, 2 only right, 3 changed
	L  int      `json:"l"`
	R  int      `json:"r"`
	A  string   `json:"a"`
	B  string   `json:"b"`
	SA [][2]int `json:"sa,omitempty"`
	SB [][2]int `json:"sb,omitempty"`
}

// CmpView is everything the compare window shows.
type CmpView struct {
	ID        int        `json:"id"`
	Left      string     `json:"left"`
	Right     string     `json:"right"`
	Binary    bool       `json:"binary"`
	Note      string     `json:"note"`
	Identical bool       `json:"identical"`
	Options   CmpOptions `json:"options"`
	// text mode
	Rows      []CmpRow  `json:"rows"`
	Blocks    [][2]int  `json:"blocks"` // first and last row of each difference
	Stats     cmpStats  `json:"stats"`
	Encoding  [2]string `json:"encoding"`
	Modified  [2]bool   `json:"modified"`
	CanUndo   bool      `json:"canUndo"`
	CanRedo   bool      `json:"canRedo"`
	LineCount [2]int    `json:"lineCount"`
	// binary mode
	SizeA     int64      `json:"sizeA"`
	SizeB     int64      `json:"sizeB"`
	Runs      [][2]int64 `json:"runs"` // offset and length of each run of differing bytes
	DiffBytes int        `json:"diffBytes"`
	Truncated bool       `json:"truncated"`
}

type cmpStats struct {
	Blocks   int `json:"blocks"`
	Deleted  int `json:"deleted"`
	Inserted int `json:"inserted"`
	Changed  int `json:"changed"`
}

// CmpBytes are bytes of both files from one offset, for the hex view.
type CmpBytes struct {
	A []byte `json:"a"`
	B []byte `json:"b"`
}

func (c *CompareFiles) get(id int) (*cmpSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sess[id]
	if s == nil {
		return nil, errors.New("This compare window is closed.")
	}
	return s, nil
}

// cmpCheckFile checks that a path is a regular file that can be compared.
func cmpCheckFile(p string) (os.FileInfo, error) {
	if strings.Contains(p, "://") {
		return nil, fmt.Errorf("%s is not on this computer. Only files on this computer can be compared for now.", p)
	}
	fi, err := os.Stat(p)
	if err != nil {
		return nil, fmt.Errorf("The file %s is not there.", p)
	}
	if fi.IsDir() {
		return nil, errors.New("Please select two files to compare.")
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file, so it can't be compared.", filepath.Base(p))
	}
	return fi, nil
}

// load (re)reads both files in the mode the options and the files ask for.
func (s *cmpSession) load() error {
	fa, err := cmpCheckFile(s.left)
	if err != nil {
		return err
	}
	fb, err := cmpCheckFile(s.right)
	if err != nil {
		return err
	}
	s.s, s.bin, s.note = nil, nil, ""
	binary := s.opts.Binary
	if !binary && (fa.Size() > cmpTextLimit || fb.Size() > cmpTextLimit) {
		binary, s.note = true, "The files are too big to compare as text, so their bytes are compared."
	}
	if !binary {
		ses, err := compare.OpenFiles(s.left, s.right, s.opts.engine(), s.opts.detect())
		if err != nil {
			return err
		}
		if ses.Binary() {
			binary, s.note = true, "The files are not text, so their bytes are compared."
		} else {
			s.s = ses
		}
	}
	if binary {
		res, err := compare.CompareBinaryFiles(context.Background(), s.left, s.right, compare.DefaultMaxBinaryDiffs)
		if err != nil {
			return err
		}
		s.bin = res
	}
	return nil
}

// utf16Spans turns the engine's byte spans of a line into changed [start, end) pairs in UTF-16 units.
func utf16Spans(text string, spans []compare.Span) [][2]int {
	var out [][2]int
	pos, u := 0, 0 // byte position and UTF-16 position reached so far
	at := func(b int) int {
		for pos < b && pos < len(text) {
			r, n := utf8.DecodeRuneInString(text[pos:])
			pos += n
			if r >= 0x10000 {
				u += 2
			} else {
				u++
			}
		}
		return u
	}
	for _, sp := range spans {
		a := at(sp.Start)
		b := at(sp.End)
		if sp.Changed && b > a {
			if n := len(out); n > 0 && out[n-1][1] == a {
				out[n-1][1] = b
			} else {
				out = append(out, [2]int{a, b})
			}
		}
	}
	return out
}

func (s *cmpSession) view(id int) *CmpView {
	v := &CmpView{ID: id, Left: s.left, Right: s.right, Options: s.opts, Note: s.note, Rows: []CmpRow{}, Blocks: [][2]int{}, Runs: [][2]int64{}}
	if s.bin != nil {
		v.Binary = true
		v.SizeA, v.SizeB = s.bin.SizeA, s.bin.SizeB
		for _, r := range s.bin.Runs {
			v.Runs = append(v.Runs, [2]int64{r.Offset, r.Len})
		}
		if s.bin.SizeA != s.bin.SizeB && !s.bin.Truncated {
			short := min(s.bin.SizeA, s.bin.SizeB)
			v.Runs = append(v.Runs, [2]int64{short, max(s.bin.SizeA, s.bin.SizeB) - short})
		}
		v.DiffBytes, v.Truncated, v.Identical = len(s.bin.Diffs), s.bin.Truncated, s.bin.Identical()
		return v
	}
	ses := s.s
	rows := ses.Rows()
	v.Rows = make([]CmpRow, len(rows))
	for i, r := range rows {
		cr := CmpRow{K: int(r.Kind), L: r.Left, R: r.Right, A: ses.LineText(i, compare.Left), B: ses.LineText(i, compare.Right)}
		if r.Kind == compare.RowChange {
			sa, sb := ses.InlineSpans(i)
			cr.SA, cr.SB = utf16Spans(cr.A, sa), utf16Spans(cr.B, sb)
		}
		v.Rows[i] = cr
	}
	for _, b := range ses.Blocks() {
		v.Blocks = append(v.Blocks, [2]int{b.First, b.Last})
	}
	st := ses.Stats()
	v.Stats = cmpStats{Blocks: st.Blocks, Deleted: st.Deleted, Inserted: st.Inserted, Changed: max(st.ChangedA, st.ChangedB)}
	v.Identical = ses.Identical()
	for i, side := range []compare.Side{compare.Left, compare.Right} {
		d := ses.Doc(side)
		v.Encoding[i] = d.Encoding.String()
		if d.BOM {
			v.Encoding[i] += " BOM"
		}
		v.Modified[i] = ses.Modified(side)
		v.LineCount[i] = len(d.Lines)
	}
	v.CanUndo, v.CanRedo = ses.CanUndo(), ses.CanRedo()
	return v
}

// Open compares two files and returns the view of a new compare window.
func (c *CompareFiles) Open(left, right string, o CmpOptions) (*CmpView, error) {
	if c.app.mock != nil {
		return nil, errors.New("This is the demo: its files are not real, so they can't be compared.")
	}
	s := &cmpSession{left: left, right: right, opts: o}
	if err := s.load(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.next++
	id := c.next
	c.sess[id] = s
	c.mu.Unlock()
	return s.view(id), nil
}

// Close ends a compare window's session (unsaved edits are dropped; the page asks first).
func (c *CompareFiles) Close(id int) {
	c.mu.Lock()
	delete(c.sess, id)
	c.mu.Unlock()
}

// with runs f on a session and returns the new view.
func (c *CompareFiles) with(id int, text bool, f func(s *cmpSession) error) (*CmpView, error) {
	s, err := c.get(id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if text && s.s == nil {
		return nil, errors.New("This works on text only, and the files are compared byte by byte.")
	}
	if err := f(s); err != nil {
		return nil, err
	}
	return s.view(id), nil
}

// View is the current view of a compare window.
func (c *CompareFiles) View(id int) (*CmpView, error) {
	return c.with(id, false, func(*cmpSession) error { return nil })
}

// SetOptions changes the options; switching to or from binary reads the files again.
func (c *CompareFiles) SetOptions(id int, o CmpOptions) (*CmpView, error) {
	return c.with(id, false, func(s *cmpSession) error {
		old := s.opts
		s.opts = o
		if s.s != nil && !o.Binary && old.ANSI == o.ANSI {
			s.s.SetOptions(o.engine())
			return nil
		}
		if s.s != nil && (s.s.Modified(compare.Left) || s.s.Modified(compare.Right)) {
			s.opts = old
			return errors.New("Save or undo the changes first.")
		}
		if err := s.load(); err != nil {
			s.opts = old
			_ = s.load()
			return err
		}
		return nil
	})
}

// Reopen compares the files again, read from disk; with new paths it compares other files (edits are dropped).
func (c *CompareFiles) Reopen(id int, left, right string) (*CmpView, error) {
	return c.with(id, false, func(s *cmpSession) error {
		ol, or := s.left, s.right
		s.left, s.right = left, right
		if err := s.load(); err != nil {
			s.left, s.right = ol, or
			_ = s.load()
			return err
		}
		return nil
	})
}

func cmpSide(side int) compare.Side {
	if side == 1 {
		return compare.Right
	}
	return compare.Left
}

// lineAt is the line of a side a row shows, or where a line typed into its empty cell goes.
func lineAt(ses *compare.Session, side compare.Side, row int) (int, bool) {
	rows := ses.Rows()
	pick := func(r compare.Row) int {
		if side == compare.Right {
			return r.Right
		}
		return r.Left
	}
	if row >= 0 && row < len(rows) && pick(rows[row]) >= 0 {
		return pick(rows[row]), true
	}
	for i := row + 1; i < len(rows); i++ {
		if l := pick(rows[i]); l >= 0 {
			return l, false
		}
	}
	return len(ses.Doc(side).Lines), false
}

// SetLine replaces the text a row shows on one side; in an empty cell the text is inserted there. Line breaks in the
// text make more lines.
func (c *CompareFiles) SetLine(id, side, row int, text string) (*CmpView, error) {
	return c.with(id, true, func(s *cmpSession) error {
		sd := cmpSide(side)
		parts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
		line, exists := lineAt(s.s, sd, row)
		if exists {
			if err := s.s.SetLine(sd, line, parts[0]); err != nil {
				return err
			}
			if len(parts) > 1 {
				return s.s.InsertLines(sd, line+1, parts[1:]...)
			}
			return nil
		}
		return s.s.InsertLines(sd, line, parts...)
	})
}

// DeleteLine removes the line a row shows on one side.
func (c *CompareFiles) DeleteLine(id, side, row int) (*CmpView, error) {
	return c.with(id, true, func(s *cmpSession) error {
		sd := cmpSide(side)
		line, exists := lineAt(s.s, sd, row)
		if !exists {
			return nil
		}
		return s.s.DeleteLines(sd, line, 1)
	})
}

// CopyBlock copies a difference block from one side (0 left, 1 right) over the other side.
func (c *CompareFiles) CopyBlock(id, block, from int) (*CmpView, error) {
	return c.with(id, true, func(s *cmpSession) error { return s.s.CopyBlock(block, cmpSide(from)) })
}

// CopyAll copies every difference from one side over the other side.
func (c *CompareFiles) CopyAll(id, from int) (*CmpView, error) {
	return c.with(id, true, func(s *cmpSession) error { return s.s.CopyAllBlocks(cmpSide(from)) })
}

// Undo takes back the last edit.
func (c *CompareFiles) Undo(id int) (*CmpView, error) {
	return c.with(id, true, func(s *cmpSession) error { s.s.Undo(); return nil })
}

// Redo does the last undone edit again.
func (c *CompareFiles) Redo(id int) (*CmpView, error) {
	return c.with(id, true, func(s *cmpSession) error { s.s.Redo(); return nil })
}

// Save writes one side back to its file, in its encoding and with its line endings.
func (c *CompareFiles) Save(id, side int) (*CmpView, error) {
	return c.with(id, true, func(s *cmpSession) error {
		sd := cmpSide(side)
		if err := s.s.Save(sd, ""); err != nil {
			return fmt.Errorf("Cannot write the file %s: %v", s.s.Doc(sd).Path, cmpCause(err))
		}
		return nil
	})
}

func cmpCause(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// SetEncoding reads one side again in another encoding ("Auto" detects it). Unsaved edits are refused unless force.
func (c *CompareFiles) SetEncoding(id, side int, enc string, force bool) (*CmpView, error) {
	return c.with(id, true, func(s *cmpSession) error {
		e, err := compare.ParseEncoding(enc)
		if err != nil {
			return fmt.Errorf("Unknown encoding %s.", enc)
		}
		err = s.s.ReloadSide(cmpSide(side), e, s.opts.detect(), force)
		if errors.Is(err, compare.ErrModified) {
			return errors.New("This side has unsaved changes. Save or undo them first.")
		}
		return err
	})
}

// Encodings lists the encodings the encoding menu offers.
func (c *CompareFiles) Encodings() []string {
	out := []string{compare.EncAuto.String()}
	for _, e := range compare.Encodings() {
		out = append(out, e.String())
	}
	return out
}

// Bytes reads up to n bytes of both files from offset, for the hex view.
func (c *CompareFiles) Bytes(id int, offset int64, n int) (*CmpBytes, error) {
	s, err := c.get(id)
	if err != nil {
		return nil, err
	}
	if n <= 0 || n > 1<<20 {
		n = 1 << 16
	}
	read := func(p string) ([]byte, error) {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		buf := make([]byte, n)
		k, err := f.ReadAt(buf, offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		return buf[:k], nil
	}
	s.mu.Lock()
	l, r := s.left, s.right
	s.mu.Unlock()
	a, err := read(l)
	if err != nil {
		return nil, err
	}
	b, err := read(r)
	if err != nil {
		return nil, err
	}
	return &CmpBytes{A: a, B: b}, nil
}

// Same says whether two files have the same bytes (the quick check before the window opens).
func (c *CompareFiles) Same(left, right string) (bool, error) {
	if _, err := cmpCheckFile(left); err != nil {
		return false, err
	}
	if _, err := cmpCheckFile(right); err != nil {
		return false, err
	}
	return compare.FilesEqual(context.Background(), left, right)
}

// ExternalStart starts the external compare program the user set (%1 = left file, %2 = right file).
func (c *CompareFiles) ExternalStart(template, left, right string) error {
	if c.app.mock != nil {
		return errors.New("This is the demo: its files are not real, so they can't be compared.")
	}
	args, err := compare.ExternalCommand(template, left, right)
	if err != nil {
		return fmt.Errorf("The compare program setting is not valid: %v", err)
	}
	if len(args) == 0 {
		return errors.New("No compare program is set.")
	}
	cmd := exec.Command(args[0], args[1:]...)
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("The compare program %s could not be started: %v", args[0], cmpCause(err))
	}
	go cmd.Wait()
	return nil
}
