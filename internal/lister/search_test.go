// Tests for Lister search: plain, case-insensitive (including accented
// letters), whole words, regular expressions, hex patterns and backward
// search, in several encodings, with matches straddling the internal search
// window edges, and with cancellation.

package lister

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func find(t *testing.T, data []byte, q Query, from, dataStart int64) (Match, bool) {
	t.Helper()
	s, err := Compile(q)
	if err != nil {
		t.Fatal(err)
	}
	m, ok, err := s.Find(context.Background(), NewBytesSource(data), from, dataStart)
	if err != nil {
		t.Fatal(err)
	}
	return m, ok
}

func TestSearchTable(t *testing.T) {
	hu := "Az ÁRVÍZTŰRŐ tükörfúrógép és az árvíztűrő gép.\nWord words sword word_x WORD."
	tests := []struct {
		name  string
		data  []byte
		q     Query
		from  int64
		want  int64 // -1 = not found
		wantN int64
	}{
		{"plain", []byte(hu), Query{Pattern: "tükör", CaseSensitive: true}, 0, int64(strings.Index(hu, "tükör")), int64(len("tükör"))},
		{"case sensitive miss", []byte(hu), Query{Pattern: "TÜKÖR", CaseSensitive: true}, 0, -1, 0},
		{"case insensitive accented", []byte(hu), Query{Pattern: "árvíztűrő"}, 0, int64(strings.Index(hu, "ÁRVÍZTŰRŐ")), int64(len("ÁRVÍZTŰRŐ"))},
		{"next after from", []byte(hu), Query{Pattern: "árvíztűrő"}, 4, int64(strings.Index(hu, "árvíztűrő")), int64(len("árvíztűrő"))},
		{"whole word", []byte(hu), Query{Pattern: "word", WholeWords: true}, int64(strings.Index(hu, "Word")) + 1, int64(strings.Index(hu, "WORD")), 4},
		{"whole word cs", []byte(hu), Query{Pattern: "word", WholeWords: true, CaseSensitive: true}, 0, -1, 0},
		{"regex", []byte(hu), Query{Pattern: `t\pL+gép`}, 0, int64(strings.Index(hu, "tükörfúrógép")), int64(len("tükörfúrógép"))},
		{"regex line anchor", []byte(hu), Query{Pattern: `^word`}, 0, int64(strings.Index(hu, "Word")), 4},
		{"regex whole word", []byte(hu), Query{Pattern: `w.rd`, Regex: true, WholeWords: true}, int64(strings.Index(hu, "sword")), int64(strings.Index(hu, "WORD")), 4},
		{"hex", []byte("abc\x00\xFF\x10def"), Query{Pattern: "00 ff 10", Hex: true}, 0, 3, 3},
		{"hex 0x style", []byte("abc\x00\xFF\x10def"), Query{Pattern: "0x00,0xFF", Hex: true}, 0, 3, 2},
		{"backward", []byte("one two one two one"), Query{Pattern: "one", Backward: true}, 16, 8, 3},
		{"backward none", []byte("one two"), Query{Pattern: "two", Backward: true}, 4, -1, 0},
		{"backward overlapping", []byte("aaaa"), Query{Pattern: "aa", Backward: true}, 4, 2, 2},
		{"utf16 case insensitive", append([]byte{0xFF, 0xFE}, EncodeString(UTF16LE, hu)...), Query{Pattern: "tükörfúrógép", Encoding: UTF16LE}, 2, 2 + 2*int64(len([]rune(hu[:strings.Index(hu, "tükör")]))), 2 * 12},
		{"1250", EncodeString(Win1250, hu), Query{Pattern: "ÁRVÍZTŰRŐ", Encoding: Win1250, CaseSensitive: true}, 0, 3, 9},
		{"852 insensitive", EncodeString(DOS852, "xx Őrült"), Query{Pattern: "őrült", Encoding: DOS852}, 0, 3, 5},
		{"shift-jis", EncodeString(ShiftJIS, "abc日本語テキスト"), Query{Pattern: "テキスト", Encoding: ShiftJIS}, 0, 9, 8},
		{"gbk", EncodeString(GBK, "中文文本abc文本"), Query{Pattern: "文本", Encoding: GBK}, 5, 11, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "regex" || tt.name == "regex line anchor" {
				tt.q.Regex = true
			}
			ds := int64(0)
			if bytes.HasPrefix(tt.data, []byte{0xFF, 0xFE}) {
				ds = 2
			}
			m, ok := find(t, tt.data, tt.q, tt.from, ds)
			if tt.want < 0 {
				if ok {
					t.Fatalf("unexpected match %+v", m)
				}
				return
			}
			if !ok || m.Offset != tt.want || m.Length != tt.wantN {
				t.Fatalf("got %+v ok=%v want offset %d len %d", m, ok, tt.want, tt.wantN)
			}
		})
	}
}

func TestSearchAcrossWindowEdge(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 2*searchWindow+100)
	for _, at := range []int{searchWindow - 3, 2*searchWindow - 1} {
		d := append([]byte(nil), data...)
		copy(d[at:], "NEEDLE")
		for _, q := range []Query{{Pattern: "needle"}, {Pattern: "NEE.LE", Regex: true}, {Pattern: "4E 45 45 44 4C 45", Hex: true}} {
			m, ok := find(t, d, q, 0, 0)
			if !ok || m.Offset != int64(at) || m.Length != 6 {
				t.Fatalf("%+v at %d: got %+v %v", q, at, m, ok)
			}
			q.Backward = true
			m, ok = find(t, d, q, int64(len(d)), 0)
			if !ok || m.Offset != int64(at) {
				t.Fatalf("backward %+v at %d: got %+v %v", q, at, m, ok)
			}
		}
	}
}

func TestSearchSequence(t *testing.T) {
	// Pressing F3 repeatedly visits every match once, in order; Shift+F3
	// walks them back.
	data := []byte(strings.Repeat("ab needle cd ", 50))
	s, _ := Compile(Query{Pattern: "needle"})
	src := NewBytesSource(data)
	var fw []int64
	for from := int64(0); ; {
		m, ok, err := s.Find(context.Background(), src, from, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		fw = append(fw, m.Offset)
		from = m.Offset + 1
	}
	if len(fw) != 50 {
		t.Fatalf("forward found %d", len(fw))
	}
	sb, _ := Compile(Query{Pattern: "needle", Backward: true})
	from := int64(len(data))
	for i := len(fw) - 1; i >= 0; i-- {
		m, ok, _ := sb.Find(context.Background(), src, from, 0)
		if !ok || m.Offset != fw[i] {
			t.Fatalf("backward %d: %+v", i, m)
		}
		from = m.Offset
	}
	all, err := s.FindAll(context.Background(), src, 0, int64(len(data)), 0, 10)
	if err != nil || len(all) != 10 || all[3].Offset != fw[3] {
		t.Fatalf("FindAll %v %v", all, err)
	}
}

func TestSearchErrorsAndCancel(t *testing.T) {
	if _, err := Compile(Query{}); !errors.Is(err, ErrEmptyPattern) {
		t.Fatal("empty pattern")
	}
	if _, err := Compile(Query{Pattern: "zz", Hex: true}); err == nil {
		t.Fatal("bad hex accepted")
	}
	if _, err := Compile(Query{Pattern: "(", Regex: true}); err == nil {
		t.Fatal("bad regex accepted")
	}
	s, _ := Compile(Query{Pattern: "nothere"})
	calls := 0
	s.Progress = func(pos, size int64) { calls++ }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := s.Find(ctx, NewBytesSource(make([]byte, 100)), 0, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	_, ok, _ := s.Find(context.Background(), NewBytesSource(make([]byte, searchWindow+10)), 0, 0)
	if ok || calls == 0 {
		t.Fatalf("progress not reported (calls=%d)", calls)
	}
}

func TestViewerSearch(t *testing.T) {
	data := append([]byte{0xEF, 0xBB, 0xBF}, "első\nmásodik ELSŐ"...)
	v := mustViewer(t, data, Options{})
	m, ok, err := v.Search(context.Background(), Query{Pattern: "első"}, 4)
	if err != nil || !ok || m.Offset != int64(3+len("első\nmásodik ")) || m.End() != int64(len(data)) {
		t.Fatalf("got %+v %v %v", m, ok, err)
	}
}
