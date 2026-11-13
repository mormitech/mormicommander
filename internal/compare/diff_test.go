// Tests for the line diff: hand-written cases for the options (case,
// whitespace, repeated lines, resync) and randomized checks that every op
// list is gap-free, that equal ops really pair equal lines, that the ops turn
// A into B, and that Myers finds a longest common subsequence (compared with
// a textbook dynamic-programming LCS on small inputs).
package compare

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// checkOps validates structural invariants of ops for inputs a, b.
func checkOps(t *testing.T, a, b []string, ops []Op, opts Options) {
	t.Helper()
	ai, bi := 0, 0
	for k, op := range ops {
		if op.A1 != ai || op.B1 != bi {
			t.Fatalf("op %d %+v has a gap (expected start %d,%d)", k, op, ai, bi)
		}
		if op.A2 < op.A1 || op.B2 < op.B1 {
			t.Fatalf("op %d %+v has negative length", k, op)
		}
		switch op.Kind {
		case OpEqual:
			if !opts.IgnoreRepeatedLines {
				if op.A2-op.A1 != op.B2-op.B1 {
					t.Fatalf("equal op %d %+v with unequal lengths", k, op)
				}
				for x := 0; x < op.A2-op.A1; x++ {
					if NormalizeLine(a[op.A1+x], opts) != NormalizeLine(b[op.B1+x], opts) {
						t.Fatalf("equal op %d pairs %q with %q", k, a[op.A1+x], b[op.B1+x])
					}
				}
			}
		case OpDelete:
			if op.B1 != op.B2 || op.A1 == op.A2 {
				t.Fatalf("bad delete %+v", op)
			}
		case OpInsert:
			if op.A1 != op.A2 || op.B1 == op.B2 {
				t.Fatalf("bad insert %+v", op)
			}
		case OpChange:
			if op.A1 == op.A2 || op.B1 == op.B2 {
				t.Fatalf("bad change %+v", op)
			}
		}
		if k > 0 && op.IsDiff() && ops[k-1].IsDiff() {
			t.Fatalf("adjacent diff ops %d and %d", k-1, k)
		}
		ai, bi = op.A2, op.B2
	}
	if ai != len(a) || bi != len(b) {
		t.Fatalf("ops end at %d,%d; want %d,%d", ai, bi, len(a), len(b))
	}
}

func lcsLen(a, b []string) int {
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	return dp[0][0]
}

func equalCount(ops []Op) int {
	n := 0
	for _, op := range ops {
		if op.Kind == OpEqual {
			n += op.A2 - op.A1
		}
	}
	return n
}

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func TestDiffLinesBasic(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		opts Options
		want []Op
	}{
		{"both empty", "", "", Options{}, nil},
		{"identical", "a\nb", "a\nb", Options{}, []Op{{OpEqual, 0, 2, 0, 2}}},
		{"insert into empty", "", "x\ny", Options{}, []Op{{OpInsert, 0, 0, 0, 2}}},
		{"delete all", "x\ny", "", Options{}, []Op{{OpDelete, 0, 2, 0, 0}}},
		{"change middle", "a\nb\nc", "a\nX\nc", Options{},
			[]Op{{OpEqual, 0, 1, 0, 1}, {OpChange, 1, 2, 1, 2}, {OpEqual, 2, 3, 2, 3}}},
		{"insert middle", "a\nc", "a\nb\nc", Options{},
			[]Op{{OpEqual, 0, 1, 0, 1}, {OpInsert, 1, 1, 1, 2}, {OpEqual, 1, 2, 2, 3}}},
		{"delete end", "a\nb\nc", "a\nb", Options{},
			[]Op{{OpEqual, 0, 2, 0, 2}, {OpDelete, 2, 3, 2, 2}}},
		{"ignore case", "Hello\nWORLD", "hello\nworld", Options{IgnoreCase: true},
			[]Op{{OpEqual, 0, 2, 0, 2}}},
		{"case matters", "Hello", "hello", Options{}, []Op{{OpChange, 0, 1, 0, 1}}},
		{"hungarian case", "ŐRÜLT ÍRÓ", "őrült író", Options{IgnoreCase: true},
			[]Op{{OpEqual, 0, 1, 0, 1}}},
		{"ws change", "  a  b\t", "a b", Options{Whitespace: WhitespaceIgnoreChange},
			[]Op{{OpEqual, 0, 1, 0, 1}}},
		{"ws change not all", "ab", "a b", Options{Whitespace: WhitespaceIgnoreChange},
			[]Op{{OpChange, 0, 1, 0, 1}}},
		{"ws all", "a=b", "a = b", Options{Whitespace: WhitespaceIgnoreAll},
			[]Op{{OpEqual, 0, 1, 0, 1}}},
		{"ws exact", "a=b", "a = b", Options{}, []Op{{OpChange, 0, 1, 0, 1}}},
		{"repeated lines", "x\nr\nr\nr\ny", "x\nr\ny", Options{IgnoreRepeatedLines: true},
			[]Op{{OpEqual, 0, 1, 0, 1}, {OpEqual, 1, 4, 1, 2}, {OpEqual, 4, 5, 2, 3}}},
		{"repeated lines off", "x\nr\nr\ny", "x\nr\ny", Options{},
			[]Op{{OpEqual, 0, 2, 0, 2}, {OpDelete, 2, 3, 2, 2}, {OpEqual, 3, 4, 2, 3}}},
		{"repeated with change", "r\nr\na", "r\nb", Options{IgnoreRepeatedLines: true},
			[]Op{{OpEqual, 0, 2, 0, 1}, {OpChange, 2, 3, 1, 2}}},
		{"resync merges", "a\n1\nb\n2\nc", "a\nX\nb\nY\nc", Options{ResyncLines: 2},
			[]Op{{OpEqual, 0, 1, 0, 1}, {OpChange, 1, 4, 1, 4}, {OpEqual, 4, 5, 4, 5}}},
		{"resync 1 keeps", "a\n1\nb\n2\nc", "a\nX\nb\nY\nc", Options{ResyncLines: 1},
			[]Op{{OpEqual, 0, 1, 0, 1}, {OpChange, 1, 2, 1, 2}, {OpEqual, 2, 3, 2, 3}, {OpChange, 3, 4, 3, 4}, {OpEqual, 4, 5, 4, 5}}},
		{"resync long run kept", "1\na\nb\n2", "X\na\nb\nY", Options{ResyncLines: 2},
			[]Op{{OpChange, 0, 1, 0, 1}, {OpEqual, 1, 3, 1, 3}, {OpChange, 3, 4, 3, 4}}},
	}
	for _, tc := range tests {
		for _, alg := range []Algorithm{AlgorithmPatience, AlgorithmMyers} {
			t.Run(fmt.Sprintf("%s/alg%d", tc.name, alg), func(t *testing.T) {
				opts := tc.opts
				opts.Algorithm = alg
				a, b := lines(tc.a), lines(tc.b)
				got := DiffLines(a, b, opts)
				checkOps(t, a, b, got, opts)
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("got %+v\nwant %+v", got, tc.want)
				}
			})
		}
	}
}

func TestPatienceKeepsFunctionsTogether(t *testing.T) {
	// Classic example where Myers matches the braces of the wrong function.
	a := lines("void a() {\n  x();\n}\n\nvoid b() {\n  y();\n}")
	b := lines("void b() {\n  y();\n}\n\nvoid a() {\n  x();\n}\n\nvoid b() {\n  z();\n}")
	ops := DiffLines(a, b, Options{})
	checkOps(t, a, b, ops, Options{})
	// "void a() {" is unique in both and must be matched.
	matched := false
	for _, op := range ops {
		if op.Kind == OpEqual && op.A1 == 0 && op.B1 == 4 {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("patience did not anchor on 'void a()': %+v", ops)
	}
}

func randLines(r *rand.Rand, n, alphabet int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = string(rune('a' + r.Intn(alphabet)))
	}
	return out
}

func TestDiffRandomInvariants(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for iter := 0; iter < 2000; iter++ {
		a := randLines(r, r.Intn(30), 1+r.Intn(6))
		b := randLines(r, r.Intn(30), 1+r.Intn(6))
		for _, opts := range []Options{
			{Algorithm: AlgorithmMyers},
			{Algorithm: AlgorithmPatience},
			{Algorithm: AlgorithmMyers, ResyncLines: 3},
			{Algorithm: AlgorithmPatience, IgnoreRepeatedLines: true},
		} {
			ops := DiffLines(a, b, opts)
			checkOps(t, a, b, ops, opts)
			if opts.Algorithm == AlgorithmMyers && opts.ResyncLines <= 1 && !opts.IgnoreRepeatedLines {
				if got, want := equalCount(ops), lcsLen(a, b); got != want {
					t.Fatalf("myers not minimal: %d equal lines, LCS %d\na=%v\nb=%v", got, want, a, b)
				}
			}
		}
	}
}

func TestDiffCostLimitStillValid(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	a := randLines(r, 3000, 20)
	b := randLines(r, 3000, 20)
	for _, cost := range []int{1, 5, 50} {
		opts := Options{Algorithm: AlgorithmMyers, MaxCost: cost}
		ops := DiffLines(a, b, opts)
		checkOps(t, a, b, ops, opts)
	}
	// Unlimited must still be minimal on a smaller input.
	a2, b2 := a[:200], b[:200]
	ops := DiffLines(a2, b2, Options{Algorithm: AlgorithmMyers, MaxCost: -1})
	if got, want := equalCount(ops), lcsLen(a2, b2); got != want {
		t.Fatalf("unlimited not minimal: %d vs %d", got, want)
	}
}

func TestDiffLargeSimilarFiles(t *testing.T) {
	a := make([]string, 50000)
	for i := range a {
		a[i] = fmt.Sprintf("line %d", i)
	}
	b := append([]string(nil), a...)
	b[100] = "changed"
	b = append(b[:2000], b[2010:]...)
	b = append(b[:30000], append([]string{"new1", "new2"}, b[30000:]...)...)
	for _, alg := range []Algorithm{AlgorithmMyers, AlgorithmPatience} {
		ops := DiffLines(a, b, Options{Algorithm: alg})
		checkOps(t, a, b, ops, Options{})
		s := Summarize(ops)
		if s.Blocks != 3 || s.ChangedA != 1 || s.Deleted != 10 || s.Inserted != 2 {
			t.Fatalf("alg %d: unexpected stats %+v", alg, s)
		}
	}
}

func TestSummarize(t *testing.T) {
	ops := []Op{{OpEqual, 0, 2, 0, 2}, {OpChange, 2, 4, 2, 3}, {OpEqual, 4, 5, 3, 4}, {OpDelete, 5, 6, 4, 4}, {OpInsert, 6, 6, 4, 7}}
	got := Summarize(ops)
	want := Stats{Equal: 3, Deleted: 1, Inserted: 3, ChangedA: 2, ChangedB: 1, Blocks: 3}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
	if got.Identical() || !Summarize(nil).Identical() {
		t.Fatal("Identical wrong")
	}
	if OpChange.String() != "change" || OpKind(99).String() != "?" {
		t.Fatal("OpKind.String")
	}
}

func TestNormalizeLine(t *testing.T) {
	tests := []struct {
		in   string
		opts Options
		want string
	}{
		{"  A\t b  ", Options{}, "  A\t b  "},
		{"  A\t b  ", Options{Whitespace: WhitespaceIgnoreChange}, "A b"},
		{"  A\t b  ", Options{Whitespace: WhitespaceIgnoreAll}, "Ab"},
		{"  A\t b  ", Options{Whitespace: WhitespaceIgnoreAll, IgnoreCase: true}, "ab"},
		{"K", Options{IgnoreCase: true}, "k"}, // Kelvin sign
		{"ÁRVÍZTŰRŐ", Options{IgnoreCase: true}, "árvíztűrő"},
	}
	for _, tc := range tests {
		if got := NormalizeLine(tc.in, tc.opts); got != tc.want {
			t.Errorf("NormalizeLine(%q, %+v) = %q, want %q", tc.in, tc.opts, got, tc.want)
		}
	}
}
