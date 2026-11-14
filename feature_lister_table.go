package main

// The Lister's table view (board issue #26, F-NEW-10): CSV, TSV, JSON Lines and SQLite files shown as a grid with column
// names and types, in the Lister window and in quick view. It is an extra mode (key T, Options > Table); files open in
// it by themselves only when the Lister option "Open CSV, TSV, JSONL and SQLite files as a table" is on, which it is
// not by default, so the Lister behaves as before. The page asks for the grid's shape with Table and then for the rows
// it scrolls to with TableRows, so only what is on screen is drawn. internal/table does the reading: the first 10,000
// rows, from the file opened read-only by the Lister session; a SQLite database is parsed from its bytes and never
// opened with SQLite, so nothing is written or locked.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"mormicommander/internal/lister"
	"mormicommander/internal/table"
)

// listerTableDecodeLimit caps how much of a text table in an encoding other than UTF-8 (UTF-16, a code page) is
// decoded; UTF-8 files are read as a stream and stop at the row cap instead.
const listerTableDecodeLimit = 64 << 20

// listerTableRowsLimit caps the rows of one TableRows call.
const listerTableRowsLimit = 2000

// ListerTable is the shape of the grid: its columns and how many rows there are.
type ListerTable struct {
	Format    string             `json:"format"` // csv, tsv, jsonl or sqlite
	Columns   []table.Column     `json:"columns"`
	Rows      int                `json:"rows"`      // rows read
	Truncated bool               `json:"truncated"` // the file has more rows than were read
	Limit     int                `json:"limit"`     // the most rows that are read
	Note      string             `json:"note"`
	Tables    []table.TableEntry `json:"tables"`  // SQLite: the tables of the database
	Current   string             `json:"current"` // SQLite: the table shown
}

// listerTableState is what a session keeps of its grid.
type listerTableState struct {
	grid    *table.Table
	tables  []table.TableEntry
	current string
}

// tableKind is the table format of a session's file ("" when it is not one).
func (s *listerSession) tableKind() string {
	sample := make([]byte, 100)
	n, _ := s.file.ReadAt(sample, 0)
	return table.Kind(s.path, sample[:n])
}

// Table reads the file as a table (again, when name picks another table of a SQLite database) and returns its shape.
func (l *Lister) Table(id int, name string) (ListerTable, error) {
	s, err := l.session(id)
	if err != nil {
		return ListerTable{}, err
	}
	s.mu.Lock()
	st := s.table
	s.mu.Unlock()
	if st == nil || (name != "" && name != st.current) {
		st, err = s.loadTable(name)
		if err != nil {
			return ListerTable{}, err
		}
		s.mu.Lock()
		s.table = st
		s.mu.Unlock()
	}
	g := st.grid
	return ListerTable{Format: g.Format, Columns: g.Columns, Rows: len(g.Rows), Truncated: g.Truncated, Limit: table.MaxRows,
		Note: g.Note, Tables: st.tables, Current: st.current}, nil
}

// TableRows returns count rows of the grid from row start (a nil cell is a NULL).
func (l *Lister) TableRows(id, start, count int) ([][]*string, error) {
	s, err := l.session(id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	st := s.table
	s.mu.Unlock()
	if st == nil {
		return nil, errors.New("The table is not read yet.")
	}
	rows := st.grid.Rows
	start = max(0, min(start, len(rows)))
	end := min(len(rows), start+max(0, min(count, listerTableRowsLimit)))
	out := rows[start:end]
	if out == nil {
		out = [][]*string{}
	}
	return out, nil
}

// loadTable reads the grid of the session's file; for SQLite, name is the table ("" = the first one that can be read).
func (s *listerSession) loadTable(name string) (*listerTableState, error) {
	kind := s.tableKind()
	switch kind {
	case "sqlite":
		return s.loadSQLite(name)
	case "csv", "tsv", "jsonl":
		r, cut, err := s.tableText()
		if err != nil {
			return nil, err
		}
		var g *table.Table
		if kind == "jsonl" {
			g, err = table.ReadJSONL(r)
		} else if kind == "tsv" {
			g, err = table.ReadDelimited(r, '\t')
		} else {
			g, err = table.ReadDelimited(r, ',')
		}
		if err != nil {
			return nil, fmt.Errorf("The file can't be shown as a table: %v.", err)
		}
		if cut && !g.Truncated {
			g.Truncated = true
		}
		return &listerTableState{grid: g}, nil
	}
	return nil, errors.New("This file is not a table: CSV, TSV, JSON Lines (JSONL) and SQLite files can be shown as one.")
}

// tableText gives the text of a CSV, TSV or JSONL file as UTF-8: the file itself when it is UTF-8 (or plain ASCII),
// else the start of it decoded from its encoding (cut tells that the decoded part ends before the file does).
func (s *listerSession) tableText() (io.Reader, bool, error) {
	s.mu.Lock()
	enc := s.raw.Encoding()
	s.mu.Unlock()
	size := s.file.Size()
	if enc == lister.UTF8 {
		return io.NewSectionReader(s.file, 0, size), false, nil
	}
	n := min(size, listerTableDecodeLimit)
	b := make([]byte, n)
	got, err := s.file.ReadAt(b, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, listerOpenError(s.path, err)
	}
	b = b[:got]
	if e, k := lister.BOM(b); k > 0 {
		enc, b = e, b[k:]
	}
	text := lister.DecodeString(enc, b)
	cut := n < size
	if cut {
		// the last line is likely cut in two: leave it out
		if i := strings.LastIndexByte(text, '\n'); i >= 0 {
			text = text[:i+1]
		}
	}
	return strings.NewReader(text), cut, nil
}

// loadSQLite reads one table of a SQLite database.
func (s *listerSession) loadSQLite(name string) (*listerTableState, error) {
	db, err := table.OpenSQLite(s.file, s.file.Size())
	if err != nil {
		return nil, fmt.Errorf("The database can't be read: %v.", err)
	}
	st := &listerTableState{tables: db.Tables()}
	if name == "" {
		for _, e := range st.tables {
			if e.Readable {
				name = e.Name
				break
			}
		}
	}
	if name == "" {
		if len(st.tables) == 0 {
			st.grid = &table.Table{Format: "sqlite", Note: "The database has no tables."}
		} else {
			st.grid = &table.Table{Format: "sqlite", Note: "None of the tables of this database can be shown."}
		}
		return st, nil
	}
	g, err := db.Read(name)
	if err != nil {
		g = &table.Table{Format: "sqlite", Note: fmt.Sprintf("The table %s can't be shown: %v.", name, err)}
	}
	if db.WAL() {
		if fi, err := os.Stat(s.path + "-wal"); err == nil && fi.Size() > 0 {
			g.Note = strings.TrimSpace(g.Note + " Changes still in the database's -wal file are not shown.")
		}
	}
	st.grid, st.current = g, name
	return st, nil
}
