// Tests for the binary comparison and the file equality helpers. Files are
// created in t.TempDir().
package compare

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompareBinary(t *testing.T) {
	big := bytes.Repeat([]byte{7}, binChunk*3+5)
	bigMod := append([]byte(nil), big...)
	bigMod[binChunk-1] = 8 // across a chunk boundary
	bigMod[binChunk] = 9
	bigMod[len(big)-1] = 1

	tests := []struct {
		name      string
		a, b      []byte
		max       int
		diffs     []ByteDiff
		runs      []ByteRun
		sizeA     int64
		sizeB     int64
		truncated bool
		identical bool
		first     int64
	}{
		{"both empty", nil, nil, 0, nil, nil, 0, 0, false, true, -1},
		{"same", []byte("abc"), []byte("abc"), 0, nil, nil, 3, 3, false, true, -1},
		{"one byte", []byte("abc"), []byte("aXc"), 0,
			[]ByteDiff{{1, 'b', 'X'}}, []ByteRun{{1, 1}}, 3, 3, false, false, 1},
		{"length only", []byte("abc"), []byte("abcdef"), 0, nil, nil, 3, 6, false, false, 3},
		{"length and diff", []byte("abcdef"), []byte("aXc"), 0,
			[]ByteDiff{{1, 'b', 'X'}}, []ByteRun{{1, 1}}, 6, 3, false, false, 1},
		{"run", []byte("aaaaa"), []byte("abbba"), 0,
			[]ByteDiff{{1, 'a', 'b'}, {2, 'a', 'b'}, {3, 'a', 'b'}}, []ByteRun{{1, 3}}, 5, 5, false, false, 1},
		{"truncated", []byte("aaaaa"), []byte("bbbbb"), 2,
			[]ByteDiff{{0, 'a', 'b'}, {1, 'a', 'b'}}, []ByteRun{{0, 2}}, 5, 5, true, false, 0},
		{"unlimited", []byte("aaa"), []byte("bbb"), -1,
			[]ByteDiff{{0, 'a', 'b'}, {1, 'a', 'b'}, {2, 'a', 'b'}}, []ByteRun{{0, 3}}, 3, 3, false, false, 0},
		{"chunk boundary", big, bigMod, 0,
			[]ByteDiff{{int64(binChunk - 1), 7, 8}, {int64(binChunk), 7, 9}, {int64(len(big) - 1), 7, 1}},
			[]ByteRun{{int64(binChunk - 1), 2}, {int64(len(big) - 1), 1}},
			int64(len(big)), int64(len(big)), false, false, int64(binChunk - 1)},
		{"long tail", []byte("x"), append([]byte("x"), big...), 0, nil, nil, 1, int64(len(big) + 1), false, false, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := CompareBinary(context.Background(), bytes.NewReader(tc.a), bytes.NewReader(tc.b), tc.max)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(res.Diffs, tc.diffs) || !reflect.DeepEqual(res.Runs, tc.runs) {
				t.Fatalf("diffs %+v runs %+v", res.Diffs, res.Runs)
			}
			if res.SizeA != tc.sizeA || res.SizeB != tc.sizeB || res.Truncated != tc.truncated {
				t.Fatalf("sizes %d/%d truncated %v", res.SizeA, res.SizeB, res.Truncated)
			}
			if res.Identical() != tc.identical || res.FirstDifference() != tc.first {
				t.Fatalf("identical %v first %d", res.Identical(), res.FirstDifference())
			}
			eq, err := ReadersEqual(context.Background(), bytes.NewReader(tc.a), bytes.NewReader(tc.b))
			if err != nil || eq != tc.identical {
				t.Fatalf("ReadersEqual = %v, %v", eq, err)
			}
		})
	}
}

func TestCompareBinaryCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CompareBinary(ctx, bytes.NewReader([]byte("a")), bytes.NewReader([]byte("b")), 0); err == nil {
		t.Fatal("expected context error")
	}
}

func TestFilesEqual(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("a", bytes.Repeat([]byte("hello"), 50000))
	b := write("b", bytes.Repeat([]byte("hello"), 50000))
	c := write("c", append(bytes.Repeat([]byte("hello"), 49999), []byte("hellO")...))
	d := write("d", []byte("short"))
	ctx := context.Background()
	for _, tc := range []struct {
		x, y string
		want bool
	}{{a, b, true}, {a, a, true}, {a, c, false}, {a, d, false}} {
		got, err := FilesEqual(ctx, tc.x, tc.y)
		if err != nil || got != tc.want {
			t.Errorf("FilesEqual(%s,%s) = %v,%v want %v", filepath.Base(tc.x), filepath.Base(tc.y), got, err, tc.want)
		}
	}
	if _, err := FilesEqual(ctx, a, filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file must fail")
	}
	res, err := CompareBinaryFiles(ctx, a, c, 0)
	if err != nil || len(res.Diffs) != 1 || res.Diffs[0].Offset != 5*50000-1 {
		t.Fatalf("CompareBinaryFiles: %+v %v", res, err)
	}
	res, err = CompareBinaryFiles(ctx, a, d, 1)
	if err != nil || !res.Truncated || res.SizeA != 250000 || res.SizeB != 5 {
		t.Fatalf("truncated sizes from stat: %+v %v", res, err)
	}
}
