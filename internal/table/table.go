// Package table reads files that hold rows and columns (CSV, TSV, JSON Lines, Parquet and SQLite databases) into a small grid
// the Lister can show as a table (board issue #26, F-NEW-10). Every reader only reads: files are opened by the caller
// for reading, and a SQLite database is parsed straight from its bytes, so nothing is ever written, locked or created
// next to it (no journal, no -shm file). Only the first MaxRows rows are read, so a file of any size opens quickly; the
// grid says when there were more. Each column gets a name and a type: the declared type of a SQLite column, or one
// guessed from the values for the text formats (integer, number, boolean, date, text).
package table

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxRows is how many rows are read at most.
const MaxRows = 10000

// MaxCell caps the characters kept of one cell; longer values end with an ellipsis.
const MaxCell = 2000

// Column is one column of a grid.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Table is the grid read from a file. A nil cell is a SQL NULL (or a JSON key a line does not have).
type Table struct {
	Format    string      `json:"format"` // "csv", "tsv", "jsonl", "sqlite" or "parquet"
	Columns   []Column    `json:"columns"`
	Rows      [][]*string `json:"-"`
	Truncated bool        `json:"truncated"` // there are more rows than were read
	Total     int64       `json:"total"`     // when Truncated: how many rows the file has (0 = not known)
	Note      string      `json:"note"`      // something the user should know (rows that could not be read, a -wal file)
}

// Kind says which table format a file is in, from its name and its first bytes; "" when it is none of them. A SQLite
// database and a Parquet file are known by their first bytes whatever their name.
func Kind(name string, sample []byte) string {
	if IsSQLite(sample) {
		return "sqlite"
	}
	if IsParquet(sample) {
		return "parquet"
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".csv":
		return "csv"
	case ".tsv", ".tab":
		return "tsv"
	case ".jsonl", ".ndjson":
		return "jsonl"
	}
	return ""
}

// IsSQLite says whether the bytes start with the header of a SQLite 3 database.
func IsSQLite(sample []byte) bool { return bytes.HasPrefix(sample, []byte(sqliteMagic)) }

// clip shortens a cell to MaxCell characters and makes sure it is valid UTF-8.
func clip(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	if len(s) <= MaxCell {
		return s
	}
	n := 0
	for i := range s {
		if n == MaxCell {
			return s[:i] + "…"
		}
		n++
	}
	return s
}

func strp(s string) *string { return &s }

var dateRx = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}([ T]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?)?$`)

// guess names the type of one text value: "" for an empty cell, else integer, number, boolean, date or text.
func guess(v string) string {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		return ""
	case isInteger(v):
		return "integer"
	case isNumber(v):
		return "number"
	case strings.EqualFold(v, "true") || strings.EqualFold(v, "false"):
		return "boolean"
	case dateRx.MatchString(v):
		return "date"
	}
	return "text"
}

func isInteger(v string) bool {
	_, err := strconv.ParseInt(v, 10, 64)
	return err == nil
}

func isNumber(v string) bool {
	if strings.ContainsAny(v, "xXpP_") || strings.EqualFold(v, "nan") || strings.Contains(strings.ToLower(v), "inf") {
		return false // only plain decimal numbers count, not 0x1p3, 1_000, NaN or Inf
	}
	_, err := strconv.ParseFloat(v, 64)
	return err == nil
}

// merge combines the type so far with the type of one more value: integers and numbers make numbers, anything else
// that differs makes text.
func merge(have, next string) string {
	switch {
	case next == "" || have == next:
		return have
	case have == "":
		return next
	case (have == "integer" && next == "number") || (have == "number" && next == "integer"):
		return "number"
	}
	return "text"
}

// inferTypes fills in the type of every column from its values (columns with no values at all are "text").
func inferTypes(t *Table) {
	types := make([]string, len(t.Columns))
	for _, r := range t.Rows {
		for i, c := range r {
			if i < len(types) && c != nil && types[i] != "text" {
				types[i] = merge(types[i], guess(*c))
			}
		}
	}
	for i := range t.Columns {
		if types[i] == "" {
			types[i] = "text"
		}
		t.Columns[i].Type = types[i]
	}
}
