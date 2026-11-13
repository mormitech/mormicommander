// Tests for hunk grouping and unified diff output. The unified output is also
// checked by applying it back with a tiny patch routine.
package compare

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func TestWriteUnified(t *testing.T) {
	a := lines("1\n2\n3\n4\n5\n6\n7\n8\n9\n10")
	b := lines("1\n2\nthree\n4\n5\n6\n7\n8\n9\n10\n11")
	ops := DiffLines(a, b, Options{})
	var buf bytes.Buffer
	if err := WriteUnified(&buf, "a.txt", "b.txt", a, b, ops, 2); err != nil {
		t.Fatal(err)
	}
	want := `--- a.txt
+++ b.txt
@@ -1,5 +1,5 @@
 1
 2
-3
+three
 4
 5
@@ -9,2 +9,3 @@
 9
 10
+11
`
	if buf.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", buf.String(), want)
	}
	// Large context merges the two hunks into one.
	h := Hunks(a, b, ops, 4)
	if len(h) != 1 || h[0].Header() != "@@ -1,10 +1,11 @@" {
		t.Fatalf("merged hunks: %d %v", len(h), h)
	}
	buf.Reset()
	if err := WriteUnified(&buf, "a", "b", a, a, DiffLines(a, a, Options{}), 3); err != nil || buf.Len() != 0 {
		t.Fatal("identical input must produce no output")
	}
}

func TestHunkHeaderEdgeCases(t *testing.T) {
	tests := []struct {
		a, b string
		want string
	}{
		{"", "x", "@@ -0,0 +1 @@"},
		{"x", "", "@@ -1 +0,0 @@"},
		{"a\nb", "a\nb\nc", "@@ -1,2 +1,3 @@"},
	}
	for _, tc := range tests {
		a, b := lines(tc.a), lines(tc.b)
		h := Hunks(a, b, DiffLines(a, b, Options{}), 3)
		if len(h) != 1 || h[0].Header() != tc.want {
			t.Errorf("%q->%q: got %v want %s", tc.a, tc.b, h, tc.want)
		}
	}
}

// applyHunks applies hunks to a, verifying context lines.
func applyHunks(a []string, hunks []Hunk) ([]string, error) {
	var out []string
	pos := 0
	for _, h := range hunks {
		if h.AStart < pos {
			return nil, fmt.Errorf("overlapping hunk at %d", h.AStart)
		}
		out = append(out, a[pos:h.AStart]...)
		pos = h.AStart
		for _, l := range h.Lines {
			switch l.Kind {
			case ' ':
				if a[pos] != l.Text {
					return nil, fmt.Errorf("context mismatch at %d", pos)
				}
				out = append(out, l.Text)
				pos++
			case '-':
				if a[pos] != l.Text {
					return nil, fmt.Errorf("delete mismatch at %d", pos)
				}
				pos++
			case '+':
				out = append(out, l.Text)
			}
		}
	}
	return append(out, a[pos:]...), nil
}

func TestHunksRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for iter := 0; iter < 1000; iter++ {
		a := randLines(r, r.Intn(40), 4)
		b := randLines(r, r.Intn(40), 4)
		ops := DiffLines(a, b, Options{})
		for _, ctx := range []int{0, 1, 3} {
			got, err := applyHunks(a, Hunks(a, b, ops, ctx))
			if err != nil {
				t.Fatalf("ctx %d: %v", ctx, err)
			}
			if strings.Join(got, "\n") != strings.Join(b, "\n") || len(got) != len(b) {
				t.Fatalf("ctx %d: patched %v want %v", ctx, got, b)
			}
		}
	}
}
