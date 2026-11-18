package main

// The Lister's table view (board issue #26, F-NEW-10): CSV, TSV, JSON Lines, Parquet and SQLite files shown as a grid with column
// names and types, in the Lister window and in quick view. It is an extra mode (key T, Options > Table); files open in
// it by themselves only when the Lister option "Open CSV, TSV, JSONL, Parquet and SQLite files as a table" is on, which it is
// not by default, so the Lister behaves as before. The page asks for the grid's shape with Table and then for the rows
// it scrolls to with TableRows, so only what is on screen is drawn, and searches the cells with TableFind. internal/table
// does the reading: the first 10,000 rows, from the file opened read-only by the Lister session (and how many rows
// there are in all, when that can be found quickly); a SQLite database and a Parquet file are parsed from their bytes,
// never opened with another library, so nothing is written or locked.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
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
	Total     int64              `json:"total"`     // when truncated: the rows the file has (0 = not known)
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
	return ListerTable{Format: g.Format, Columns: g.Columns, Rows: len(g.Rows), Truncated: g.Truncated, Total: g.Total, Limit: table.MaxRows,
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

// ListerTableQuery is a search in the grid's cells.
type ListerTableQuery struct {
	Pattern       string `json:"pattern"`
	CaseSensitive bool   `json:"caseSensitive"`
	WholeCell     bool   `json:"wholeCell"` // the cell must be the text, not just hold it
	Regex         bool   `json:"regex"`
	Backward      bool   `json:"backward"`
}

// ListerTableMatch is where a search found its text (Row and Col from 0).
type ListerTableMatch struct {
	Found bool `json:"found"`
	Row   int  `json:"row"`
	Col   int  `json:"col"`
}

// TableFind looks for the text in the cells of the grid, row by row and left to right, from the cell after (row, col)
// (before it when searching backward; row -1 starts at the top, or at the bottom backward). NULL cells hold no text.
func (l *Lister) TableFind(id int, q ListerTableQuery, row, col int) (ListerTableMatch, error) {
	s, err := l.session(id)
	if err != nil {
		return ListerTableMatch{}, err
	}
	s.mu.Lock()
	st := s.table
	s.mu.Unlock()
	if st == nil {
		return ListerTableMatch{}, errors.New("The table is not read yet.")
	}
	match, err := tableMatcher(q)
	if err != nil {
		return ListerTableMatch{}, err
	}
	rows := st.grid.Rows
	width := len(st.grid.Columns)
	if width == 0 || len(rows) == 0 {
		return ListerTableMatch{}, nil
	}
	pos := row*width + col // cells in reading order
	step := 1
	if q.Backward {
		step = -1
		if row < 0 {
			pos = len(rows) * width
		}
	} else if row < 0 {
		pos = -1
	}
	for pos += step; pos >= 0 && pos < len(rows)*width; pos += step {
		r, c := pos/width, pos%width
		if c < len(rows[r]) && rows[r][c] != nil && match(*rows[r][c]) {
			return ListerTableMatch{Found: true, Row: r, Col: c}, nil
		}
	}
	return ListerTableMatch{}, nil
}

// tableMatcher makes the test of one cell for a query.
func tableMatcher(q ListerTableQuery) (func(string) bool, error) {
	if q.Pattern == "" {
		return nil, errors.New("Type the text to search for.")
	}
	if q.Regex {
		expr := q.Pattern
		if q.WholeCell {
			expr = `^(?:` + expr + `)$`
		}
		if !q.CaseSensitive {
			expr = `(?i)` + expr
		}
		rx, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("The regular expression is not valid: %v.", err)
		}
		return rx.MatchString, nil
	}
	p := q.Pattern
	if !q.CaseSensitive {
		p = strings.ToLower(p)
	}
	return func(v string) bool {
		if !q.CaseSensitive {
			v = strings.ToLower(v)
		}
		if q.WholeCell {
			return v == p
		}
		return strings.Contains(v, p)
	}, nil
}

// loadTable reads the grid of the session's file; for SQLite, name is the table ("" = the first one that can be read).
func (s *listerSession) loadTable(name string) (*listerTableState, error) {
	kind := s.tableKind()
	switch kind {
	case "sqlite":
		return s.loadSQLite(name)
	case "parquet":
		g, err := table.ReadParquet(s.file, s.file.Size())
		if err != nil {
			return nil, fmt.Errorf("The file can't be shown as a table: %v.", err)
		}
		return &listerTableState{grid: g}, nil
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
		if cut {
			g.Truncated, g.Total = true, 0 // only the start of the file was decoded: the count is not known
		}
		return &listerTableState{grid: g}, nil
	}
	return nil, errors.New("This file is not a table: CSV, TSV, JSON Lines (JSONL), Parquet and SQLite files can be shown as one.")
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
