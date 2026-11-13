// Shared helpers for the syncdirs tests. Every test builds its two trees in
// fresh t.TempDir() folders from a small map of path -> spec, with fixed
// dates, so the results never depend on the machine's clock or contents.
package syncdirs

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// t0 is the base date of all test files.
var t0 = time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

// spec describes one test entry. A path ending in "/" is a folder.
type spec struct {
	data string
	at   time.Duration // offset from t0
	link string        // non-empty: create a symlink with this target
}

func file(data string, at time.Duration) spec { return spec{data: data, at: at} }

// build creates the entries under root.
func build(t *testing.T, root string, entries map[string]spec) {
	t.Helper()
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var dirs []string
	for _, k := range keys {
		s := entries[k]
		p := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(k, "/")))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		switch {
		case s.link != "":
			if err := os.Symlink(s.link, p); err != nil {
				t.Skipf("symlinks not available: %v", err)
			}
		case strings.HasSuffix(k, "/"):
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			dirs = append(dirs, p)
		default:
			if err := os.WriteFile(p, []byte(s.data), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(p, t0.Add(s.at), t0.Add(s.at)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, d := range dirs {
		_ = os.Chtimes(d, t0, t0)
	}
}

// trees makes two temp folders filled with the given entries.
func trees(t *testing.T, left, right map[string]spec) (string, string) {
	t.Helper()
	l, r := filepath.Join(t.TempDir(), "L"), filepath.Join(t.TempDir(), "R")
	if err := os.Mkdir(l, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(r, 0o755); err != nil {
		t.Fatal(err)
	}
	build(t, l, left)
	build(t, r, right)
	return l, r
}

func mustCompare(t *testing.T, l, r string, o Options) *Result {
	t.Helper()
	res, err := Compare(context.Background(), DirFS(l), DirFS(r), o, nil)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	return res
}

// rows renders the result as "path category action" lines for easy
// comparison in table tests.
func rows(res *Result) []string {
	var out []string
	for _, it := range res.Items {
		s := it.Path + " " + it.Category.String() + " " + it.Action.String()
		if it.IsDir {
			s = it.Path + "/ " + it.Category.String() + " " + it.Action.String()
		}
		out = append(out, s)
	}
	return out
}

func equalLines(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
