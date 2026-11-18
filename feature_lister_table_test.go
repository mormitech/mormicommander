// Tests of the Lister's table view (feature_lister_table.go): a CSV, a UTF-16 TSV and a JSONL file in temp folders
// open with the table kind set and read as grids through Table and TableRows; a SQLite database (copied from
// internal/table/testdata) lists its tables and switches between them; reading it leaves the file unchanged and
// creates nothing next to it; a Parquet file (internal/table/testdata) reads by its magic; a file of more than 10,000
// rows says how many it has; TableFind finds text in the cells forwards and backwards; a file that is not a table says
// so; and Reload reads a changed file again.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestListerTableCSV(t *testing.T) {
	p := writeTemp(t, "a.csv", "name,n\nalpha,1\nbeta,2\ngamma,3\n")
	l := newTestLister()
	info, err := l.Open(p, ListerOptions{Mode: "text"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info.ID)
	if info.Table != "csv" || info.Suggest != "text" {
		t.Fatalf("table %q suggest %q", info.Table, info.Suggest)
	}
	tb, err := l.Table(info.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if tb.Format != "csv" || tb.Rows != 3 || len(tb.Columns) != 2 || tb.Columns[1].Type != "integer" || tb.Limit != 10000 {
		t.Fatalf("table: %+v", tb)
	}
	rows, err := l.TableRows(info.ID, 1, 5)
	if err != nil || len(rows) != 2 || *rows[0][0] != "beta" || *rows[1][1] != "3" {
		t.Fatalf("rows: %v %v", rows, err)
	}
	if rows, _ := l.TableRows(info.ID, 99, 5); len(rows) != 0 {
		t.Fatalf("rows past the end: %v", rows)
	}
	os.WriteFile(p, []byte("name,n\nalpha,1\n"), 0o644)
	if c, err := l.Reload(info.ID); err != nil || !c.Changed {
		t.Fatalf("reload: %+v %v", c, err)
	}
	if tb, _ := l.Table(info.ID, ""); tb.Rows != 1 {
		t.Fatalf("after reload: %d rows", tb.Rows)
	}
}

func TestListerTableUTF16AndJSONL(t *testing.T) {
	u := utf16.Encode([]rune("név\tszám\nárvíztűrő\t1\n"))
	b := []byte{0xff, 0xfe}
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	p := filepath.Join(t.TempDir(), "w.tsv")
	os.WriteFile(p, b, 0o644)
	l := newTestLister()
	info, err := l.Open(p, ListerOptions{Mode: "text"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info.ID)
	tb, err := l.Table(info.ID, "")
	if err != nil || tb.Columns[0].Name != "név" || tb.Rows != 1 {
		t.Fatalf("utf-16 tsv: %+v %v", tb, err)
	}
	rows, _ := l.TableRows(info.ID, 0, 1)
	if *rows[0][0] != "árvíztűrő" {
		t.Fatalf("cell %q", *rows[0][0])
	}

	j := writeTemp(t, "e.jsonl", "{\"a\":1,\"b\":null}\n{\"a\":2,\"b\":\"x\"}\n")
	info2, err := l.Open(j, ListerOptions{Mode: "text"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info2.ID)
	tb, err = l.Table(info2.ID, "")
	if err != nil || info2.Table != "jsonl" || tb.Rows != 2 {
		t.Fatalf("jsonl: %+v %v", tb, err)
	}
	rows, _ = l.TableRows(info2.ID, 0, 2)
	if len(rows[0]) > 1 && rows[0][1] != nil {
		t.Fatalf("a JSON null is a NULL cell: %v", rows[0])
	}
}

func TestListerTableSQLite(t *testing.T) {
	src, err := os.ReadFile("internal/table/testdata/sample.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "data.bin") // known by its header, not its name
	os.WriteFile(p, src, 0o444)
	l := newTestLister()
	info, err := l.Open(p, ListerOptions{Mode: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Table != "sqlite" {
		t.Fatalf("table kind %q", info.Table)
	}
	tb, err := l.Table(info.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if tb.Current != "auto" || len(tb.Tables) != 6 || tb.Rows != 1 {
		t.Fatalf("first table: %+v", tb)
	}
	tb, err = l.Table(info.ID, "people")
	if err != nil || tb.Current != "people" || tb.Rows != 1500 || tb.Columns[0].Name != "id" {
		t.Fatalf("people: %+v %v", tb.Current, err)
	}
	rows, _ := l.TableRows(info.ID, 1499, 10)
	if len(rows) != 1 || *rows[0][1] != "person 1500" {
		t.Fatalf("last row: %v", rows)
	}
	tb, err = l.Table(info.ID, "kv")
	if err != nil || tb.Rows != 1 || tb.Note != "" {
		t.Fatalf("kv (WITHOUT ROWID): %+v %v", tb, err)
	}
	l.Close(info.ID)
	after, _ := os.ReadFile(p)
	if !bytes.Equal(after, src) {
		t.Fatal("the database changed")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Fatalf("files next to the database: %d", len(ents))
	}
}

func TestListerTableNotATable(t *testing.T) {
	p := writeTemp(t, "a.txt", "just text\n")
	l := newTestLister()
	info, err := l.Open(p, ListerOptions{Mode: "text"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info.ID)
	if info.Table != "" {
		t.Fatalf("table kind %q", info.Table)
	}
	if _, err := l.Table(info.ID, ""); err == nil || !strings.Contains(err.Error(), "not a table") {
		t.Fatalf("err %v", err)
	}
	if _, err := l.TableRows(info.ID, 0, 1); err == nil {
		t.Fatal("rows of no table")
	}
}

func TestListerTableParquetAndCount(t *testing.T) {
	src, err := os.ReadFile("internal/table/testdata/flat.parquet")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "data.bin") // known by its magic, not its name
	os.WriteFile(p, src, 0o444)
	l := newTestLister()
	info, err := l.Open(p, ListerOptions{Mode: "text"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info.ID)
	tb, err := l.Table(info.ID, "")
	if err != nil || info.Table != "parquet" || tb.Format != "parquet" || tb.Rows != 5 || tb.Columns[1].Type != "string" {
		t.Fatalf("parquet: %q %+v %v", info.Table, tb, err)
	}
	if after, _ := os.ReadFile(p); !bytes.Equal(after, src) {
		t.Fatal("the file changed")
	}

	var b strings.Builder
	b.WriteString("n\n")
	for i := range 12345 {
		fmt.Fprintf(&b, "%d\n", i)
	}
	c := writeTemp(t, "big.csv", b.String())
	info2, err := l.Open(c, ListerOptions{Mode: "text"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info2.ID)
	tb, err = l.Table(info2.ID, "")
	if err != nil || tb.Rows != 10000 || !tb.Truncated || tb.Total != 12345 {
		t.Fatalf("big csv: rows %d truncated %v total %d %v", tb.Rows, tb.Truncated, tb.Total, err)
	}
}

func TestListerTableFind(t *testing.T) {
	p := writeTemp(t, "f.csv", "name,city\nAnna,Pécs\nBéla,Győr\nanna,Budapest\nZoe,\n")
	l := newTestLister()
	info, err := l.Open(p, ListerOptions{Mode: "text"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info.ID)
	if _, err := l.TableFind(info.ID, ListerTableQuery{Pattern: "x"}, -1, 0); err == nil {
		t.Fatal("a search before the table is read")
	}
	l.Table(info.ID, "")
	find := func(q ListerTableQuery, row, col int) string {
		t.Helper()
		m, err := l.TableFind(info.ID, q, row, col)
		if err != nil {
			t.Fatal(err)
		}
		if !m.Found {
			return "-"
		}
		return fmt.Sprintf("%d,%d", m.Row, m.Col)
	}
	cases := []struct {
		q        ListerTableQuery
		row, col int
		want     string
	}{
		{ListerTableQuery{Pattern: "anna"}, -1, 0, "0,0"},
		{ListerTableQuery{Pattern: "anna"}, 0, 0, "2,0"},
		{ListerTableQuery{Pattern: "anna"}, 2, 0, "-"},
		{ListerTableQuery{Pattern: "anna", CaseSensitive: true}, -1, 0, "2,0"},
		{ListerTableQuery{Pattern: "anna", Backward: true}, 2, 0, "0,0"},
		{ListerTableQuery{Pattern: "anna", Backward: true}, -1, 0, "2,0"},
		{ListerTableQuery{Pattern: "GYŐR"}, 1, -1, "1,1"},
		{ListerTableQuery{Pattern: "Bud"}, -1, 0, "2,1"},
		{ListerTableQuery{Pattern: "Bud", WholeCell: true}, -1, 0, "-"},
		{ListerTableQuery{Pattern: "^b.*a$", Regex: true}, -1, 0, "1,0"},
		{ListerTableQuery{Pattern: "p.cs", Regex: true, WholeCell: true}, -1, 0, "0,1"},
		{ListerTableQuery{Pattern: "Zoe", Backward: true}, 3, 2, "3,0"},
	}
	for _, c := range cases {
		if got := find(c.q, c.row, c.col); got != c.want {
			t.Errorf("%+v from %d,%d: %s, want %s", c.q, c.row, c.col, got, c.want)
		}
	}
	if _, err := l.TableFind(info.ID, ListerTableQuery{Pattern: "(", Regex: true}, -1, 0); err == nil {
		t.Error("a bad regular expression is an error")
	}
	if _, err := l.TableFind(info.ID, ListerTableQuery{}, -1, 0); err == nil {
		t.Error("an empty search is an error")
	}
}
