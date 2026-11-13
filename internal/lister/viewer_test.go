// Tests for the view model: text layout (line endings, tabs, wrapping,
// encodings, long lines), binary and hex rows, scrolling forward and back
// (which must visit exactly the same rows), go to offset, selection copy,
// and opening a multi-gigabyte sparse file without reading it.

package lister

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustViewer(t *testing.T, data []byte, opt Options) *Viewer {
	t.Helper()
	if opt.ANSI == "" {
		opt.ANSI = Win1250
	}
	v, err := NewViewer(NewBytesSource(data), opt)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func rowTexts(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Text
	}
	return out
}

// forwardStarts pages through the whole view and returns every row offset.
func forwardStarts(t *testing.T, v *Viewer) []int64 {
	t.Helper()
	var out []int64
	off := v.DataStart()
	for guard := 0; guard < 1_000_000; guard++ {
		rows, err := v.Rows(off, 7)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			return out
		}
		for _, r := range rows {
			if len(out) > 0 && r.Offset <= out[len(out)-1] {
				t.Fatalf("rows not increasing: %d after %d", r.Offset, out[len(out)-1])
			}
			out = append(out, r.Offset)
		}
		off = rows[len(rows)-1].End
		if off >= v.Size() {
			return out
		}
	}
	t.Fatal("too many rows")
	return nil
}

// backwardStarts walks from the end to the start one row at a time.
func backwardStarts(t *testing.T, v *Viewer) []int64 {
	t.Helper()
	var out []int64
	off := v.Size()
	for guard := 0; guard < 1_000_000; guard++ {
		prev, err := v.Back(off, 1)
		if err != nil {
			t.Fatal(err)
		}
		if prev >= off {
			break
		}
		out = append(out, prev)
		off = prev
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func equalInts(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTextRows(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		opt  Options
		want []string
	}{
		{"lf", []byte("one\ntwo\nthree"), Options{}, []string{"one", "two", "three"}},
		{"trailing lf", []byte("one\ntwo\n"), Options{}, []string{"one", "two"}},
		{"crlf", []byte("one\r\ntwo\r\n\r\nfour"), Options{}, []string{"one", "two", "", "four"}},
		{"lone cr", []byte("one\rtwo\rthree"), Options{}, []string{"one", "two", "three"}},
		{"empty lines", []byte("\n\nx"), Options{}, []string{"", "", "x"}},
		{"tabs", []byte("a\tb\tc\n\tx"), Options{TabWidth: 4}, []string{"a   b   c", "    x"}},
		{"control chars", []byte("a\x01b\x7fc"), Options{}, []string{"a b c"}},
		{"utf8 hungarian", []byte("árvíztűrő\ntükörfúrógép"), Options{}, []string{"árvíztűrő", "tükörfúrógép"}},
		{"utf8 bom skipped", []byte("\xEF\xBB\xBFhi\nthere"), Options{}, []string{"hi", "there"}},
		{"ansi 1250", EncodeString(Win1250, "árvíztűrő\nő"), Options{Encoding: Win1250}, []string{"árvíztűrő", "ő"}},
		{"dos 852", EncodeString(DOS852, "tükör\r\nfúró"), Options{Encoding: DOS852}, []string{"tükör", "fúró"}},
		{"utf16le bom", append([]byte{0xFF, 0xFE}, EncodeString(UTF16LE, "első\r\nmásodik\n")...), Options{}, []string{"első", "második"}},
		{"utf16be", EncodeString(UTF16BE, "ab\ncd"), Options{Encoding: UTF16BE}, []string{"ab", "cd"}},
		{"wrap words", []byte("the quick brown fox jumps"), Options{Wrap: true, Width: 10}, []string{"the quick ", "brown fox ", "jumps"}},
		{"wrap hard", []byte("abcdefghijklmnop"), Options{Wrap: true, Width: 5}, []string{"abcde", "fghij", "klmno", "p"}},
		{"wrap wide chars", []byte("日本語日本語"), Options{Wrap: true, Width: 5}, []string{"日本", "語日", "本語"}},
		{"no wrap keeps long line", []byte("the quick brown fox"), Options{Width: 5}, []string{"the quick brown fox"}},
		{"shift-jis", EncodeString(ShiftJIS, "日本語\nテキスト"), Options{Encoding: ShiftJIS}, []string{"日本語", "テキスト"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := mustViewer(t, tt.data, tt.opt)
			rows, err := v.Rows(0, 100)
			if err != nil {
				t.Fatal(err)
			}
			got := rowTexts(rows)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("got %q want %q", got, tt.want)
			}
			if f, b := forwardStarts(t, v), backwardStarts(t, v); !equalInts(f, b) {
				t.Fatalf("forward %v != backward %v", f, b)
			}
		})
	}
}

func TestRowMapAndOffsets(t *testing.T) {
	data := []byte("aő\tb\r\nnext")
	v := mustViewer(t, data, Options{TabWidth: 4})
	rows, _ := v.Rows(0, 10)
	if len(rows) != 2 {
		t.Fatalf("rows %d", len(rows))
	}
	r := rows[0]
	if r.Offset != 0 || r.End != 7 || rows[1].Offset != 7 || rows[1].End != 11 {
		t.Fatalf("offsets %+v %+v", r, rows[1])
	}
	// "aő" + tab to column 4 + "b": runes a,ő,' ',' ',b
	want := []int32{0, 1, 3, 3, 4}
	if len(r.Map) != len(want) {
		t.Fatalf("map %v", r.Map)
	}
	for i := range want {
		if r.Map[i] != want[i] {
			t.Fatalf("map %v want %v", r.Map, want)
		}
	}
	if !r.LineStart {
		t.Fatal("LineStart")
	}
}

func TestWrapContinuationRows(t *testing.T) {
	v := mustViewer(t, []byte("aaaa bbbb cccc\nx"), Options{Wrap: true, Width: 5})
	rows, _ := v.Rows(0, 10)
	if len(rows) != 4 || !rows[0].LineStart || rows[1].LineStart || !rows[3].LineStart {
		t.Fatalf("rows %+v", rows)
	}
	// RowStart inside the second wrapped row aligns to it.
	if s, _ := v.RowStart(rows[1].Offset + 2); s != rows[1].Offset {
		t.Fatalf("RowStart %d want %d", s, rows[1].Offset)
	}
	// Rows starting mid-line begin with that wrapped row.
	got, _ := v.Rows(rows[2].Offset, 2)
	if got[0].Text != "cccc" || got[1].Text != "x" {
		t.Fatalf("got %q", rowTexts(got))
	}
}

func TestScrollConsistencyMixed(t *testing.T) {
	var b bytes.Buffer
	for i := 0; i < 300; i++ {
		switch i % 5 {
		case 0:
			b.WriteString("short\n")
		case 1:
			b.WriteString(strings.Repeat("word ", i%37) + "\r\n")
		case 2:
			b.WriteString("\n")
		case 3:
			b.WriteString("tab\there\tő\r")
		case 4:
			b.WriteString(strings.Repeat("x", 250) + "\n")
		}
	}
	data := b.Bytes()
	for _, opt := range []Options{{}, {Wrap: true, Width: 40}, {Wrap: true, Width: 7}, {Mode: ModeBinary, Width: 33}, {Mode: ModeHex}} {
		v := mustViewer(t, data, opt)
		f, bk := forwardStarts(t, v), backwardStarts(t, v)
		if !equalInts(f, bk) {
			t.Fatalf("%+v: forward %d rows, backward %d rows", opt, len(f), len(bk))
		}
		// Every offset inside the data aligns to one of the row starts.
		for off := int64(0); off < int64(len(data)); off += 13 {
			s, err := v.RowStart(off)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, x := range f {
				if x == s {
					found = true
					break
				}
			}
			if !found || s > off {
				t.Fatalf("%+v: RowStart(%d)=%d not a row start", opt, off, s)
			}
		}
		// Forward then Back by the same amount returns to the start.
		mid := f[len(f)/2]
		fw, _ := v.Forward(mid, 5)
		bk2, _ := v.Back(fw, 5)
		if bk2 != mid {
			t.Fatalf("%+v: forward/back %d -> %d -> %d", opt, mid, fw, bk2)
		}
	}
}

func TestUTF16ScrollConsistency(t *testing.T) {
	text := strings.Repeat("első sor\r\nmásodik sor ami hosszabb\nharmadik\r", 40)
	data := append([]byte{0xFF, 0xFE}, EncodeString(UTF16LE, text)...)
	for _, opt := range []Options{{}, {Wrap: true, Width: 9}} {
		v := mustViewer(t, data, opt)
		if v.Encoding() != UTF16LE || v.DataStart() != 2 {
			t.Fatalf("detected %s start %d", v.Encoding(), v.DataStart())
		}
		f, b := forwardStarts(t, v), backwardStarts(t, v)
		if !equalInts(f, b) {
			t.Fatalf("forward %v\nbackward %v", f, b)
		}
		for _, s := range f {
			if (s-2)%2 != 0 {
				t.Fatalf("odd row start %d", s)
			}
		}
	}
}

func TestLongLineSplit(t *testing.T) {
	data := bytes.Repeat([]byte("abcdefgh"), MaxLineBytes/8*3+5) // no newlines at all
	v := mustViewer(t, data, Options{})
	rows, _ := v.Rows(0, 10)
	if len(rows) != 4 {
		t.Fatalf("rows %d", len(rows))
	}
	if rows[1].Offset != MaxLineBytes || len(rows[0].Text) != MaxLineBytes {
		t.Fatalf("split at %d len %d", rows[1].Offset, len(rows[0].Text))
	}
	if f, b := forwardStarts(t, v), backwardStarts(t, v); !equalInts(f, b) {
		t.Fatalf("forward %v backward %v", f, b)
	}
}

func TestLongUTF8LineSplitsOnCharBoundary(t *testing.T) {
	data := []byte(strings.Repeat("ő", MaxLineBytes)) // 2 bytes each
	data = append([]byte("x"), data...)               // make boundaries odd
	v := mustViewer(t, data, Options{})
	rows, _ := v.Rows(0, 3)
	for _, r := range rows {
		if strings.ContainsRune(r.Text, '�') {
			t.Fatalf("row at %d contains a broken character", r.Offset)
		}
	}
}

func TestBinaryAndHexRows(t *testing.T) {
	data := []byte("Hello\x00\x01\x02World\xff\x7f!")
	v := mustViewer(t, data, Options{Mode: ModeBinary, Width: 8, Encoding: Win1252})
	rows, _ := v.Rows(0, 10)
	if len(rows) != 2 || rows[0].Text != "Hello ☺☻" || rows[1].Offset != 8 {
		t.Fatalf("binary rows %q", rowTexts(rows))
	}
	if rows[1].Text != "Worldÿ⌂!" {
		t.Fatalf("binary row 2 %q", rows[1].Text)
	}
	v.SetOptions(Options{Mode: ModeHex, HexWidth: 16, Encoding: Win1252})
	rows, _ = v.Rows(3, 10)
	if len(rows) != 1 || rows[0].Offset != 0 {
		t.Fatalf("hex rows %+v", rows)
	}
	if rows[0].Hex != "48 65 6C 6C 6F 00 01 02  57 6F 72 6C 64 FF 7F 21" {
		t.Fatalf("hex %q", rows[0].Hex)
	}
	if rows[0].Text != "Hello...Worldÿ.!" {
		t.Fatalf("hex chars %q", rows[0].Text)
	}
	line := v.FormatHexRow(rows[0])
	if !strings.HasPrefix(line, "00000000: 48 65") || !strings.HasSuffix(line, "  Hello...Worldÿ.!") {
		t.Fatalf("FormatHexRow %q", line)
	}
	last, _ := v.LastPage(1)
	if last != 0 {
		t.Fatalf("LastPage %d", last)
	}
	// Short final row is padded so the char column lines up.
	v.SetOptions(Options{Mode: ModeHex, Encoding: Win1252})
	short := mustViewer(t, []byte("AB"), Options{Mode: ModeHex})
	r2, _ := short.Rows(0, 1)
	if got, want := len([]rune(short.FormatHexRow(r2[0]))), len([]rune(line))-14; got != want {
		t.Fatalf("padded row length %d want %d", got, want)
	}
}

func TestBackAndLastPageFixed(t *testing.T) {
	data := make([]byte, 100)
	v := mustViewer(t, data, Options{Mode: ModeHex})
	tests := []struct {
		off  int64
		n    int
		want int64
	}{
		{100, 1, 96}, {100, 2, 80}, {50, 1, 32}, {10, 5, 0}, {0, 1, 0},
	}
	for _, tt := range tests {
		if got, _ := v.Back(tt.off, tt.n); got != tt.want {
			t.Errorf("Back(%d,%d)=%d want %d", tt.off, tt.n, got, tt.want)
		}
	}
	if s, _ := v.RowStart(37); s != 32 {
		t.Fatalf("RowStart %d", s)
	}
}

func TestLastPageText(t *testing.T) {
	v := mustViewer(t, []byte("1\n2\n3\n4\n5\n"), Options{})
	top, err := v.LastPage(2)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := v.Rows(top, 5)
	if strings.Join(rowTexts(rows), ",") != "4,5" {
		t.Fatalf("last page %q", rowTexts(rows))
	}
	if end, _ := v.AtEnd(top, 2); !end {
		t.Fatal("AtEnd")
	}
	if end, _ := v.AtEnd(0, 2); end {
		t.Fatal("AtEnd at top")
	}
	// Forward stops at the last row.
	if f, _ := v.Forward(0, 100); f != 8 {
		t.Fatalf("Forward clamp %d", f)
	}
}

func TestEmptyFile(t *testing.T) {
	for _, mode := range []Mode{ModeText, ModeBinary, ModeHex} {
		v := mustViewer(t, nil, Options{Mode: mode})
		rows, err := v.Rows(0, 10)
		if err != nil || len(rows) != 0 {
			t.Fatalf("%v: rows %v err %v", mode, rows, err)
		}
		if b, _ := v.Back(0, 3); b != 0 {
			t.Fatalf("Back %d", b)
		}
		if p := v.PercentAt(0); p != 100 {
			t.Fatalf("percent %v", p)
		}
	}
}

func TestPercent(t *testing.T) {
	v := mustViewer(t, make([]byte, 1000), Options{Mode: ModeBinary})
	if v.PercentAt(500) != 50 || v.OffsetAtPercent(25) != 250 || v.OffsetAtPercent(150) != 1000 {
		t.Fatal("percent conversion")
	}
}

func TestOptionsAndEncodingSwitch(t *testing.T) {
	data := EncodeString(Win1250, "tőz\n")
	v := mustViewer(t, data, Options{ANSI: Win1250})
	if v.Encoding() != Win1250 {
		t.Fatalf("detected %s", v.Encoding())
	}
	rows, _ := v.Rows(0, 1)
	if rows[0].Text != "tőz" {
		t.Fatalf("got %q", rows[0].Text)
	}
	v.SetOptions(Options{Encoding: DOS852})
	if o := v.Options(); o.Encoding != DOS852 || o.TabWidth != 8 || o.Width != 80 {
		t.Fatalf("options %+v", o)
	}
	rows, _ = v.Rows(0, 1)
	if rows[0].Text == "tőz" {
		t.Fatal("encoding switch had no effect")
	}
	// Switching back to auto-detection.
	v.SetOptions(Options{})
	if v.Encoding() != Win1250 {
		t.Fatal("auto detection not restored")
	}
	if v.Detection().Encoding != Win1250 {
		t.Fatal("Detection")
	}
}

func TestBOMOnlyHiddenForMatchingEncoding(t *testing.T) {
	data := []byte("\xEF\xBB\xBFabc")
	v := mustViewer(t, data, Options{})
	if v.DataStart() != 3 {
		t.Fatal("BOM not skipped")
	}
	v.SetOptions(Options{Encoding: Win1252})
	if v.DataStart() != 0 {
		t.Fatal("BOM should be visible in another encoding")
	}
}

func TestTextHexAndCopy(t *testing.T) {
	data := EncodeString(UTF16LE, "árvíz\r\ntűrő")
	v := mustViewer(t, data, Options{Encoding: UTF16LE})
	s, err := v.Text(0, int64(len(data)), 0)
	if err != nil || s != "árvíz\r\ntűrő" {
		t.Fatalf("Text %q %v", s, err)
	}
	if _, err := v.Text(0, int64(len(data)), 4); err != ErrTooLarge {
		t.Fatalf("limit: %v", err)
	}
	h, _ := v.HexText(0, 4, 0)
	if h != "E1 00 72 00" {
		t.Fatalf("HexText %q", h)
	}
	if FormatHex([]byte{0xAB, 1}, 0) != "AB 01" {
		t.Fatal("FormatHex")
	}
	var out bytes.Buffer
	n, err := v.CopyRange(context.Background(), &out, 2, 6)
	if err != nil || n != 4 || !bytes.Equal(out.Bytes(), data[2:6]) {
		t.Fatalf("CopyRange %d %v %x", n, err, out.Bytes())
	}
	dump, _ := v.HexDump(0, int64(len(data)))
	if strings.Count(dump, "\n") != 2 || !strings.HasPrefix(dump, "00000000: E1 00") {
		t.Fatalf("HexDump %q", dump)
	}
	if v.Options().Mode != ModeText {
		t.Fatal("HexDump must restore the mode")
	}
}

func TestHugeSparseFileOpensInstantly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	const size = 6 << 30 // 6 GiB, sparse: takes no real disk space
	if err := f.Truncate(size - 16); err != nil {
		t.Skip("sparse files not supported:", err)
	}
	f.Seek(size-16, 0)
	f.WriteString("\nlast line here\n")
	f.Close()

	start := time.Now()
	v, lf, err := OpenViewer(path, Options{Encoding: UTF8})
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()
	if v.OffsetDigits() != 9 {
		t.Fatalf("offset digits %d", v.OffsetDigits())
	}
	top, err := v.LastPage(1)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := v.Rows(top, 3)
	if err != nil || len(rows) != 1 || rows[0].Text != "last line here" {
		t.Fatalf("rows %q err %v", rowTexts(rows), err)
	}
	// Jump to the middle: a run of NULs, shown as split pieces.
	mid := v.OffsetAtPercent(50)
	s, err := v.RowStart(mid)
	if err != nil || s > mid || mid-s > MaxLineBytes {
		t.Fatalf("RowStart mid %d -> %d %v", mid, s, err)
	}
	v.SetOptions(Options{Mode: ModeHex})
	hr, _ := v.Rows(size-16, 1)
	if len(hr) != 1 || !strings.HasPrefix(hr[0].Hex, "0A 6C 61 73") {
		t.Fatalf("hex at end %+v", hr)
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("too slow: %v", el)
	}
}
