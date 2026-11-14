// Tests of the Lister's table view (feature_lister_table.go): a CSV, a UTF-16 TSV and a JSONL file in temp folders
// open with the table kind set and read as grids through Table and TableRows; a SQLite database (copied from
// internal/table/testdata) lists its tables and switches between them; reading it leaves the file unchanged and
// creates nothing next to it; a file that is not a table says so; and Reload reads a changed file again.

package main

import (
	"bytes"
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
	if err != nil || tb.Rows != 0 || !strings.Contains(tb.Note, "WITHOUT ROWID") {
		t.Fatalf("kv: %+v %v", tb, err)
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
