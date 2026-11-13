// Tests for the command-line history (dedupe, cap, Ctrl+E/Ctrl+X stepping)
// and path completion against a real temporary folder.
package params

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestHistory(t *testing.T) {
	h := NewHistory(3, []string{"c", "b", "a"})
	if !reflect.DeepEqual(h.Items(), []string{"c", "b", "a"}) {
		t.Fatalf("load: %q", h.Items())
	}
	h.Add("  b ")
	h.Add("")
	if !reflect.DeepEqual(h.Items(), []string{"b", "c", "a"}) {
		t.Fatalf("dedupe: %q", h.Items())
	}
	h.Add("d")
	if !reflect.DeepEqual(h.Items(), []string{"d", "b", "c"}) {
		t.Fatalf("cap: %q", h.Items())
	}
	if _, ok := h.Newer(); ok {
		t.Error("Newer before browsing")
	}
	steps := []string{}
	for {
		s, ok := h.Older("draft")
		if !ok {
			break
		}
		steps = append(steps, s)
	}
	if !reflect.DeepEqual(steps, []string{"d", "b", "c"}) {
		t.Errorf("older: %q", steps)
	}
	if s, _ := h.Newer(); s != "b" {
		t.Error(s)
	}
	h.Newer()
	if s, ok := h.Newer(); !ok || s != "draft" {
		t.Errorf("draft restore: %q %v", s, ok)
	}
	h.Older("x")
	h.ResetBrowse()
	if s, _ := h.Older("y"); s != "d" {
		t.Error("reset")
	}
	if NewHistory(0, nil).max != 50 {
		t.Error("default max")
	}
}

func TestComplete(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"Docs", "docker", "My Dir", ".hidden"} {
		os.Mkdir(filepath.Join(dir, d), 0o755)
	}
	os.WriteFile(filepath.Join(dir, "doc.txt"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "My Dir", "inner.go"), nil, 0o644)
	// Creating symlinks on Windows needs a privilege CI may not have.
	cdWant := []string{"cd docker/", "cd Docs/"}
	if os.Symlink(filepath.Join(dir, "Docs"), filepath.Join(dir, "dlink")) == nil {
		cdWant = append([]string{"cd dlink/"}, cdWant...)
	}
	o := CompleteOptions{Cwd: dir, Home: dir, Style: StylePOSIX}
	lines := func(cs []Completion) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Line)
		}
		return out
	}
	cases := []struct {
		line string
		want []string
	}{
		{"vi do", []string{"vi doc.txt", "vi docker/", "vi Docs/"}},
		{"cd d", cdWant},
		{"vi my", []string{"vi 'My Dir/'"}},
		{"vi 'My Dir/in", []string{"vi 'My Dir/inner.go'"}},
		{`vi My\ Dir/`, []string{"vi 'My Dir/inner.go'"}},
		{"ls ~/my", []string{"ls ~/'My Dir/'"}},
		{"ls .h", []string{"ls .hidden/"}},
		{"ls zzz", nil},
	}
	// A POSIX-style absolute path only exists where the temp folder is one;
	// a Windows "C:\..." folder cannot be typed in POSIX syntax.
	if runtime.GOOS != "windows" {
		cases = append(cases, struct {
			line string
			want []string
		}{"ls " + dir + "/Docs", []string{"ls " + Quote(dir+"/Docs/", StylePOSIX)}})
	}
	for _, c := range cases {
		got, err := Complete(c.line, o)
		if err != nil {
			t.Fatalf("%q: %v", c.line, err)
		}
		if !reflect.DeepEqual(lines(got), c.want) {
			t.Errorf("Complete(%q) = %q, want %q", c.line, lines(got), c.want)
		}
	}
	o.Max = 1
	if got, _ := Complete("x d", o); len(got) != 1 {
		t.Errorf("max: %d", len(got))
	}
	if _, err := Complete("x nosuch/a", o); err == nil {
		t.Error("missing folder should fail")
	}
}
