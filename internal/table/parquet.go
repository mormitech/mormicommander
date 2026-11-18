// A small read-only reader of Apache Parquet files, written from the published format
// (https://parquet.apache.org/docs/file-format/) so no Arrow library is needed. It reads the footer, then the column
// chunks of each row group until it has MaxRows rows; the footer gives the exact row count, so the grid says how many
// rows the file has. Flat columns are read, including the leaves of nested groups that have no repeated level (shown
// as "group.field"); lists and maps are listed in the note but not shown. Supported: data pages v1 and v2, the PLAIN,
// dictionary, RLE, DELTA_BINARY_PACKED, DELTA_LENGTH_BYTE_ARRAY, DELTA_BYTE_ARRAY and BYTE_STREAM_SPLIT encodings, and
// the uncompressed, Snappy, gzip, Brotli, LZ4 and Zstandard codecs (not LZO). Every offset, count and length is
// checked, so a damaged or hostile file gives an error instead of a crash or a huge allocation.

package table

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"math/bits"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/snappy"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

const parquetMagic = "PAR1"

// parquetMaxFooter and parquetMaxPage cap the footer and one page (compressed or not) of a file.
const (
	parquetMaxFooter = 64 << 20
	parquetMaxPage   = 64 << 20
)

var errParquet = errors.New("the Parquet file is damaged")

// IsParquet says whether the bytes start with the magic of a Parquet file.
func IsParquet(sample []byte) bool { return bytes.HasPrefix(sample, []byte(parquetMagic)) }

// physical types
const (
	pqBoolean = iota
	pqInt32
	pqInt64
	pqInt96
	pqFloat
	pqDouble
	pqByteArray
	pqFixed
)

// pqLeaf is one leaf column of the schema.
type pqLeaf struct {
	name           string
	typ            int // physical type
	typeLen        int // FIXED_LEN_BYTE_ARRAY length
	maxDef, maxRep int
	logical        tstruct // LogicalType union (may be nil)
	converted      int64   // ConvertedType, -1 when there is none
	scale          int
	label          string // the type the grid shows
}

// ReadParquet reads the first MaxRows rows of a Parquet file of size bytes.
func ReadParquet(r io.ReaderAt, size int64) (*Table, error) {
	if size < 12 {
		return nil, errParquet
	}
	tail := make([]byte, 8)
	if _, err := r.ReadAt(tail, size-8); err != nil {
		return nil, errParquet
	}
	if string(tail[4:]) != parquetMagic {
		if string(tail[4:]) == "PARE" {
			return nil, errors.New("the Parquet file is encrypted")
		}
		return nil, errParquet
	}
	flen := int64(binary.LittleEndian.Uint32(tail))
	if flen <= 0 || flen > parquetMaxFooter || flen > size-12 {
		return nil, errParquet
	}
	footer := make([]byte, flen)
	if _, err := r.ReadAt(footer, size-8-flen); err != nil {
		return nil, errParquet
	}
	meta, _, err := decodeThrift(footer)
	if err != nil {
		return nil, err
	}
	leaves, err := parquetSchema(meta.list(2))
	if err != nil {
		return nil, err
	}
	t := &Table{Format: "parquet"}
	var shown []int // leaf index of each grid column
	var nested []string
	for i, l := range leaves {
		if l.maxRep > 0 {
			nested = append(nested, l.name)
			continue
		}
		shown = append(shown, i)
		t.Columns = append(t.Columns, Column{Name: clip(l.name), Type: l.label})
	}
	numRows := meta.int(3)
	var notes []string
	if len(nested) > 0 {
		notes = append(notes, fmt.Sprintf("Columns with lists or maps are not shown: %s.", clip(strings.Join(nested, ", "))))
	}
	bad := map[string]bool{}
	for _, g := range meta.list(4) {
		rg, ok := g.(tstruct)
		if !ok {
			return nil, errParquet
		}
		need := int(min(rg.int(3), int64(MaxRows-len(t.Rows))))
		if need <= 0 {
			continue
		}
		chunks := rg.list(1)
		if len(chunks) != len(leaves) {
			return nil, errParquet
		}
		block := make([][]*string, need)
		for i := range block {
			block[i] = make([]*string, len(shown))
		}
		for c, li := range shown {
			ch, _ := chunks[li].(tstruct)
			vals, err := readParquetColumn(r, size, ch, &leaves[li], need)
			if err != nil && !bad[leaves[li].name] {
				bad[leaves[li].name] = true
				notes = append(notes, fmt.Sprintf("The column %s can't be read: %v.", leaves[li].name, err))
			}
			for i := 0; i < need && i < len(vals); i++ {
				block[i][c] = vals[i]
			}
		}
		t.Rows = append(t.Rows, block...)
		if len(t.Rows) >= MaxRows {
			break
		}
	}
	if numRows > int64(len(t.Rows)) {
		t.Truncated = true
		t.Total = numRows
	}
	t.Note = strings.Join(notes, " ")
	return t, nil
}

// parquetSchema reads the flattened schema (a depth-first list of SchemaElement) into its leaf columns.
func parquetSchema(elems []any) ([]pqLeaf, error) {
	if len(elems) == 0 {
		return nil, errParquet
	}
	var leaves []pqLeaf
	pos := 1
	var walk func(n int, path []string, def, rep, depth int) error
	walk = func(n int, path []string, def, rep, depth int) error {
		if depth > thriftMaxDepth {
			return errParquet
		}
		for k := 0; k < n; k++ {
			if pos >= len(elems) {
				return errParquet
			}
			e, ok := elems[pos].(tstruct)
			if !ok {
				return errParquet
			}
			pos++
			d, r := def, rep
			switch e.int(3) {
			case 1:
				d++
			case 2:
				d++
				r++
			}
			p := append(append([]string{}, path...), e.str(4))
			kids := e.int(5)
			if kids > 0 {
				if kids > int64(len(elems)) {
					return errParquet
				}
				if err := walk(int(kids), p, d, r, depth+1); err != nil {
					return err
				}
				continue
			}
			typ, ok := e.i(1)
			if !ok || typ < 0 || typ > pqFixed {
				return errParquet
			}
			l := pqLeaf{name: strings.Join(p, "."), typ: int(typ), typeLen: int(e.int(2)), maxDef: d, maxRep: r,
				logical: e.sub(10), converted: -1, scale: int(e.int(7))}
			if c, ok := e.i(6); ok {
				l.converted = c
			}
			if dec := l.logical.sub(5); dec != nil {
				l.scale = int(dec.int(1))
			}
			l.label = parquetLabel(&l, e.int(8))
			leaves = append(leaves, l)
		}
		return nil
	}
	root, ok := elems[0].(tstruct)
	if !ok {
		return nil, errParquet
	}
	if err := walk(int(min(root.int(5), int64(len(elems)))), nil, 0, 0, 0); err != nil {
		return nil, err
	}
	return leaves, nil
}

var pqTypeNames = []string{"boolean", "int32", "int64", "int96", "float", "double", "binary", "fixed"}

var pqUnits = map[int16]string{1: "ms", 2: "µs", 3: "ns"}

// parquetLabel names a column's type as the grid header shows it.
func parquetLabel(l *pqLeaf, precision int64) string {
	lt := l.logical
	unit := func(s tstruct) string {
		for k := range s.sub(2) {
			return pqUnits[k]
		}
		return ""
	}
	switch {
	case lt.sub(1) != nil || l.converted == 0:
		return "string"
	case lt.sub(4) != nil || l.converted == 4:
		return "enum"
	case lt.sub(5) != nil:
		return fmt.Sprintf("decimal(%d,%d)", lt.sub(5).int(2), l.scale)
	case l.converted == 5:
		return fmt.Sprintf("decimal(%d,%d)", precision, l.scale)
	case lt.sub(6) != nil || l.converted == 6:
		return "date"
	case lt.sub(7) != nil:
		return "time(" + unit(lt.sub(7)) + ")"
	case l.converted == 7:
		return "time(ms)"
	case l.converted == 8:
		return "time(µs)"
	case lt.sub(8) != nil:
		return "timestamp(" + unit(lt.sub(8)) + ")"
	case l.converted == 9:
		return "timestamp(ms)"
	case l.converted == 10:
		return "timestamp(µs)"
	case lt.sub(10) != nil:
		it := lt.sub(10)
		if !it.flag(2, true) {
			return fmt.Sprintf("uint%d", it.int(1))
		}
		return fmt.Sprintf("int%d", it.int(1))
	case l.converted >= 11 && l.converted <= 14:
		return []string{"uint8", "uint16", "uint32", "uint64"}[l.converted-11]
	case l.converted >= 15 && l.converted <= 18:
		return []string{"int8", "int16", "int32", "int64"}[l.converted-15]
	case lt.sub(12) != nil || l.converted == 19:
		return "json"
	case lt.sub(13) != nil || l.converted == 20:
		return "bson"
	case lt.sub(14) != nil:
		return "uuid"
	case lt.sub(15) != nil:
		return "float16"
	case l.typ == pqInt96:
		return "timestamp(int96)"
	}
	return pqTypeNames[l.typ]
}

// readParquetColumn reads the first need values of a column chunk (nil = NULL). On an error it returns the values it
// read before it.
func readParquetColumn(r io.ReaderAt, size int64, ch tstruct, l *pqLeaf, need int) ([]*string, error) {
	if ch == nil {
		return nil, errParquet
	}
	if ch.str(1) != "" {
		return nil, errors.New("its data is in another file")
	}
	md := ch.sub(3)
	if md == nil {
		return nil, errParquet
	}
	codec := md.int(4)
	start := md.int(9)
	if dict, ok := md.i(11); ok && dict > 0 && dict < start {
		start = dict
	}
	end := start + md.int(7)
	if start < 4 || end > size || end < start {
		return nil, errParquet
	}
	var dict []*string
	out := make([]*string, 0, need)
	hdr := make([]byte, 64<<10)
	for pos := start; pos < end && len(out) < need; {
		// read the page header (a header with large statistics may need a bigger buffer)
		var ph tstruct
		var hl int
		for want := int64(len(hdr)); ; want *= 4 {
			want = min(want, end-pos)
			buf := hdr
			if int64(len(buf)) < want {
				buf = make([]byte, want)
			}
			n, err := r.ReadAt(buf[:want], pos)
			if err != nil && !errors.Is(err, io.EOF) {
				return out, err
			}
			ph, hl, err = decodeThrift(buf[:n])
			if err == nil {
				break
			}
			if want >= end-pos || want >= parquetMaxPage {
				return out, errParquet
			}
		}
		pos += int64(hl)
		csize, usize := ph.int(3), ph.int(2)
		if csize < 0 || usize < 0 || csize > parquetMaxPage || usize > parquetMaxPage || pos+csize > end {
			return out, errParquet
		}
		ptype := ph.int(1)
		if ptype != 0 && ptype != 2 && ptype != 3 {
			pos += csize // index page or something newer: skip it
			continue
		}
		raw := make([]byte, csize)
		if _, err := r.ReadAt(raw, pos); err != nil && !errors.Is(err, io.EOF) {
			return out, err
		}
		pos += csize
		switch ptype {
		case 2: // dictionary page
			dh := ph.sub(7)
			data, err := decompress(codec, raw, usize)
			if err != nil {
				return out, err
			}
			n := int(dh.int(1))
			if n < 0 || (n > len(data) && l.typ != pqBoolean) {
				return out, errParquet
			}
			d := &pqDecoder{l: l, b: data}
			if dict, err = d.plain(n); err != nil {
				return out, err
			}
		case 0: // data page v1: levels and values all compressed together
			dh := ph.sub(5)
			data, err := decompress(codec, raw, usize)
			if err != nil {
				return out, err
			}
			n := int(dh.int(1))
			if n < 0 || n > 64<<20 {
				return out, errParquet
			}
			var defs []uint32
			if l.maxDef > 0 {
				if dh.int(3) != 3 {
					return out, errors.New("its levels use an encoding that is not supported")
				}
				if len(data) < 4 {
					return out, errParquet
				}
				ln := int(binary.LittleEndian.Uint32(data))
				if ln < 0 || 4+ln > len(data) {
					return out, errParquet
				}
				if defs, err = rleHybrid(data[4:4+ln], bits.Len(uint(l.maxDef)), min(n, need-len(out))); err != nil {
					return out, err
				}
				data = data[4+ln:]
			}
			if out, err = l.page(out, need, data, n, defs, dh.int(2), dict); err != nil {
				return out, err
			}
		case 3: // data page v2: levels first and never compressed, then the values
			dh := ph.sub(8)
			n := int(dh.int(1))
			rl, dl := dh.int(6), dh.int(5)
			if n < 0 || n > 64<<20 || rl < 0 || dl < 0 || rl+dl > csize {
				return out, errParquet
			}
			var defs []uint32
			var err error
			if l.maxDef > 0 {
				if defs, err = rleHybrid(raw[rl:rl+dl], bits.Len(uint(l.maxDef)), min(n, need-len(out))); err != nil {
					return out, err
				}
			}
			data := raw[rl+dl:]
			if dh.flag(7, true) {
				if data, err = decompress(codec, data, usize-rl-dl); err != nil {
					return out, err
				}
			}
			if out, err = l.page(out, need, data, n, defs, dh.int(4), dict); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

// page decodes the values of one data page of n values and appends them to out, up to need (defs: the definition
// levels of the rows still needed, nil when the column is required).
func (l *pqLeaf) page(out []*string, need int, data []byte, n int, defs []uint32, enc int64, dict []*string) ([]*string, error) {
	// only decode what is needed: the values of the rows still to read
	rows := min(n, need-len(out))
	present := rows
	if defs != nil {
		present = 0
		for _, d := range defs[:rows] {
			if int(d) == l.maxDef {
				present++
			}
		}
	}
	vals, err := decodeValues(l, data, enc, present, dict)
	if err != nil {
		return out, err
	}
	k := 0
	for i := 0; i < rows; i++ {
		if defs != nil && int(defs[i]) != l.maxDef {
			out = append(out, nil)
			continue
		}
		out = append(out, vals[k])
		k++
	}
	return out, nil
}

// decodeValues decodes n present values in encoding enc.
func decodeValues(l *pqLeaf, data []byte, enc int64, n int, dict []*string) ([]*string, error) {
	d := &pqDecoder{l: l, b: data}
	switch enc {
	case 0: // PLAIN
		return d.plain(n)
	case 2, 8: // PLAIN_DICTIONARY, RLE_DICTIONARY
		if len(data) < 1 {
			if n == 0 {
				return nil, nil
			}
			return nil, errParquet
		}
		if dict == nil {
			return nil, errors.New("its dictionary page is missing")
		}
		idx, err := rleHybrid(data[1:], int(data[0]), n)
		if err != nil {
			return nil, err
		}
		out := make([]*string, n)
		for i, x := range idx {
			if int(x) >= len(dict) {
				return nil, errParquet
			}
			out[i] = dict[x]
		}
		return out, nil
	case 3: // RLE (booleans)
		if l.typ != pqBoolean || len(data) < 4 {
			return nil, errParquet
		}
		ln := int(binary.LittleEndian.Uint32(data))
		if ln < 0 || 4+ln > len(data) {
			return nil, errParquet
		}
		v, err := rleHybrid(data[4:4+ln], 1, n)
		if err != nil {
			return nil, err
		}
		out := make([]*string, n)
		for i, x := range v {
			out[i] = strp(strconv.FormatBool(x == 1))
		}
		return out, nil
	case 5: // DELTA_BINARY_PACKED
		if l.typ != pqInt32 && l.typ != pqInt64 {
			return nil, errParquet
		}
		v, err := d.deltaInts(n)
		if err != nil {
			return nil, err
		}
		out := make([]*string, n)
		for i, x := range v {
			if l.typ == pqInt32 {
				x = int64(int32(x))
			}
			out[i] = strp(l.formatInt(x))
		}
		return out, nil
	case 6: // DELTA_LENGTH_BYTE_ARRAY
		vals, err := d.deltaLengthArrays(n)
		if err != nil {
			return nil, err
		}
		out := make([]*string, n)
		for i, b := range vals {
			out[i] = strp(l.formatBytes(b))
		}
		return out, nil
	case 7: // DELTA_BYTE_ARRAY
		prefix, err := d.deltaInts(n)
		if err != nil {
			return nil, err
		}
		suffix, err := d.deltaLengthArrays(n)
		if err != nil {
			return nil, err
		}
		out := make([]*string, n)
		var prev []byte
		for i := range n {
			p := prefix[i]
			if p < 0 || p > int64(len(prev)) {
				return nil, errParquet
			}
			v := append(append([]byte{}, prev[:p]...), suffix[i]...)
			out[i] = strp(l.formatBytes(v))
			prev = v
		}
		return out, nil
	case 9: // BYTE_STREAM_SPLIT
		w := map[int]int{pqInt32: 4, pqFloat: 4, pqInt64: 8, pqDouble: 8, pqFixed: l.typeLen}[l.typ]
		if w <= 0 || n*w > len(data) {
			return nil, errParquet
		}
		joined := make([]byte, n*w)
		for i := range n {
			for k := range w {
				joined[i*w+k] = data[k*n+i]
			}
		}
		return (&pqDecoder{l: l, b: joined}).plain(n)
	}
	return nil, fmt.Errorf("it uses an encoding (%d) that is not supported", enc)
}

// pqDecoder reads values from a page's bytes.
type pqDecoder struct {
	l *pqLeaf
	b []byte
	p int
}

func (d *pqDecoder) take(n int) ([]byte, error) {
	if n < 0 || n > len(d.b)-d.p {
		return nil, errParquet
	}
	d.p += n
	return d.b[d.p-n : d.p], nil
}

// plain decodes n values in the PLAIN encoding.
func (d *pqDecoder) plain(n int) ([]*string, error) {
	l := d.l
	out := make([]*string, 0, min(n, len(d.b)+8))
	if l.typ == pqBoolean {
		for i := range n {
			if i/8 >= len(d.b) {
				return nil, errParquet
			}
			out = append(out, strp(strconv.FormatBool(d.b[i/8]>>(i%8)&1 == 1)))
		}
		return out, nil
	}
	for range n {
		var s string
		switch l.typ {
		case pqInt32:
			b, err := d.take(4)
			if err != nil {
				return nil, err
			}
			s = l.formatInt(int64(int32(binary.LittleEndian.Uint32(b))))
		case pqInt64:
			b, err := d.take(8)
			if err != nil {
				return nil, err
			}
			s = l.formatInt(int64(binary.LittleEndian.Uint64(b)))
		case pqInt96:
			b, err := d.take(12)
			if err != nil {
				return nil, err
			}
			nanos := int64(binary.LittleEndian.Uint64(b))
			day := int64(binary.LittleEndian.Uint32(b[8:]))
			s = formatTime((day-2440588)*86400*1e9+nanos, 1e9)
		case pqFloat:
			b, err := d.take(4)
			if err != nil {
				return nil, err
			}
			s = strconv.FormatFloat(float64(math.Float32frombits(binary.LittleEndian.Uint32(b))), 'g', -1, 32)
		case pqDouble:
			b, err := d.take(8)
			if err != nil {
				return nil, err
			}
			s = strconv.FormatFloat(math.Float64frombits(binary.LittleEndian.Uint64(b)), 'g', -1, 64)
		case pqByteArray:
			lb, err := d.take(4)
			if err != nil {
				return nil, err
			}
			b, err := d.take(int(binary.LittleEndian.Uint32(lb)))
			if err != nil {
				return nil, err
			}
			s = l.formatBytes(b)
		case pqFixed:
			if l.typeLen <= 0 {
				return nil, errParquet
			}
			b, err := d.take(l.typeLen)
			if err != nil {
				return nil, err
			}
			s = l.formatBytes(b)
		}
		out = append(out, strp(s))
	}
	return out, nil
}

func (d *pqDecoder) uvarint() (uint64, error) {
	v, n := binary.Uvarint(d.b[d.p:])
	if n <= 0 {
		return 0, errParquet
	}
	d.p += n
	return v, nil
}

func (d *pqDecoder) zigzag() (int64, error) {
	v, err := d.uvarint()
	return int64(v>>1) ^ -int64(v&1), err
}

// deltaInts decodes n values in the DELTA_BINARY_PACKED encoding (the block holds at least n values).
func (d *pqDecoder) deltaInts(n int) ([]int64, error) {
	blockSize, err := d.uvarint()
	if err != nil {
		return nil, err
	}
	minis, err := d.uvarint()
	if err != nil {
		return nil, err
	}
	total, err := d.uvarint()
	if err != nil {
		return nil, err
	}
	first, err := d.zigzag()
	if err != nil {
		return nil, err
	}
	if blockSize == 0 || minis == 0 || blockSize%128 != 0 || minis > blockSize || (blockSize/minis)%32 != 0 || blockSize > 1<<16 {
		return nil, errParquet
	}
	if total < uint64(n) {
		return nil, errParquet
	}
	per := int(blockSize / minis)
	out := make([]int64, 0, n)
	if n == 0 {
		return out, d.skipDelta(total, minis, per)
	}
	out = append(out, first)
	prev := first
	read := uint64(1)
	for read < total {
		minDelta, err := d.zigzag()
		if err != nil {
			return nil, err
		}
		widths, err := d.take(int(minis))
		if err != nil {
			return nil, err
		}
		for _, w := range widths {
			if read >= total {
				break // the rest of the miniblocks are not written
			}
			if w > 64 {
				return nil, errParquet
			}
			b, err := d.take(per * int(w) / 8)
			if err != nil {
				return nil, err
			}
			for i := 0; i < per && read < total; i++ {
				if len(out) == n { // the rest is not needed: only step past it
					read += min(uint64(per-i), total-read)
					break
				}
				prev += minDelta + int64(unpack(b, i, int(w)))
				out = append(out, prev)
				read++
			}
		}
	}
	return out, nil
}

// skipDelta reads past the blocks of a DELTA_BINARY_PACKED run when none of its values are needed.
func (d *pqDecoder) skipDelta(total, minis uint64, per int) error {
	for read := uint64(1); read < total; {
		if _, err := d.zigzag(); err != nil {
			return err
		}
		widths, err := d.take(int(minis))
		if err != nil {
			return err
		}
		for _, w := range widths {
			if read >= total {
				break
			}
			if _, err := d.take(per * int(w) / 8); err != nil {
				return err
			}
			read += uint64(per)
		}
	}
	return nil
}

// deltaLengthArrays decodes n byte arrays in the DELTA_LENGTH_BYTE_ARRAY encoding.
func (d *pqDecoder) deltaLengthArrays(n int) ([][]byte, error) {
	lens, err := d.deltaInts(n)
	if err != nil {
		return nil, err
	}
	out := make([][]byte, n)
	for i, ln := range lens {
		if ln < 0 || ln > int64(len(d.b)) {
			return nil, errParquet
		}
		if out[i], err = d.take(int(ln)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// unpack reads the i-th value of width w bits from bit-packed bytes (least significant bit first).
func unpack(b []byte, i, w int) uint64 {
	var v uint64
	bit := i * w
	for k := 0; k < w; k++ {
		if b[(bit+k)/8]>>((bit+k)%8)&1 == 1 {
			v |= 1 << k
		}
	}
	return v
}

// rleHybrid decodes n values of the RLE / bit-packing hybrid encoding of width w bits.
func rleHybrid(b []byte, w, n int) ([]uint32, error) {
	if w < 0 || w > 32 {
		return nil, errParquet
	}
	out := make([]uint32, 0, n)
	if w == 0 {
		return make([]uint32, n), nil
	}
	p := 0
	for len(out) < n {
		h, k := binary.Uvarint(b[p:])
		if k <= 0 {
			return nil, errParquet
		}
		p += k
		if h&1 == 0 { // a run of one value
			run := h >> 1
			bw := (w + 7) / 8
			if p+bw > len(b) {
				return nil, errParquet
			}
			var v uint32
			for i := 0; i < bw; i++ {
				v |= uint32(b[p+i]) << (8 * i)
			}
			p += bw
			for ; run > 0 && len(out) < n; run-- {
				out = append(out, v)
			}
			continue
		}
		groups := h >> 1
		if groups > uint64(len(b)) || p+int(groups)*w > len(b) {
			return nil, errParquet
		}
		packed := b[p : p+int(groups)*w]
		p += int(groups) * w
		for i := 0; i < int(groups)*8 && len(out) < n; i++ {
			out = append(out, uint32(unpack(packed, i, w)))
		}
	}
	return out, nil
}

// decompress undoes a page's codec; size is how big the page is uncompressed.
func decompress(codec int64, b []byte, size int64) ([]byte, error) {
	if size < 0 || size > parquetMaxPage {
		return nil, errParquet
	}
	var out []byte
	var err error
	switch codec {
	case 0:
		return b, nil
	case 1:
		var n int
		if n, err = snappy.DecodedLen(b); err == nil && int64(n) <= parquetMaxPage {
			out, err = snappy.Decode(nil, b)
		} else if err == nil {
			err = errParquet
		}
	case 2:
		var zr *gzip.Reader
		if zr, err = gzip.NewReader(bytes.NewReader(b)); err == nil {
			out, err = io.ReadAll(io.LimitReader(zr, size+1))
		}
	case 4:
		out, err = io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(b)), size+1))
	case 5:
		if out, err = hadoopLZ4(b, size); err != nil {
			out, err = lz4Block(b, size) // some writers store a plain LZ4 block under this codec
		}
	case 6:
		zstdOnce.Do(func() {
			zstdDec, zstdErr = zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(parquetMaxPage))
		})
		if err = zstdErr; err == nil {
			out, err = zstdDec.DecodeAll(b, make([]byte, 0, size))
		}
	case 7:
		out, err = lz4Block(b, size)
	default:
		return nil, fmt.Errorf("it is compressed with a codec (%d) that is not supported", codec)
	}
	if err != nil {
		return nil, fmt.Errorf("its data can't be uncompressed: %v", err)
	}
	if int64(len(out)) != size {
		return nil, errParquet
	}
	return out, nil
}

// zstdDec is one Zstandard decoder for every page (DecodeAll may be called from several goroutines at once).
var (
	zstdOnce sync.Once
	zstdDec  *zstd.Decoder
	zstdErr  error
)

func lz4Block(b []byte, size int64) ([]byte, error) {
	out := make([]byte, size)
	n, err := lz4.UncompressBlock(b, out)
	return out[:n], err
}

// hadoopLZ4 reads Hadoop's LZ4 framing: blocks of [uncompressed size, compressed size] (big-endian) and an LZ4 block.
func hadoopLZ4(b []byte, size int64) ([]byte, error) {
	out := make([]byte, 0, size)
	for len(b) > 0 {
		if len(b) < 8 {
			return nil, errParquet
		}
		us, cs := int64(binary.BigEndian.Uint32(b)), int64(binary.BigEndian.Uint32(b[4:]))
		if cs > int64(len(b)-8) || int64(len(out))+us > size {
			return nil, errParquet
		}
		blk, err := lz4Block(b[8:8+cs], us)
		if err != nil || int64(len(blk)) != us {
			return nil, errParquet
		}
		out = append(out, blk...)
		b = b[8+cs:]
	}
	return out, nil
}

// formatInt writes an INT32 or INT64 value as its logical type says: a date, a time, a timestamp, a decimal or an
// unsigned number.
func (l *pqLeaf) formatInt(v int64) string {
	lt := l.logical
	unitOf := func(s tstruct, def int64) int64 {
		for k := range s.sub(2) {
			if u, ok := map[int16]int64{1: 1e3, 2: 1e6, 3: 1e9}[k]; ok {
				return u
			}
		}
		return def
	}
	switch {
	case lt.sub(6) != nil || l.converted == 6:
		return time.Unix(v*86400, 0).UTC().Format("2006-01-02")
	case lt.sub(8) != nil:
		return formatTime(v, unitOf(lt.sub(8), 1e3))
	case l.converted == 9:
		return formatTime(v, 1e3)
	case l.converted == 10:
		return formatTime(v, 1e6)
	case lt.sub(7) != nil:
		return formatClock(v, unitOf(lt.sub(7), 1e3))
	case l.converted == 7:
		return formatClock(v, 1e3)
	case l.converted == 8:
		return formatClock(v, 1e6)
	case lt.sub(5) != nil || l.converted == 5:
		return scaleDecimal(big.NewInt(v), l.scale)
	case (lt.sub(10) != nil && !lt.sub(10).flag(2, true)) || (l.converted >= 11 && l.converted <= 14):
		if l.typ == pqInt32 {
			return strconv.FormatUint(uint64(uint32(v)), 10)
		}
		return strconv.FormatUint(uint64(v), 10)
	}
	return strconv.FormatInt(v, 10)
}

// formatTime writes a count of units (per second) since 1970 as a UTC date and time.
func formatTime(v, perSec int64) string {
	t := time.Unix(v/perSec, (v%perSec)*(1e9/perSec)).UTC()
	switch perSec {
	case 1e3:
		return t.Format("2006-01-02 15:04:05.000")
	case 1e6:
		return t.Format("2006-01-02 15:04:05.000000")
	}
	return t.Format("2006-01-02 15:04:05.000000000")
}

// formatClock writes a time of day given in units per second.
func formatClock(v, perSec int64) string {
	sec := v / perSec
	frac := v % perSec
	digits := len(strconv.FormatInt(perSec, 10)) - 1
	return fmt.Sprintf("%02d:%02d:%02d.%0*d", sec/3600, sec/60%60, sec%60, digits, frac)
}

// scaleDecimal writes an unscaled integer with scale digits after the decimal point.
func scaleDecimal(v *big.Int, scale int) string {
	s := v.String()
	if scale <= 0 || scale > 100 {
		return s
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	if len(s) <= scale {
		s = strings.Repeat("0", scale-len(s)+1) + s
	}
	s = s[:len(s)-scale] + "." + s[len(s)-scale:]
	if neg {
		s = "-" + s
	}
	return s
}

// formatBytes writes a BYTE_ARRAY or FIXED_LEN_BYTE_ARRAY value: text, a decimal, a UUID, a half-precision float, or
// hex for binary data.
func (l *pqLeaf) formatBytes(b []byte) string {
	lt := l.logical
	switch {
	case lt.sub(5) != nil || l.converted == 5:
		v := new(big.Int).SetBytes(b)
		if len(b) > 0 && b[0]&0x80 != 0 { // two's complement
			v.Sub(v, new(big.Int).Lsh(big.NewInt(1), uint(8*len(b))))
		}
		return scaleDecimal(v, l.scale)
	case lt.sub(14) != nil && len(b) == 16:
		h := hex.EncodeToString(b)
		return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	case lt.sub(15) != nil && len(b) == 2:
		return strconv.FormatFloat(float64(half(binary.LittleEndian.Uint16(b))), 'g', -1, 32)
	case l.typ == pqByteArray && (lt.sub(13) == nil && l.converted != 20) && utf8.Valid(b):
		return clip(string(b)) // strings, enums, JSON, and binary columns that hold text
	}
	const show = 32
	h := strings.ToUpper(hex.EncodeToString(b[:min(len(b), show)]))
	if len(b) > show {
		return fmt.Sprintf("x'%s…' (%d bytes)", h, len(b))
	}
	return "x'" + h + "'"
}

// half turns an IEEE 754 half-precision float into a float32.
func half(h uint16) float32 {
	sign := uint32(h>>15) << 31
	exp := uint32(h>>10) & 0x1f
	frac := uint32(h) & 0x3ff
	switch {
	case exp == 0 && frac == 0:
		return math.Float32frombits(sign)
	case exp == 0: // subnormal
		f := float32(frac) / 1024 * float32(math.Pow(2, -14))
		if sign != 0 {
			return -f
		}
		return f
	case exp == 31:
		return math.Float32frombits(sign | 0xff<<23 | frac<<13)
	}
	return math.Float32frombits(sign | (exp+112)<<23 | frac<<13)
}
