// Tests for the lazy line index: line starts and line numbers must agree
// with a naive full scan for every line ending style, in 8-bit and UTF-16
// text, across checkpoint and read-chunk boundaries, and when a file grows.

package lister

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// naiveStarts returns every line start (offsets after LF, CRLF or CR).
func naiveStarts(data []byte, unit int, be bool, start int) []int64 {
	out := []int64{int64(start)}
	for i := start; i+unit <= len(data); {
		c := unitAt(data, i, unit, be)
		switch c {
		case '\n':
			out = append(out, int64(i+unit))
		case '\r':
			j := i + unit
			if j+unit <= len(data) && unitAt(data, j, unit, be) == '\n' {
				j += unit
			}
			out = append(out, int64(j))
			i = j
			continue
		}
		i += unit
	}
	// A final break does not start a line.
	if len(out) > 1 && out[len(out)-1] >= int64(len(data)) {
		out = out[:len(out)-1]
	}
	return out
}

func randomText(r *rand.Rand, lines int) string {
	var b bytes.Buffer
	ends := []string{"\n", "\r\n", "\r", "\n\n"}
	for i := 0; i < lines; i++ {
		n := r.Intn(30)
		for k := 0; k < n; k++ {
			b.WriteByte("abcdefgh őú"[r.Intn(11)])
		}
		b.WriteString(ends[r.Intn(len(ends))])
	}
	return b.String()
}

func TestLineIndexAgainstNaive(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	text := randomText(r, 5000) + "tail without newline"
	cases := []struct {
		name  string
		enc   Encoding
		data  []byte
		start int
	}{
		{"utf8", UTF8, []byte(text), 0},
		{"utf8 bom", UTF8, append([]byte("\xEF\xBB\xBF"), text...), 3},
		{"utf16le", UTF16LE, append([]byte{0xFF, 0xFE}, EncodeString(UTF16LE, text)...), 2},
		{"utf16be", UTF16BE, EncodeString(UTF16BE, text), 0},
	}
	ctx := context.Background()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := naiveStarts(c.data, c.enc.unitSize(), c.enc == UTF16BE, c.start)
			ix := NewLineIndex(NewBytesSource(c.data), c.enc, int64(c.start))
			// Ask in a scattered order so both the main scan and the
			// checkpoint walks are exercised.
			order := r.Perm(len(want))
			for _, i := range order[:400] {
				got, err := ix.LineOffset(ctx, int64(i+1))
				if err != nil || got != want[i] {
					t.Fatalf("LineOffset(%d) = %d,%v want %d", i+1, got, err, want[i])
				}
			}
			for k := 0; k < 400; k++ {
				i := r.Intn(len(want))
				off := want[i]
				if i+1 < len(want) && want[i+1] > off+1 {
					off += int64(r.Intn(int(want[i+1] - off)))
				}
				got, err := ix.LineAt(ctx, off)
				if err != nil || got != int64(i+1) {
					t.Fatalf("LineAt(%d) = %d,%v want %d", off, got, err, i+1)
				}
			}
			n, err := ix.Count(ctx)
			if err != nil || n != int64(len(want)) {
				t.Fatalf("Count = %d,%v want %d", n, err, len(want))
			}
			if _, err := ix.LineOffset(ctx, n+1); !errors.Is(err, ErrBeyondEnd) {
				t.Fatalf("beyond end: %v", err)
			}
			if known, done := ix.Known(); !done || known != n {
				t.Fatalf("Known %d %v", known, done)
			}
		})
	}
}

func TestLineIndexSmallCases(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		data  string
		count int64
	}{
		{"", 1}, {"x", 1}, {"\n", 1}, {"a\nb", 2}, {"a\nb\n", 2}, {"a\r\n\r\nb", 3}, {"\r", 1}, {"\r\r", 2},
	}
	for _, tt := range tests {
		ix := NewLineIndex(NewBytesSource([]byte(tt.data)), UTF8, 0)
		n, err := ix.Count(ctx)
		if err != nil || n != tt.count {
			t.Errorf("%q: count %d want %d", tt.data, n, tt.count)
		}
		if l, _ := ix.LineAt(ctx, 0); l != 1 {
			t.Errorf("%q: LineAt(0)=%d", tt.data, l)
		}
		if off, err := ix.LineOffset(ctx, 0); off != 0 || err != nil {
			t.Errorf("%q: LineOffset(0)=%d %v", tt.data, off, err)
		}
	}
}

func TestLineIndexLargeChunks(t *testing.T) {
	// Lines across several 1 MiB scan chunks, with CRLF pairs placed right
	// on chunk edges.
	var b bytes.Buffer
	for b.Len() < 3*scanChunk+100 {
		b.WriteString("0123456789abcde\r\n")
	}
	data := b.Bytes()
	data[scanChunk-1] = '\r'
	data[scanChunk] = '\n'
	want := naiveStarts(data, 1, false, 0)
	ix := NewLineIndex(NewBytesSource(data), UTF8, 0)
	ctx := context.Background()
	n, _ := ix.Count(ctx)
	if n != int64(len(want)) {
		t.Fatalf("count %d want %d", n, len(want))
	}
	for _, i := range []int{0, 1, 1023, 1024, 1025, len(want) / 2, len(want) - 1} {
		got, err := ix.LineOffset(ctx, int64(i+1))
		if err != nil || got != want[i] {
			t.Fatalf("line %d: %d want %d", i+1, got, want[i])
		}
	}
	if s, size := ix.Progress(); s != size {
		t.Fatalf("progress %d/%d", s, size)
	}
}

func TestLineIndexCancel(t *testing.T) {
	data := bytes.Repeat([]byte("line\n"), 300000)
	ix := NewLineIndex(NewBytesSource(data), UTF8, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ix.Count(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestLineIndexGrowingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grow.log")
	if err := os.WriteFile(path, []byte("a\nb\r"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, f, err := OpenViewer(path, Options{Encoding: UTF8})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx := context.Background()
	if n, _ := v.Lines().Count(ctx); n != 2 {
		t.Fatalf("count %d", n)
	}
	// Append the LF that pairs with the trailing CR, plus more lines.
	af, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	af.WriteString("\nc\nd")
	af.Close()
	ch, err := v.Reload()
	if err != nil || !ch.Grown {
		t.Fatalf("reload %+v %v", ch, err)
	}
	if n, _ := v.Lines().Count(ctx); n != 4 {
		t.Fatalf("count after growth %d", n)
	}
	if off, _ := v.Lines().LineOffset(ctx, 3); off != 5 {
		t.Fatalf("line 3 at %d", off)
	}
	// Truncate: the index starts over.
	os.WriteFile(path, []byte("x\n"), 0o644)
	ch, _ = v.Reload()
	if !ch.Truncated {
		t.Fatalf("expected truncation %+v", ch)
	}
	if n, _ := v.Lines().Count(ctx); n != 1 {
		t.Fatalf("count after truncation %d", n)
	}
}
