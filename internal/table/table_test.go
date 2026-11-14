// Tests of the table readers: CSV and TSV (header, loose quotes, ragged rows, the row cap, guessed types), JSON Lines
// (keys in order, mixed values, broken lines), the CREATE TABLE parser, and the SQLite reader against two small
// databases in testdata made with the sqlite3 tool (a multi-level b-tree with 512-byte pages, overflow pages, blobs,
// an added column, quoted names, a virtual generated column, WITHOUT ROWID, a view, and a UTF-16 database). Damaged
// databases are made by cutting and scribbling over a copy in memory; they must give an error, never a crash or a hang.

package table

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func cell(t *testing.T, tb *Table, r, c int) string {
	t.Helper()
	if r >= len(tb.Rows) || c >= len(tb.Rows[r]) || tb.Rows[r][c] == nil {
		return "<nil>"
	}
	return *tb.Rows[r][c]
}

func colNames(tb *Table) string {
	var s []string
	for _, c := range tb.Columns {
		s = append(s, c.Name+":"+c.Type)
	}
	return strings.Join(s, ",")
}

func TestKind(t *testing.T) {
	cases := map[string]string{"a.csv": "csv", "A.CSV": "csv", "b.tsv": "tsv", "b.tab": "tsv", "c.jsonl": "jsonl", "c.ndjson": "jsonl", "d.txt": "", "e.json": ""}
	for name, want := range cases {
		if got := Kind(name, []byte("x,y\n")); got != want {
			t.Errorf("Kind(%q) = %q, want %q", name, got, want)
		}
	}
	if got := Kind("whatever.bin", []byte(sqliteMagic+"rest")); got != "sqlite" {
		t.Errorf("a SQLite header is %q", got)
	}
}

func TestReadCSV(t *testing.T) {
	src := "\xef\xbb\xbfid,name,price,ok,when\n1,apple,1.5,true,2024-01-02\n2,\"pear, green\",2,false,2024-01-03T10:00:00Z\n3,bad\"quote,x,TRUE,\n4,extra,3,true,2024-02-01,more\n"
	tb, err := ReadDelimited(strings.NewReader(src), ',')
	if err != nil {
		t.Fatal(err)
	}
	if got := colNames(tb); got != "id:integer,name:text,price:text,ok:boolean,when:date,Column 6:text" {
		t.Errorf("columns: %s", got)
	}
	if len(tb.Rows) != 4 || cell(t, tb, 1, 1) != "pear, green" || cell(t, tb, 2, 1) != `bad"quote` || cell(t, tb, 3, 5) != "more" {
		t.Errorf("rows: %d %q %q %q", len(tb.Rows), cell(t, tb, 1, 1), cell(t, tb, 2, 1), cell(t, tb, 3, 5))
	}
	if tb.Truncated || tb.Format != "csv" {
		t.Errorf("truncated %v format %q", tb.Truncated, tb.Format)
	}
}

func TestReadCSVNumbers(t *testing.T) {
	tb, err := ReadDelimited(strings.NewReader("a,b,c\n1,1,0x10\n2.5,,NaN\n"), ',')
	if err != nil {
		t.Fatal(err)
	}
	if got := colNames(tb); got != "a:number,b:integer,c:text" {
		t.Errorf("columns: %s", got)
	}
}

func TestReadTSVAndCap(t *testing.T) {
	var b strings.Builder
	b.WriteString("n\tsquare\n")
	for i := 0; i < MaxRows+5; i++ {
		fmt.Fprintf(&b, "%d\t%d\n", i, i*i)
	}
	tb, err := ReadDelimited(strings.NewReader(b.String()), '\t')
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Rows) != MaxRows || !tb.Truncated || tb.Format != "tsv" {
		t.Fatalf("rows %d truncated %v", len(tb.Rows), tb.Truncated)
	}
	if cell(t, tb, MaxRows-1, 1) != fmt.Sprint((MaxRows-1)*(MaxRows-1)) {
		t.Errorf("last row: %q", cell(t, tb, MaxRows-1, 1))
	}
	exact, _ := ReadDelimited(strings.NewReader("n\n"+strings.Repeat("1\n", MaxRows)), '\t')
	if len(exact.Rows) != MaxRows || exact.Truncated {
		t.Errorf("exactly MaxRows rows: %d truncated %v", len(exact.Rows), exact.Truncated)
	}
}

func TestReadEmptyAndLongCell(t *testing.T) {
	tb, err := ReadDelimited(strings.NewReader(""), ',')
	if err != nil || len(tb.Columns) != 0 || len(tb.Rows) != 0 {
		t.Fatalf("empty: %v %+v", err, tb)
	}
	long := strings.Repeat("é", MaxCell+50)
	tb, _ = ReadDelimited(strings.NewReader("a\n"+long+"\n"), ',')
	if c := cell(t, tb, 0, 0); len([]rune(c)) != MaxCell+1 || !strings.HasSuffix(c, "…") {
		t.Errorf("long cell kept %d characters", len([]rune(c)))
	}
}

func TestReadJSONL(t *testing.T) {
	src := "{\"b\":1,\"a\":\"x\"}\n\n{\"a\":\"y\",\"c\":{\"d\":[1,2]},\"b\":2.5}\n{\"a\":null,\"b\":true}\n[1,2]\nnot json\n"
	tb, err := ReadJSONL(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := colNames(tb); got != "b:mixed,a:text,c:object,value:mixed" {
		t.Errorf("columns: %s", got)
	}
	if len(tb.Rows) != 5 {
		t.Fatalf("rows: %d", len(tb.Rows))
	}
	if cell(t, tb, 1, 2) != `{"d":[1,2]}` || cell(t, tb, 2, 1) != "<nil>" || cell(t, tb, 2, 0) != "true" || cell(t, tb, 3, 3) != "[1,2]" || cell(t, tb, 4, 3) != "not json" {
		t.Errorf("cells: %q %q %q %q %q", cell(t, tb, 1, 2), cell(t, tb, 2, 1), cell(t, tb, 2, 0), cell(t, tb, 3, 3), cell(t, tb, 4, 3))
	}
	if !strings.Contains(tb.Note, "1 lines") {
		t.Errorf("note: %q", tb.Note)
	}
}

func TestParseCreateTable(t *testing.T) {
	cols, wr, ok := parseCreateTable(`CREATE TABLE "t" ( -- comment
		id integer primary key, /* c */ "na""me" varchar ( 10 , 2 ) not null, [x y] DEFAULT 1,
		g INT AS (id*2), s INT GENERATED ALWAYS AS (id) STORED, CONSTRAINT c UNIQUE (x) )`)
	if !ok || wr {
		t.Fatalf("ok %v wr %v", ok, wr)
	}
	want := []schemaCol{{"id", "integer", true, true, false}, {`na"me`, "varchar(10, 2)", true, false, false}, {"x y", "", true, false, false}, {"g", "INT", false, false, false}, {"s", "INT", true, false, false}}
	if fmt.Sprint(cols) != fmt.Sprint(want) {
		t.Errorf("got  %v\nwant %v", cols, want)
	}
	cols, _, _ = parseCreateTable("CREATE TABLE p (a INTEGER, b TEXT, PRIMARY KEY (a))")
	if !cols[0].rowid || cols[1].rowid {
		t.Errorf("table primary key: %v", cols)
	}
	cols, _, _ = parseCreateTable("CREATE TABLE p (a INTEGER PRIMARY KEY DESC)")
	if cols[0].rowid {
		t.Error("INTEGER PRIMARY KEY DESC is not the rowid")
	}
	cols, _, _ = parseCreateTable("CREATE TABLE p (a INTEGER, b INT, PRIMARY KEY (a, b))")
	if cols[0].rowid {
		t.Error("a two-column key is not the rowid")
	}
	if _, wr, _ := parseCreateTable("CREATE TABLE k (a TEXT PRIMARY KEY) WITHOUT ROWID"); !wr {
		t.Error("WITHOUT ROWID not seen")
	}
	if _, _, ok := parseCreateTable("CREATE TABLE c AS SELECT 1"); ok {
		t.Error("AS SELECT has no column list")
	}
}

func openTestDB(t *testing.T, name string) *DB {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	d, err := OpenSQLite(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSQLiteTables(t *testing.T) {
	d := openTestDB(t, "sample.sqlite")
	var got []string
	for _, e := range d.Tables() {
		got = append(got, fmt.Sprintf("%s:%v", e.Name, e.Readable))
	}
	if s := strings.Join(got, ","); s != `auto:true,copy:true,kv:false,odd "name":true,people:true,sqlite_sequence:true` {
		t.Errorf("tables: %s", s)
	}
	if d.WAL() {
		t.Error("not a WAL database")
	}
	if _, err := d.Read("kv"); err == nil || !strings.Contains(err.Error(), "WITHOUT ROWID") {
		t.Errorf("kv: %v", err)
	}
	if _, err := d.Read("v_people"); err == nil {
		t.Error("a view is not a table")
	}
}

func TestSQLitePeople(t *testing.T) {
	d := openTestDB(t, "sample.sqlite")
	tb, err := d.Read("people")
	if err != nil {
		t.Fatal(err)
	}
	if got := colNames(tb); got != "id:INTEGER,name:TEXT,score:REAL,photo:BLOB,note:VARCHAR(20),extra:TEXT" {
		t.Errorf("columns: %s", got)
	}
	if len(tb.Rows) != 1500 || tb.Truncated || tb.Note != "" {
		t.Fatalf("rows %d truncated %v note %q", len(tb.Rows), tb.Truncated, tb.Note)
	}
	for i, r := range tb.Rows {
		if *r[0] != fmt.Sprint(i+1) || *r[1] != fmt.Sprintf("person %d", i+1) {
			t.Fatalf("row %d: %q %q", i, *r[0], *r[1])
		}
	}
	if cell(t, tb, 0, 2) != "1.5" || cell(t, tb, 3, 2) != "-42.0" || cell(t, tb, 4, 2) != "3.0" {
		t.Errorf("scores: %q %q %q", cell(t, tb, 0, 2), cell(t, tb, 3, 2), cell(t, tb, 4, 2))
	}
	if cell(t, tb, 6, 4) != "<nil>" || cell(t, tb, 0, 4) != "n1" || cell(t, tb, 0, 3) != "<nil>" {
		t.Errorf("nulls: %q %q %q", cell(t, tb, 6, 4), cell(t, tb, 0, 4), cell(t, tb, 0, 3))
	}
	if long := cell(t, tb, 2, 4); len([]rune(long)) != MaxCell+1 || !strings.HasPrefix(long, "long-long") || !strings.HasSuffix(long, "long-…") {
		t.Errorf("overflowing text: %d characters", len([]rune(long)))
	}
	if b := cell(t, tb, 99, 3); !strings.HasPrefix(b, "x'") || !strings.HasSuffix(b, "(40 bytes)") {
		t.Errorf("blob: %q", b)
	}
	if cell(t, tb, 5, 5) != "x" || cell(t, tb, 0, 5) != "<nil>" {
		t.Errorf("added column: %q %q", cell(t, tb, 5, 5), cell(t, tb, 0, 5))
	}
}

func TestSQLiteOddTables(t *testing.T) {
	d := openTestDB(t, "sample.sqlite")
	tb, err := d.Read(`odd "name"`)
	if err != nil {
		t.Fatal(err)
	}
	if got := colNames(tb); got != "first col:INT,second:TEXT,total:INT,last:TEXT" {
		t.Errorf("columns: %s", got)
	}
	if cell(t, tb, 0, 0) != "-1" || cell(t, tb, 0, 2) != "<nil>" || cell(t, tb, 0, 3) != "end" || cell(t, tb, 1, 0) != "9223372036854775807" || cell(t, tb, 1, 1) != "<nil>" {
		t.Errorf("cells: %v", tb.Rows)
	}
	tb, err = d.Read("copy")
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Rows) != 3 || cell(t, tb, 2, 1) != "person 3" {
		t.Errorf("copy: %s %d", colNames(tb), len(tb.Rows))
	}
	tb, _ = d.Read("auto")
	if cell(t, tb, 0, 0) != "1" || cell(t, tb, 0, 1) != "one" {
		t.Errorf("auto: %v", tb.Rows)
	}
}

func TestSQLiteUTF16(t *testing.T) {
	d := openTestDB(t, "utf16.sqlite")
	tb, err := d.Read("words")
	if err != nil {
		t.Fatal(err)
	}
	if cell(t, tb, 0, 0) != "árvíztűrő" || cell(t, tb, 1, 0) != "日本" || cell(t, tb, 1, 1) != "2" {
		t.Errorf("cells: %q %q %q", cell(t, tb, 0, 0), cell(t, tb, 1, 0), cell(t, tb, 1, 1))
	}
}

func TestSQLiteDamaged(t *testing.T) {
	orig, err := os.ReadFile("testdata/sample.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSQLite(bytes.NewReader([]byte("not a database at all")), 21); err == nil {
		t.Error("garbage opened")
	}
	// cut short: the schema may still open, but reading must stop with an error or a note
	for _, n := range []int{100, 512, 4096, len(orig) / 2} {
		b := orig[:n]
		d, err := OpenSQLite(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			continue
		}
		if tb, err := d.Read("people"); err == nil && tb.Note == "" && len(tb.Rows) == 1500 {
			t.Errorf("cut at %d: read everything", n)
		}
	}
	// scribbled over: every page byte pattern must end in a result or an error, quickly
	for seed := 0; seed < 200; seed++ {
		b := append([]byte(nil), orig...)
		for k := 0; k < 20; k++ {
			p := 100 + (seed*7919+k*104729)%(len(b)-100)
			b[p] ^= byte(seed*31 + k + 1)
		}
		d, err := OpenSQLite(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			continue
		}
		for _, e := range d.Tables() {
			d.Read(e.Name)
		}
	}
	// a page that points to itself must not loop
	b := append([]byte(nil), orig...)
	b[100] = 5                                            // page 1 becomes an interior page...
	b[100+8], b[100+9], b[100+10], b[100+11] = 0, 0, 0, 1 // ...whose right-most child is page 1
	if _, err := OpenSQLite(bytes.NewReader(b), int64(len(b))); err == nil {
		t.Error("a loop in the b-tree was not noticed")
	}
}

func TestVarint(t *testing.T) {
	cases := []struct {
		b []byte
		v uint64
		n int
	}{{[]byte{0x05}, 5, 1}, {[]byte{0x81, 0x00}, 128, 2}, {[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, ^uint64(0), 9}, {[]byte{0x81}, 0, 0}}
	for _, c := range cases {
		if v, n := varint(c.b); v != c.v || n != c.n {
			t.Errorf("varint(%x) = %d, %d", c.b, v, n)
		}
	}
}
