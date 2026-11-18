// A small read-only reader of SQLite 3 database files, written from the published file format
// (https://www.sqlite.org/fileformat2.html) so no C library or driver is needed. It reads the schema table on page 1,
// lists the tables, and walks a table's b-tree in rowid order to decode its records. It never writes and never takes a
// lock, so a database another program has open is read as it is on disk; changes that are still only in a "-wal" file
// next to it are not seen (the caller can say so). Tables declared WITHOUT ROWID are stored in an index b-tree, which is
// walked in key order the same way; virtual tables are listed but not read. Every page number and length is checked, so a damaged or hostile file gives
// an error instead of a crash or an endless loop.

package table

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

const sqliteMagic = "SQLite format 3\x00"

// maxPayload caps how much of one record is read; columns past the cap show as NULL.
const maxPayload = 32 << 20

// maxDepth caps how deep a b-tree may go (real ones are a few levels deep).
const maxDepth = 40

// errDamaged is returned for anything in the file that breaks the format.
var errDamaged = errors.New("the database file is damaged or not a SQLite database")

// DB is an open SQLite database file.
type DB struct {
	r        io.ReaderAt
	pageSize int
	usable   int
	pages    uint32
	enc      uint32 // 1 UTF-8, 2 UTF-16le, 3 UTF-16be
	wal      bool
	objects  []sqlObject
}

// sqlObject is one row of the schema table (sqlite_schema).
type sqlObject struct {
	typ, name string
	root      uint32
	sql       string
}

// TableEntry is one table of a database, as the table list shows it.
type TableEntry struct {
	Name     string `json:"name"`
	Readable bool   `json:"readable"`
	Why      string `json:"why,omitempty"` // why it can't be shown
}

// OpenSQLite reads the header and the schema of a database of size bytes.
func OpenSQLite(r io.ReaderAt, size int64) (*DB, error) {
	hdr := make([]byte, 100)
	if _, err := r.ReadAt(hdr, 0); err != nil {
		return nil, errDamaged
	}
	if !IsSQLite(hdr) {
		return nil, errors.New("this is not a SQLite database")
	}
	ps := int(binary.BigEndian.Uint16(hdr[16:]))
	if ps == 1 {
		ps = 65536
	}
	if ps < 512 || ps > 65536 || ps&(ps-1) != 0 {
		return nil, errDamaged
	}
	d := &DB{r: r, pageSize: ps, usable: ps - int(hdr[20]), pages: uint32(size / int64(ps))}
	if d.usable < 480 || d.pages < 1 {
		return nil, errDamaged
	}
	d.enc = binary.BigEndian.Uint32(hdr[56:])
	if d.enc == 0 {
		d.enc = 1
	}
	if d.enc > 3 {
		return nil, errDamaged
	}
	d.wal = hdr[18] == 2 || hdr[19] == 2
	err := d.walk(1, func(_ int64, payload []byte) (bool, error) {
		v, err := d.record(payload)
		if err != nil {
			return false, err
		}
		if len(v) < 5 {
			return true, nil
		}
		o := sqlObject{typ: v[0].text(), name: v[1].text(), sql: v[4].text()}
		if v[3].kind == kindInt && v[3].i > 0 && v[3].i <= math.MaxUint32 {
			o.root = uint32(v[3].i)
		}
		d.objects = append(d.objects, o)
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// WAL says whether the database is in write-ahead-log mode (recent changes may then be in its -wal file).
func (d *DB) WAL() bool { return d.wal }

// Tables lists the tables: the user's first, by name, then SQLite's own (sqlite_sequence, sqlite_stat1 and so on).
func (d *DB) Tables() []TableEntry {
	var user, internal []TableEntry
	for _, o := range d.objects {
		if o.typ != "table" {
			continue
		}
		e := TableEntry{Name: o.name, Readable: true}
		up := strings.ToUpper(o.sql)
		switch {
		case strings.HasPrefix(strings.TrimSpace(up), "CREATE VIRTUAL"):
			e.Readable, e.Why = false, "This is a virtual table; its rows are made by a SQLite module, not stored in the file."
		case o.root == 0:
			e.Readable, e.Why = false, "This table has no data in the file."
		}
		if strings.HasPrefix(strings.ToLower(o.name), "sqlite_") {
			internal = append(internal, e)
		} else {
			user = append(user, e)
		}
	}
	by := func(l []TableEntry) {
		sort.SliceStable(l, func(i, j int) bool { return strings.ToLower(l[i].Name) < strings.ToLower(l[j].Name) })
	}
	by(user)
	by(internal)
	return append(user, internal...)
}

// Read reads the first MaxRows rows of a table.
func (d *DB) Read(name string) (*Table, error) {
	var obj *sqlObject
	for i := range d.objects {
		if d.objects[i].typ == "table" && d.objects[i].name == name {
			obj = &d.objects[i]
			break
		}
	}
	if obj == nil {
		return nil, fmt.Errorf("the database has no table named %q", name)
	}
	for _, e := range d.Tables() {
		if e.Name == name && !e.Readable {
			return nil, errors.New(strings.TrimSuffix(e.Why, "."))
		}
	}
	cols, withoutRowid, ok := parseCreateTable(obj.sql)
	// where each column is in the record: in declaration order, except that a WITHOUT ROWID table's record starts with
	// its primary key columns (in primary key order) and has the others after them
	order := make([]int, 0, len(cols))
	if withoutRowid {
		for k := 1; k <= len(cols); k++ {
			for i, c := range cols {
				if c.pk == k {
					order = append(order, i)
				}
			}
		}
	}
	for i, c := range cols {
		if !withoutRowid || c.pk == 0 {
			order = append(order, i)
		}
	}
	t := &Table{Format: "sqlite"}
	for _, c := range cols {
		t.Columns = append(t.Columns, Column{Name: c.name, Type: c.typ})
	}
	cut := false
	err := d.walk(obj.root, func(rowid int64, payload []byte) (bool, error) {
		if len(t.Rows) == MaxRows {
			t.Truncated = true
			return false, nil
		}
		if len(payload) >= maxPayload {
			cut = true
		}
		vals, err := d.record(payload)
		if err != nil {
			return false, err
		}
		var row []*string
		if ok && len(cols) > 0 {
			row = make([]*string, len(cols))
			k := 0
			for _, i := range order {
				c := cols[i]
				if !c.stored {
					continue // a generated VIRTUAL column is computed when read, never stored
				}
				if k < len(vals) && vals[k].kind != kindNull {
					v := vals[k]
					if v.kind == kindInt && c.real {
						v = value{kind: kindFloat, f: float64(v.i)} // SQLite stores whole REAL values as integers to save space
					}
					row[i] = strp(v.String())
				} else if c.rowid {
					row[i] = strp(strconv.FormatInt(rowid, 10)) // INTEGER PRIMARY KEY is the rowid itself
				}
				k++
			}
		} else {
			// no column list could be read (CREATE TABLE ... AS SELECT): name the columns by number
			for len(t.Columns) < len(vals) {
				t.Columns = append(t.Columns, Column{Name: "column" + strconv.Itoa(len(t.Columns)+1)})
			}
			row = make([]*string, len(vals))
			for i, v := range vals {
				if v.kind != kindNull {
					row[i] = strp(v.String())
				}
			}
		}
		t.Rows = append(t.Rows, row)
		return true, nil
	})
	if err != nil {
		if len(t.Rows) == 0 {
			return nil, err
		}
		t.Note = fmt.Sprintf("The table could only be read up to row %d: %v.", len(t.Rows), err)
	}
	if cut && t.Note == "" {
		t.Note = "Some rows are very large; their last columns are shown as NULL."
	}
	if t.Truncated {
		if n, err := d.count(obj.root, countPages); err == nil {
			t.Total = n
		}
	}
	return t, nil
}

// countPages caps how many pages count reads to find how many rows a table has (a few seconds at most).
const countPages = 1 << 20

// page reads page n (1-based).
func (d *DB) page(n uint32) ([]byte, error) {
	if n < 1 || n > d.pages {
		return nil, errDamaged
	}
	b := make([]byte, d.pageSize)
	if _, err := d.r.ReadAt(b, int64(n-1)*int64(d.pageSize)); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return b, nil
}

// walk calls fn with every row of the b-tree rooted at page root, in key order, until fn says stop. A table b-tree
// (a rowid table) gives each row's rowid; an index b-tree (a WITHOUT ROWID table) gives 0, its key is the record.
func (d *DB) walk(root uint32, fn func(rowid int64, payload []byte) (bool, error)) error {
	_, _, typ, _, err := d.pageHeader(root)
	if err != nil {
		return err
	}
	_, err = d.walkPage(root, 0, typ == 2 || typ == 10, map[uint32]bool{}, fn)
	return err
}

// pageHeader reads the b-tree header of page pg: its bytes, where the header starts, the page type and the number of
// cells. Only b-tree pages pass (2 and 10 index, 5 and 13 table).
func (d *DB) pageHeader(pg uint32) (buf []byte, off int, typ byte, n int, err error) {
	buf, err = d.page(pg)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	if pg == 1 {
		off = 100
	}
	if off+12 > d.usable {
		return nil, 0, 0, 0, errDamaged
	}
	typ = buf[off]
	if typ != 2 && typ != 5 && typ != 10 && typ != 13 {
		return nil, 0, 0, 0, errDamaged
	}
	n = int(binary.BigEndian.Uint16(buf[off+3:]))
	if off+headerLen(typ)+2*n > d.usable {
		return nil, 0, 0, 0, errDamaged
	}
	return buf, off, typ, n, nil
}

// headerLen is the length of a b-tree page header: interior pages have the right-most child pointer too.
func headerLen(typ byte) int {
	if typ == 2 || typ == 5 {
		return 12
	}
	return 8
}

func (d *DB) walkPage(pg uint32, depth int, index bool, seen map[uint32]bool, fn func(int64, []byte) (bool, error)) (bool, error) {
	if depth > maxDepth || seen[pg] {
		return false, errDamaged
	}
	seen[pg] = true
	buf, off, typ, n, err := d.pageHeader(pg)
	if err != nil {
		return false, err
	}
	if (typ == 2 || typ == 10) != index {
		return false, errDamaged // a page of another kind of b-tree than its root
	}
	hl := headerLen(typ)
	interior := hl == 12
	for i := 0; i < n; i++ {
		p := int(binary.BigEndian.Uint16(buf[off+hl+2*i:]))
		if p < off+hl || p >= d.usable {
			return false, errDamaged
		}
		if interior {
			if p+4 > d.usable {
				return false, errDamaged
			}
			more, err := d.walkPage(binary.BigEndian.Uint32(buf[p:]), depth+1, index, seen, fn)
			if err != nil || !more {
				return more, err
			}
			if !index {
				continue // a table interior cell only holds a key, its rows are in the leaves
			}
			p += 4 // an index interior cell holds a row (its key) after the child pointer
		}
		size, k := varint(buf[p:d.usable])
		if k == 0 {
			return false, errDamaged
		}
		p += k
		var rowid uint64
		if !index {
			rowid, k = varint(buf[p:d.usable])
			if k == 0 {
				return false, errDamaged
			}
			p += k
		}
		payload, err := d.payload(buf, p, size, index)
		if err != nil {
			return false, err
		}
		more, err := fn(int64(rowid), payload)
		if err != nil || !more {
			return more, err
		}
	}
	if interior {
		return d.walkPage(binary.BigEndian.Uint32(buf[off+8:]), depth+1, index, seen, fn)
	}
	return true, nil
}

// count counts the rows of the b-tree rooted at page root from the page headers alone, without decoding a record:
// the cells of the leaves, and for an index b-tree the cells of the interior pages too. It stops with an error after
// limit pages.
func (d *DB) count(root uint32, limit int) (int64, error) {
	seen := map[uint32]bool{}
	var walk func(pg uint32, depth int) (int64, error)
	walk = func(pg uint32, depth int) (int64, error) {
		if depth > maxDepth || seen[pg] || len(seen) >= limit {
			return 0, errDamaged
		}
		seen[pg] = true
		buf, off, typ, n, err := d.pageHeader(pg)
		if err != nil {
			return 0, err
		}
		if typ == 10 || typ == 13 {
			return int64(n), nil
		}
		var total int64
		if typ == 2 {
			total = int64(n)
		}
		for i := 0; i <= n; i++ {
			at := off + 8 // the right-most child last
			if i < n {
				at = int(binary.BigEndian.Uint16(buf[off+12+2*i:]))
				if at < off+12 || at+4 > d.usable {
					return 0, errDamaged
				}
			}
			c, err := walk(binary.BigEndian.Uint32(buf[at:]), depth+1)
			if err != nil {
				return 0, err
			}
			total += c
		}
		return total, nil
	}
	return walk(root, 0)
}

// payload gathers the record of a cell that starts at buf[p] and is size bytes long: the part on the page and, when
// it spills, the overflow pages after it (up to maxPayload). Index cells keep less on the page than table leaf cells.
func (d *DB) payload(buf []byte, p int, size uint64, index bool) ([]byte, error) {
	u := d.usable
	x := u - 35
	if index {
		x = ((u-12)*64)/255 - 23
	}
	m := ((u-12)*32)/255 - 23
	local := int(size)
	if size > uint64(x) {
		k := m + int((size-uint64(m))%uint64(u-4))
		local = m
		if k <= x {
			local = k
		}
	}
	if p+local > u {
		return nil, errDamaged
	}
	want := int(min(size, maxPayload))
	out := make([]byte, 0, want)
	out = append(out, buf[p:p+min(local, want)]...)
	if uint64(local) >= size {
		return out, nil
	}
	if p+local+4 > u {
		return nil, errDamaged
	}
	next := binary.BigEndian.Uint32(buf[p+local:])
	for hops := uint32(0); len(out) < want; hops++ {
		if next == 0 || hops > d.pages {
			return nil, errDamaged
		}
		ov, err := d.page(next)
		if err != nil {
			return nil, err
		}
		next = binary.BigEndian.Uint32(ov)
		out = append(out, ov[4:4+min(u-4, want-len(out))]...)
	}
	return out, nil
}

// varint reads a SQLite variable-length integer; n is 0 when b is too short.
func varint(b []byte) (v uint64, n int) {
	for i := 0; i < 8; i++ {
		if i >= len(b) {
			return 0, 0
		}
		v = v<<7 | uint64(b[i]&0x7f)
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	if len(b) < 9 {
		return 0, 0
	}
	return v<<8 | uint64(b[8]), 9
}

// value kinds of a record field
const (
	kindNull = iota
	kindInt
	kindFloat
	kindText
	kindBlob
)

// value is one field of a record.
type value struct {
	kind int
	i    int64
	f    float64
	b    []byte // text (already UTF-8) or blob
	size int    // the full length of a text or blob that was cut short
}

func (v value) text() string {
	if v.kind == kindText {
		return string(v.b)
	}
	return ""
}

// String is the text a cell shows: numbers as SQLite prints them, a blob as hex.
func (v value) String() string {
	switch v.kind {
	case kindInt:
		return strconv.FormatInt(v.i, 10)
	case kindFloat:
		s := strconv.FormatFloat(v.f, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eEIN") {
			s += ".0"
		}
		return s
	case kindText:
		return clip(string(v.b))
	case kindBlob:
		const show = 32
		h := strings.ToUpper(hex.EncodeToString(v.b[:min(len(v.b), show)]))
		if v.size > show {
			return fmt.Sprintf("x'%s…' (%d bytes)", h, v.size)
		}
		return "x'" + h + "'"
	}
	return ""
}

// record decodes a record: the header of serial types, then the fields. A record cut short by maxPayload ends early.
func (d *DB) record(p []byte) ([]value, error) {
	hl, n := varint(p)
	if n == 0 || hl > uint64(len(p)) || hl < uint64(n) {
		return nil, errDamaged
	}
	var types []uint64
	for pos := n; pos < int(hl); {
		t, k := varint(p[pos:hl])
		if k == 0 {
			return nil, errDamaged
		}
		types = append(types, t)
		pos += k
	}
	body := p[hl:]
	out := make([]value, 0, len(types))
	for _, t := range types {
		size := serialSize(t)
		if size < 0 {
			return nil, errDamaged
		}
		if size > len(body) {
			if t >= 12 && len(body) > 0 {
				// a long text or blob cut short by maxPayload: keep what there is
				v := d.field(t, body)
				v.size = size
				out = append(out, v)
			}
			break
		}
		v := d.field(t, body[:size])
		v.size = size
		out = append(out, v)
		body = body[size:]
	}
	return out, nil
}

// serialSize is the number of body bytes a serial type takes (-1 for the reserved types 10 and 11).
func serialSize(t uint64) int {
	switch {
	case t <= 4:
		return int(t)
	case t == 5:
		return 6
	case t == 6 || t == 7:
		return 8
	case t == 8 || t == 9:
		return 0
	case t == 10 || t == 11:
		return -1
	case t > 2*math.MaxInt32:
		return -1
	}
	return int((t - 12) / 2)
}

// field decodes one field of serial type t from its bytes.
func (d *DB) field(t uint64, b []byte) value {
	switch {
	case t == 0:
		return value{kind: kindNull}
	case t >= 1 && t <= 6:
		var v int64
		for _, c := range b {
			v = v<<8 | int64(c)
		}
		shift := 64 - 8*len(b)
		return value{kind: kindInt, i: v << shift >> shift} // sign-extend
	case t == 7:
		return value{kind: kindFloat, f: math.Float64frombits(binary.BigEndian.Uint64(b))}
	case t == 8:
		return value{kind: kindInt}
	case t == 9:
		return value{kind: kindInt, i: 1}
	case t%2 == 0:
		return value{kind: kindBlob, b: b}
	}
	return value{kind: kindText, b: []byte(d.decodeText(b))}
}

// decodeText turns text stored in the database's encoding into UTF-8.
func (d *DB) decodeText(b []byte) string {
	if d.enc == 1 {
		return string(b)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		if d.enc == 2 {
			u[i] = binary.LittleEndian.Uint16(b[2*i:])
		} else {
			u[i] = binary.BigEndian.Uint16(b[2*i:])
		}
	}
	return string(utf16.Decode(u))
}
