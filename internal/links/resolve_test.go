// Tests for following link chains: multi-hop chains, relative links through a
// linked folder, broken chains, two-link and self loops, and the
// "points to its own ancestor" check used by recursive operations.

package links

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveChain(t *testing.T) {
	dir := realTemp(t)
	target := filepath.Join(dir, "real", "file.txt")
	writeFile(t, target, "x")
	l1 := filepath.Join(dir, "l1")
	l2 := filepath.Join(dir, "l2")
	symlinkOrSkip(t, target, l1)
	symlinkOrSkip(t, "l1", l2) // relative hop

	res, err := Resolve(l2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Final != target || !res.Exists || res.IsDir {
		t.Errorf("res = %+v", res)
	}
	want := []string{l2, l1, target}
	if len(res.Chain) != len(want) {
		t.Fatalf("chain = %v, want %v", res.Chain, want)
	}
	for i := range want {
		if res.Chain[i] != want[i] {
			t.Errorf("chain[%d] = %q, want %q", i, res.Chain[i], want[i])
		}
	}
}

func TestResolveRelativeThroughLinkedFolder(t *testing.T) {
	dir := realTemp(t)
	// deep/a/link -> ../b/file ; alias -> deep/a ; resolving alias/link must
	// interpret "../b" relative to deep/a, not relative to dir.
	writeFile(t, filepath.Join(dir, "deep", "b", "file"), "f")
	if err := os.MkdirAll(filepath.Join(dir, "deep", "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, filepath.Join("..", "b", "file"), filepath.Join(dir, "deep", "a", "link"))
	symlinkOrSkip(t, filepath.Join(dir, "deep", "a"), filepath.Join(dir, "alias"))
	res, err := Resolve(filepath.Join(dir, "alias", "link"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "deep", "b", "file"); res.Final != want {
		t.Errorf("Final = %q, want %q", res.Final, want)
	}
}

func TestResolveNotLink(t *testing.T) {
	dir := realTemp(t)
	f := filepath.Join(dir, "plain")
	writeFile(t, f, "")
	res, err := Resolve(f)
	if err != nil || res.Final != f || !res.Exists || len(res.Chain) != 1 {
		t.Errorf("res = %+v, err = %v", res, err)
	}
	if _, err := Resolve(filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrBroken) {
		t.Errorf("missing path: err = %v, want plain ErrNotExist", err)
	}
	if _, err := Resolve(filepath.Join(dir, "nodir", "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing parent: err = %v", err)
	}
}

func TestResolveBrokenAndLoops(t *testing.T) {
	dir := realTemp(t)
	broken := filepath.Join(dir, "broken")
	symlinkOrSkip(t, filepath.Join(dir, "gone"), broken)
	brokenParent := filepath.Join(dir, "brokenparent")
	symlinkOrSkip(t, filepath.Join(dir, "nodir", "x"), brokenParent)
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	symlinkOrSkip(t, "b", a)
	symlinkOrSkip(t, "a", b)
	self := filepath.Join(dir, "self")
	symlinkOrSkip(t, "self", self)
	viaLoop := filepath.Join(dir, "vialoop")
	symlinkOrSkip(t, filepath.Join(a, "child"), viaLoop) // parent of the target loops

	tests := []struct {
		path string
		want error
	}{
		{broken, ErrBroken},
		{brokenParent, ErrBroken},
		{a, ErrLoop},
		{self, ErrLoop},
		{viaLoop, ErrLoop},
	}
	for _, tc := range tests {
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			res, err := Resolve(tc.path)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if res == nil {
				t.Fatal("nil resolution")
			}
			if tc.want == ErrBroken && (res.Exists || res.Final == "") {
				t.Errorf("broken res = %+v", res)
			}
			if tc.want == ErrLoop {
				var le *LoopError
				if !errors.As(err, &le) || le.Error() == "" {
					t.Errorf("not a *LoopError: %T", err)
				}
				if !HasLoop(tc.path) {
					t.Error("HasLoop = false")
				}
			}
		})
	}
	if HasLoop(broken) {
		t.Error("broken link reported as loop")
	}
}

func TestPointsToAncestor(t *testing.T) {
	dir := realTemp(t)
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "other", "f"), "")
	up := filepath.Join(sub, "up")
	symlinkOrSkip(t, "..", up)
	root := filepath.Join(sub, "root")
	symlinkOrSkip(t, dir, root)
	side := filepath.Join(sub, "side")
	symlinkOrSkip(t, filepath.Join(dir, "other"), side)
	here := filepath.Join(sub, "here")
	symlinkOrSkip(t, ".", here)
	tests := []struct {
		path string
		want bool
	}{
		{up, true}, {root, true}, {here, true}, {side, false}, {filepath.Join(dir, "other"), false},
	}
	for _, tc := range tests {
		if got := PointsToAncestor(tc.path); got != tc.want {
			t.Errorf("PointsToAncestor(%s) = %v, want %v", filepath.Base(tc.path), got, tc.want)
		}
	}
}

func TestIdentityAndIsLink(t *testing.T) {
	dir := realTemp(t)
	f := filepath.Join(dir, "f")
	writeFile(t, f, "")
	l := filepath.Join(dir, "l")
	symlinkOrSkip(t, f, l)
	if !IsLink(l) || IsLink(f) || IsLink(filepath.Join(dir, "missing")) {
		t.Error("IsLink")
	}
	a, ok1 := IdentityOf(f)
	b, ok2 := IdentityOf(l)
	if ok1 && ok2 && a != b {
		t.Error("link and target have different identities")
	}
	if _, ok := IdentityOf(filepath.Join(dir, "missing")); ok {
		t.Error("identity of missing file")
	}
}

func TestCleanNTPrefix(t *testing.T) {
	tests := map[string]string{
		`\??\C:\x`:          `C:\x`,
		`\??\UNC\srv\share`: `\\srv\share`,
		`relative`:          `relative`,
	}
	for in, want := range tests {
		if got := cleanNTPrefix(in); got != want {
			t.Errorf("cleanNTPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
