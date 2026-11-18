// Tests of the Parquet reader. There is no Parquet writer in the module, so the tests build small files themselves
// with a Thrift compact writer and page encoders written from the format specification: flat and optional columns,
// a field of an optional group, a list column (left out), two row groups, dictionary pages, data pages v1 and v2, every
// supported encoding and codec, logical types (string, date, timestamp, decimal, UUID), a file of more than MaxRows rows,
// and damaged files (every byte changed in turn, cut short, encrypted).

package table

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/snappy"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

// tw writes the Thrift compact protocol.
type tw struct {
	b    []byte
	last []int16
}

func (w *tw) uv(v uint64)           { w.b = binary.AppendUvarint(w.b, v) }
func (w *tw) zz(v int64)            { w.uv(uint64(v<<1) ^ uint64(v>>63)) }
func (w *tw) begin()                { w.last = append(w.last, 0) }
func (w *tw) end()                  { w.b = append(w.b, 0); w.last = w.last[:len(w.last)-1] }
func (w *tw) i32(id int16, v int64) { w.field(id, 5); w.zz(v) }
func (w *tw) i64(id int16, v int64) { w.field(id, 6); w.zz(v) }
func (w *tw) bin(id int16, b []byte) {
	w.field(id, 8)
	w.uv(uint64(len(b)))
	w.b = append(w.b, b...)
}
func (w *tw) boolean(id int16, v bool) {
	if v {
		w.field(id, 1)
	} else {
		w.field(id, 2)
	}
}

func (w *tw) field(id int16, typ byte) {
	top := &w.last[len(w.last)-1]
	if d := id - *top; d > 0 && d <= 15 {
		w.b = append(w.b, byte(d)<<4|typ)
	} else {
		w.b = append(w.b, typ)
		w.zz(int64(id))
	}
	*top = id
}

func (w *tw) sub(id int16, f func()) {
	w.field(id, 12)
	w.begin()
	f()
	w.end()
}

func (w *tw) listHead(id int16, typ byte, n int) {
	w.field(id, 9)
	if n < 15 {
		w.b = append(w.b, byte(n)<<4|typ)
	} else {
		w.b = append(w.b, 0xf0|typ)
		w.uv(uint64(n))
	}
}

func (w *tw) structs(id int16, n int, f func(i int)) {
	w.listHead(id, 12, n)
	for i := range n {
		w.begin()
		f(i)
		w.end()
	}
}

// ---- page encoders ----

func le32(v int) []byte { return binary.LittleEndian.AppendUint32(nil, uint32(v)) }

func plainI64(v ...int64) []byte {
	var b []byte
	for _, x := range v {
		b = binary.LittleEndian.AppendUint64(b, uint64(x))
	}
	return b
}

func plainI32(v ...int32) []byte {
	var b []byte
	for _, x := range v {
		b = binary.LittleEndian.AppendUint32(b, uint32(x))
	}
	return b
}

func plainF64(v ...float64) []byte {
	var b []byte
	for _, x := range v {
		b = binary.LittleEndian.AppendUint64(b, math.Float64bits(x))
	}
	return b
}

func plainStr(v ...string) []byte {
	var b []byte
	for _, s := range v {
		b = append(append(b, le32(len(s))...), s...)
	}
	return b
}

func plainBool(v ...bool) []byte {
	b := make([]byte, (len(v)+7)/8)
	for i, x := range v {
		if x {
			b[i/8] |= 1 << (i % 8)
		}
	}
	return b
}

// pack bit-packs values of width w, least significant bit first.
func pack(v []uint64, w int) []byte {
	b := make([]byte, (len(v)*w+7)/8)
	for i, x := range v {
		for k := 0; k < w; k++ {
			if x>>k&1 == 1 {
				bit := i*w + k
				b[bit/8] |= 1 << (bit % 8)
			}
		}
	}
	return b
}

// rleRuns writes values as RLE runs of equal values.
func rleRuns(v []uint32, w int) []byte {
	var b []byte
	for i := 0; i < len(v); {
		j := i
		for j < len(v) && v[j] == v[i] {
			j++
		}
		b = binary.AppendUvarint(b, uint64(j-i)<<1)
		for k := 0; k < (w+7)/8; k++ {
			b = append(b, byte(v[i]>>(8*k)))
		}
		i = j
	}
	return b
}

// bitPacked writes values as one bit-packed run (padded to a multiple of 8).
func bitPacked(v []uint32, w int) []byte {
	groups := (len(v) + 7) / 8
	u := make([]uint64, groups*8)
	for i, x := range v {
		u[i] = uint64(x)
	}
	return append(binary.AppendUvarint(nil, uint64(groups)<<1|1), pack(u, w)...)
}

// deltaPacked writes DELTA_BINARY_PACKED with blocks of 128 values in 4 miniblocks.
func deltaPacked(v []int64) []byte {
	w := &tw{}
	w.uv(128)
	w.uv(4)
	w.uv(uint64(len(v)))
	if len(v) == 0 {
		w.zz(0)
		return w.b
	}
	w.zz(v[0])
	for start := 1; start < len(v); start += 128 {
		end := min(len(v), start+128)
		deltas := make([]int64, end-start)
		minD := int64(math.MaxInt64)
		for i := range deltas {
			deltas[i] = v[start+i] - v[start+i-1]
			minD = min(minD, deltas[i])
		}
		w.zz(minD)
		var widths []byte
		var data []byte
		for m := 0; m < 4; m++ {
			lo := m * 32
			if lo >= len(deltas) {
				widths = append(widths, 0)
				continue
			}
			u := make([]uint64, 32)
			var mx uint64
			for i := lo; i < min(lo+32, len(deltas)); i++ {
				u[i-lo] = uint64(deltas[i] - minD)
				mx = max(mx, u[i-lo])
			}
			bw := bits.Len64(mx)
			widths = append(widths, byte(bw))
			data = append(data, pack(u, bw)...)
		}
		w.b = append(append(w.b, widths...), data...)
	}
	return w.b
}

func deltaLength(v [][]byte) []byte {
	lens := make([]int64, len(v))
	var data []byte
	for i, s := range v {
		lens[i] = int64(len(s))
		data = append(data, s...)
	}
	return append(deltaPacked(lens), data...)
}

func deltaBytes(v []string) []byte {
	prefix := make([]int64, len(v))
	suffix := make([][]byte, len(v))
	prev := ""
	for i, s := range v {
		p := 0
		for p < len(s) && p < len(prev) && s[p] == prev[p] {
			p++
		}
		prefix[i], suffix[i] = int64(p), []byte(s[p:])
		prev = s
	}
	return append(deltaPacked(prefix), deltaLength(suffix)...)
}

func compressFor(t *testing.T, codec int, b []byte) []byte {
	t.Helper()
	switch codec {
	case 1:
		return snappy.Encode(nil, b)
	case 2:
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		zw.Write(b)
		zw.Close()
		return buf.Bytes()
	case 4:
		var buf bytes.Buffer
		bw := brotli.NewWriter(&buf)
		bw.Write(b)
		bw.Close()
		return buf.Bytes()
	case 5, 7:
		out := make([]byte, lz4.CompressBlockBound(len(b)))
		n, err := lz4.CompressBlock(b, out, nil)
		if err != nil || n == 0 {
			t.Fatalf("lz4: %v", err)
		}
		if codec == 7 {
			return out[:n]
		}
		h := binary.BigEndian.AppendUint32(nil, uint32(len(b)))
		h = binary.BigEndian.AppendUint32(h, uint32(n))
		return append(h, out[:n]...)
	case 6:
		zw, _ := zstd.NewWriter(nil)
		defer zw.Close()
		return zw.EncodeAll(b, nil)
	}
	return b
}

func pageHeader(ptype, usize, csize int, id int16, f func(w *tw)) []byte {
	w := &tw{}
	w.begin()
	w.i32(1, int64(ptype))
	w.i32(2, int64(usize))
	w.i32(3, int64(csize))
	w.sub(id, func() { f(w) })
	w.end()
	return w.b
}

func dictPage(t *testing.T, codec, n int, values []byte) []byte {
	c := compressFor(t, codec, values)
	return append(pageHeader(2, len(values), len(c), 7, func(w *tw) { w.i32(1, int64(n)); w.i32(2, 0) }), c...)
}

func dataV1(t *testing.T, codec, enc, n int, defs []uint32, maxDef int, values []byte) []byte {
	var body []byte
	if maxDef > 0 {
		lv := rleRuns(defs, bits.Len(uint(maxDef)))
		body = append(le32(len(lv)), lv...)
	}
	body = append(body, values...)
	c := compressFor(t, codec, body)
	return append(pageHeader(0, len(body), len(c), 5, func(w *tw) {
		w.i32(1, int64(n))
		w.i32(2, int64(enc))
		w.i32(3, 3)
		w.i32(4, 3)
	}), c...)
}

func dataV2(t *testing.T, codec, enc, n int, defs []uint32, maxDef int, values []byte, compressed bool) []byte {
	var lv []byte
	nulls := 0
	if maxDef > 0 {
		lv = rleRuns(defs, bits.Len(uint(maxDef)))
		for _, d := range defs {
			if int(d) != maxDef {
				nulls++
			}
		}
	}
	c := values
	if compressed {
		c = compressFor(t, codec, values)
	}
	h := pageHeader(3, len(lv)+len(values), len(lv)+len(c), 8, func(w *tw) {
		w.i32(1, int64(n))
		w.i32(2, int64(nulls))
		w.i32(3, int64(n))
		w.i32(4, int64(enc))
		w.i32(5, int64(len(lv)))
		w.i32(6, 0)
		w.boolean(7, compressed)
	})
	return append(append(h, lv...), c...)
}

// ---- file builder ----

// pqSchemaEl is one schema element of a test file.
type pqSchemaEl struct {
	name              string
	typ, typeLen, rep int // typ -1 for a group
	children          int
	converted         int // -1 none
	scale, precision  int
	logical           func(w *tw)
}

// pqChunk is the pages of one column chunk.
type pqChunk struct {
	typ, codec int
	dict, data []byte
	values     int
}

func buildParquet(schema []pqSchemaEl, numRows int64, groups [][]pqChunk, rowsPer []int64) []byte {
	buf := []byte(parquetMagic)
	type placed struct {
		c                pqChunk
		dictOff, dataOff int
		total            int
	}
	var all [][]placed
	for _, g := range groups {
		var ps []placed
		for _, c := range g {
			p := placed{c: c, dictOff: -1}
			start := len(buf)
			if c.dict != nil {
				p.dictOff = len(buf)
				buf = append(buf, c.dict...)
			}
			p.dataOff = len(buf)
			buf = append(buf, c.data...)
			p.total = len(buf) - start
			ps = append(ps, p)
		}
		all = append(all, ps)
	}
	w := &tw{}
	w.begin()
	w.i32(1, 1)
	w.structs(2, len(schema), func(i int) {
		e := schema[i]
		if e.typ >= 0 {
			w.i32(1, int64(e.typ))
		}
		if e.typeLen > 0 {
			w.i32(2, int64(e.typeLen))
		}
		if i > 0 {
			w.i32(3, int64(e.rep))
		}
		w.bin(4, []byte(e.name))
		if e.typ < 0 {
			w.i32(5, int64(e.children))
		}
		if e.converted >= 0 {
			w.i32(6, int64(e.converted))
		}
		if e.scale > 0 {
			w.i32(7, int64(e.scale))
			w.i32(8, int64(e.precision))
		}
		if e.logical != nil {
			w.sub(10, func() { e.logical(w) })
		}
	})
	w.i64(3, numRows)
	w.structs(4, len(all), func(g int) {
		w.structs(1, len(all[g]), func(i int) {
			p := all[g][i]
			w.i64(2, int64(p.dataOff))
			w.sub(3, func() {
				w.i32(1, int64(p.c.typ))
				w.listHead(2, 5, 1)
				w.zz(0)
				w.listHead(3, 8, 1)
				w.uv(1)
				w.b = append(w.b, 'x')
				w.i32(4, int64(p.c.codec))
				w.i64(5, int64(p.c.values))
				w.i64(6, int64(p.total))
				w.i64(7, int64(p.total))
				w.i64(9, int64(p.dataOff))
				if p.dictOff >= 0 {
					w.i64(11, int64(p.dictOff))
				}
			})
		})
		w.i64(2, 100)
		w.i64(3, rowsPer[g])
	})
	w.end()
	buf = append(buf, w.b...)
	buf = append(buf, le32(len(w.b))...)
	return append(buf, parquetMagic...)
}

func req(name string, typ, conv int) pqSchemaEl {
	return pqSchemaEl{name: name, typ: typ, converted: conv}
}
func opt(name string, typ, conv int) pqSchemaEl {
	return pqSchemaEl{name: name, typ: typ, rep: 1, converted: conv}
}

// flatFile is the file of TestParquetFlat: two row groups of 3 and 2 rows.
func flatFile(t *testing.T) []byte {
	str := func(w *tw) { w.sub(1, func() {}) }
	schema := []pqSchemaEl{
		{name: "schema", typ: -1, children: 9, converted: -1},
		req("id", pqInt64, -1),
		{name: "name", typ: pqByteArray, rep: 1, converted: -1, logical: str},
		opt("score", pqDouble, -1),
		req("flag", pqBoolean, -1),
		req("day", pqInt32, 6),
		{name: "ts", typ: pqInt64, converted: -1, logical: func(w *tw) {
			w.sub(8, func() { w.boolean(1, true); w.sub(2, func() { w.sub(2, func() {}) }) })
		}},
		{name: "price", typ: pqInt32, converted: 5, scale: 2, precision: 5},
		{name: "info", typ: -1, rep: 1, children: 1, converted: -1},
		opt("city", pqByteArray, 0),
		{name: "tags", typ: -1, rep: 1, children: 1, converted: 3},
		{name: "list", typ: -1, rep: 2, children: 1, converted: -1},
		opt("element", pqInt32, -1),
	}
	g1 := []pqChunk{
		{typ: pqInt64, data: dataV1(t, 0, 0, 3, nil, 0, plainI64(1, 2, 3)), values: 3},
		{typ: pqByteArray, codec: 1, dict: dictPage(t, 1, 1, plainStr("alpha")),
			data: dataV1(t, 1, 8, 3, []uint32{1, 0, 1}, 1, append([]byte{0}, bitPacked([]uint32{0, 0}, 0)...)), values: 3},
		{typ: pqDouble, codec: 2, data: dataV1(t, 2, 0, 3, []uint32{1, 0, 1}, 1, plainF64(1.5, -2)), values: 3},
		{typ: pqBoolean, codec: 6, data: dataV1(t, 6, 0, 3, nil, 0, plainBool(true, false, true)), values: 3},
		{typ: pqInt32, codec: 7, data: dataV1(t, 7, 0, 3, nil, 0, plainI32(0, 19000, -1)), values: 3},
		{typ: pqInt64, codec: 4, data: dataV1(t, 4, 0, 3, nil, 0, plainI64(0, 1700000000123456, -1)), values: 3},
		{typ: pqInt32, codec: 5, data: dataV1(t, 5, 0, 3, nil, 0, plainI32(12345, -5, 7)), values: 3},
		{typ: pqByteArray, data: dataV1(t, 0, 0, 3, []uint32{2, 1, 0}, 2, plainStr("Pécs")), values: 3},
		{typ: pqInt32, data: []byte("never read"), values: 0},
	}
	g2 := []pqChunk{
		{typ: pqInt64, data: dataV1(t, 0, 0, 2, nil, 0, plainI64(4, 5)), values: 2},
		{typ: pqByteArray, codec: 1, dict: dictPage(t, 1, 2, plainStr("beta", "gamma")),
			data: dataV1(t, 1, 2, 2, []uint32{1, 1}, 1, append([]byte{1}, bitPacked([]uint32{0, 1}, 1)...)), values: 2},
		{typ: pqDouble, codec: 2, data: dataV1(t, 2, 0, 2, []uint32{1, 0}, 1, plainF64(3.25)), values: 2},
		{typ: pqBoolean, codec: 6, data: dataV1(t, 6, 0, 2, nil, 0, plainBool(true, false)), values: 2},
		{typ: pqInt32, codec: 7, data: dataV1(t, 7, 0, 2, nil, 0, plainI32(1, 2)), values: 2},
		{typ: pqInt64, codec: 4, data: dataV1(t, 4, 0, 2, nil, 0, plainI64(1, 2)), values: 2},
		{typ: pqInt32, codec: 5, data: dataV1(t, 5, 0, 2, nil, 0, plainI32(0, 100)), values: 2},
		{typ: pqByteArray, data: dataV1(t, 0, 0, 2, []uint32{2, 2}, 2, plainStr("Győr", "Bp")), values: 2},
		{typ: pqInt32, data: []byte("never read"), values: 0},
	}
	return buildParquet(schema, 5, [][]pqChunk{g1, g2}, []int64{3, 2})
}

func readPQ(t *testing.T, b []byte) *Table {
	t.Helper()
	tb, err := ReadParquet(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

func TestParquetFlat(t *testing.T) {
	b := flatFile(t)
	if Kind("data.bin", b[:100]) != "parquet" {
		t.Error("Parquet not known by its magic")
	}
	tb := readPQ(t, b)
	if got := colNames(tb); got != "id:int64,name:string,score:double,flag:boolean,day:date,ts:timestamp(µs),price:decimal(5,2),info.city:string" {
		t.Errorf("columns: %s", got)
	}
	if tb.Format != "parquet" || len(tb.Rows) != 5 || tb.Truncated || tb.Note != "Columns with lists or maps are not shown: tags.list.element." {
		t.Fatalf("rows %d truncated %v note %q", len(tb.Rows), tb.Truncated, tb.Note)
	}
	want := [][]string{
		{"1", "alpha", "1.5", "true", "1970-01-01", "1970-01-01 00:00:00.000000", "123.45", "Pécs"},
		{"2", "<nil>", "<nil>", "false", time.Unix(19000*86400, 0).UTC().Format("2006-01-02"), "2023-11-14 22:13:20.123456", "-0.05", "<nil>"},
		{"3", "alpha", "-2", "true", "1969-12-31", "1969-12-31 23:59:59.999999", "0.07", "<nil>"},
		{"4", "beta", "3.25", "true", "1970-01-02", "1970-01-01 00:00:00.000001", "0.00", "Győr"},
		{"5", "gamma", "<nil>", "false", "1970-01-03", "1970-01-01 00:00:00.000002", "1.00", "Bp"},
	}
	for r, w := range want {
		for c, v := range w {
			if got := cell(t, tb, r, c); got != v {
				t.Errorf("row %d col %d: %q, want %q", r, c, got, v)
			}
		}
	}
}

func TestParquetV2Encodings(t *testing.T) {
	const n = 300
	var ints []int64
	var strs []string
	var defs []uint32
	var lens [][]byte
	var dbl []float64
	var uuids, decs []byte
	for i := range n {
		ints = append(ints, int64(i*i-1000))
		if i%7 == 3 {
			defs = append(defs, 0)
		} else {
			defs = append(defs, 1)
			strs = append(strs, fmt.Sprintf("key-%05d", i))
		}
		if i%2 == 0 {
			lens = append(lens, []byte(fmt.Sprint("v", i)))
		} else {
			lens = append(lens, []byte{0xff, byte(i)})
		}
		dbl = append(dbl, float64(i)/4)
		u := make([]byte, 16)
		u[15] = byte(i)
		uuids = append(uuids, u...)
		decs = binary.BigEndian.AppendUint32(decs, uint32(int32(i*1000-1234567)))
	}
	split := make([]byte, 8*n)
	raw := plainF64(dbl...)
	for i := range n {
		for k := range 8 {
			split[k*n+i] = raw[i*8+k]
		}
	}
	schema := []pqSchemaEl{
		{name: "schema", typ: -1, children: 6, converted: -1},
		req("n", pqInt64, -1),
		{name: "s", typ: pqByteArray, rep: 1, converted: -1, logical: func(w *tw) { w.sub(1, func() {}) }},
		req("bin", pqByteArray, -1),
		req("d", pqDouble, -1),
		{name: "u", typ: pqFixed, typeLen: 16, converted: -1, logical: func(w *tw) { w.sub(14, func() {}) }},
		{name: "m", typ: pqFixed, typeLen: 4, converted: -1, logical: func(w *tw) {
			w.sub(5, func() { w.i32(1, 3); w.i32(2, 9) })
		}},
	}
	g := []pqChunk{
		{typ: pqInt64, codec: 6, data: dataV2(t, 6, 5, n, nil, 0, deltaPacked(ints), true), values: n},
		{typ: pqByteArray, codec: 1, data: dataV2(t, 1, 7, n, defs, 1, deltaBytes(strs), true), values: n},
		{typ: pqByteArray, data: dataV1(t, 0, 6, n, nil, 0, deltaLength(lens)), values: n},
		{typ: pqDouble, codec: 1, data: dataV2(t, 1, 9, n, nil, 0, split, false), values: n},
		{typ: pqFixed, data: dataV1(t, 0, 0, n, nil, 0, uuids), values: n},
		{typ: pqFixed, codec: 6, data: dataV1(t, 6, 0, n, nil, 0, decs), values: n},
	}
	tb := readPQ(t, buildParquet(schema, n, [][]pqChunk{g}, []int64{n}))
	if got := colNames(tb); got != "n:int64,s:string,bin:binary,d:double,u:uuid,m:decimal(9,3)" {
		t.Errorf("columns: %s", got)
	}
	if len(tb.Rows) != n || tb.Note != "" {
		t.Fatalf("rows %d note %q", len(tb.Rows), tb.Note)
	}
	k := 0
	for i := range n {
		if got := cell(t, tb, i, 0); got != fmt.Sprint(i*i-1000) {
			t.Fatalf("row %d n: %q", i, got)
		}
		s := cell(t, tb, i, 1)
		if defs[i] == 0 {
			if s != "<nil>" {
				t.Fatalf("row %d s: %q, want NULL", i, s)
			}
		} else {
			if s != strs[k] {
				t.Fatalf("row %d s: %q, want %q", i, s, strs[k])
			}
			k++
		}
		wantBin := fmt.Sprint("v", i)
		if i%2 == 1 {
			wantBin = fmt.Sprintf("x'FF%02X'", byte(i))
		}
		if got := cell(t, tb, i, 2); got != wantBin {
			t.Fatalf("row %d bin: %q, want %q", i, got, wantBin)
		}
		if got := cell(t, tb, i, 3); got != fmt.Sprint(float64(i)/4) {
			t.Fatalf("row %d d: %q", i, got)
		}
		if got := cell(t, tb, i, 4); got != fmt.Sprintf("00000000-0000-0000-0000-0000000000%02x", byte(i)) {
			t.Fatalf("row %d u: %q", i, got)
		}
	}
	if cell(t, tb, 0, 5) != "-1234.567" || cell(t, tb, 299, 5) != "-935.567" {
		t.Errorf("decimals: %q %q", cell(t, tb, 0, 5), cell(t, tb, 299, 5))
	}
}

func TestParquetCap(t *testing.T) {
	schema := []pqSchemaEl{{name: "schema", typ: -1, children: 1, converted: -1}, req("i", pqInt32, -1)}
	var groups [][]pqChunk
	var per []int64
	next := int32(0)
	for _, rows := range []int{4000, 4000, 2100, 50} {
		v := make([]int32, rows)
		for i := range v {
			v[i] = next
			next++
		}
		groups = append(groups, []pqChunk{{typ: pqInt32, codec: 1, data: dataV1(t, 1, 0, rows, nil, 0, plainI32(v...)), values: rows}})
		per = append(per, int64(rows))
	}
	tb := readPQ(t, buildParquet(schema, 10150, groups, per))
	if len(tb.Rows) != MaxRows || !tb.Truncated || tb.Total != 10150 {
		t.Fatalf("rows %d truncated %v total %d", len(tb.Rows), tb.Truncated, tb.Total)
	}
	if cell(t, tb, MaxRows-1, 0) != fmt.Sprint(MaxRows-1) || cell(t, tb, 4000, 0) != "4000" {
		t.Errorf("cells: %q %q", cell(t, tb, MaxRows-1, 0), cell(t, tb, 4000, 0))
	}
}

func TestParquetDamaged(t *testing.T) {
	orig := flatFile(t)
	for _, b := range [][]byte{orig[:len(orig)-3], orig[:20], []byte("PAR1PAR1"), append(append([]byte{}, orig[:len(orig)-4]...), "PARE"...)} {
		if _, err := ReadParquet(bytes.NewReader(b), int64(len(b))); err == nil {
			t.Errorf("no error for a damaged file of %d bytes", len(b))
		}
	}
	// every byte changed in turn: an error or a table, never a panic or a hang
	for i := range orig {
		for _, x := range []byte{0xff, 0x01, 0x80} {
			b := append([]byte{}, orig...)
			b[i] ^= x
			ReadParquet(bytes.NewReader(b), int64(len(b)))
		}
	}
	if _, err := ReadParquet(bytes.NewReader(orig), int64(len(orig))); err != nil || strings.Contains(fmt.Sprint(err), "panic") {
		t.Error(err)
	}
}

func TestRLEHybrid(t *testing.T) {
	b := append(rleRuns([]uint32{5, 5, 5}, 3), bitPacked([]uint32{1, 2, 3, 4, 5, 6, 7, 0, 1}, 3)...)
	v, err := rleHybrid(b, 3, 12)
	if err != nil || fmt.Sprint(v) != "[5 5 5 1 2 3 4 5 6 7 0 1]" {
		t.Errorf("%v %v", v, err)
	}
	if _, err := rleHybrid(b, 3, 40); err == nil {
		t.Error("reading past the end is an error")
	}
	d := &pqDecoder{b: deltaPacked([]int64{7, 5, 3, 1, 2, 3, 4, 5})}
	if v, err := d.deltaInts(8); err != nil || fmt.Sprint(v) != "[7 5 3 1 2 3 4 5]" {
		t.Errorf("delta: %v %v", v, err)
	}
	// a hostile header that claims a huge number of values in blocks of zero-width miniblocks ends quickly
	w := &tw{}
	w.uv(1 << 16)
	w.uv(1)
	w.uv(1 << 60)
	w.zz(0)
	hostile := append(w.b, make([]byte, 1<<20)...)
	start := time.Now()
	if v, _ := (&pqDecoder{b: hostile}).deltaInts(3); len(v) > 3 || time.Since(start) > 2*time.Second {
		t.Errorf("hostile delta header: %d values in %v", len(v), time.Since(start))
	}
	start = time.Now()
	(&pqDecoder{b: hostile}).deltaInts(0)
	if time.Since(start) > 2*time.Second {
		t.Errorf("skipping took %v", time.Since(start))
	}
	if half(0x3c00) != 1 || half(0xc000) != -2 || half(0x0000) != 0 {
		t.Errorf("half: %v %v", half(0x3c00), half(0xc000))
	}
}

// TestParquetFixture checks testdata/flat.parquet (used by the Lister's tests) reads as the file flatFile builds; run
// with MC_WRITE_FIXTURES=1 to write it again.
func TestParquetFixture(t *testing.T) {
	b := flatFile(t)
	if os.Getenv("MC_WRITE_FIXTURES") == "1" {
		if err := os.WriteFile("testdata/flat.parquet", b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	have, err := os.ReadFile("testdata/flat.parquet")
	if err != nil {
		t.Fatal(err)
	}
	rows := func(tb *Table) string {
		var out []string
		for r := range tb.Rows {
			for c := range tb.Columns {
				out = append(out, cell(t, tb, r, c))
			}
		}
		return colNames(tb) + " " + strings.Join(out, "|")
	}
	if a, b := rows(readPQ(t, have)), rows(readPQ(t, b)); a != b {
		t.Errorf("testdata/flat.parquet reads as\n%s\nnot\n%s", a, b)
	}
}
