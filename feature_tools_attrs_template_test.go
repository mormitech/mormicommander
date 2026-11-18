package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"mormicommander/internal/attrs"
	"mormicommander/internal/find"
)

// F-685: a saved search narrows a recursive attribute change to the files it finds.
func TestToolsTemplateFilter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix permission bits")
	}
	dir := t.TempDir()
	st, err := find.OpenStore(filepath.Join(dir, "searches.json"))
	if err != nil {
		t.Fatal(err)
	}
	o := find.DefaultOptions()
	o.SearchFor = "*.sh"
	o.Kind = find.KindFiles
	if err := st.Save("scripts", o, false); err != nil {
		t.Fatal(err)
	}
	if _, err := toolsTemplateFrom(st, "nope"); err == nil {
		t.Fatal("an unknown template must be an error")
	}
	q, err := toolsTemplateFrom(st, "scripts")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "tree")
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	for _, n := range []string{"a.sh", "b.txt", "sub/c.sh"} {
		os.WriteFile(filepath.Join(root, n), nil, 0o644)
	}
	req := attrs.Request{Paths: []string{root}, Recursive: true, Perm: attrs.PermChange{Set: attrs.FromUnix(0o100)},
		Filter: &attrs.Filter{Match: func(p string, _ attrs.Info) bool {
			ok, err := q.Match(context.Background(), p)
			return err == nil && ok
		}}}
	plan, err := attrs.MakePlan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	plan.Apply(context.Background(), nil)
	want := map[string]os.FileMode{"a.sh": 0o744, "b.txt": 0o644, "sub/c.sh": 0o744}
	for n, m := range want {
		fi, _ := os.Stat(filepath.Join(root, n))
		if fi.Mode().Perm() != m {
			t.Errorf("%s: mode %o, want %o", n, fi.Mode().Perm(), m)
		}
	}
}
