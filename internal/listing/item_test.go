// Tests for Item helpers and reading a real folder from a temp dir.
package listing

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSplitExt(t *testing.T) {
	tests := []struct{ in, base, ext string }{
		{"a.txt", "a", "txt"},
		{"archive.tar.gz", "archive.tar", "gz"},
		{".bashrc", ".bashrc", ""},
		{".config.json", ".config", "json"},
		{"noext", "noext", ""},
		{"trailing.", "trailing.", ""},
		{"..", "..", ""},
	}
	for _, tt := range tests {
		b, e := SplitExt(tt.in)
		if b != tt.base || e != tt.ext {
			t.Errorf("SplitExt(%q) = %q,%q want %q,%q", tt.in, b, e, tt.base, tt.ext)
		}
	}
	d := Item{Name: "x.d", Attr: AttrDir}
	if b, e := SplitExtFor(&d, false); b != "x.d" || e != "" {
		t.Errorf("dir whole: %q %q", b, e)
	}
	if !IsWindowsExecName("SETUP.EXE") || IsWindowsExecName("readme.txt") {
		t.Error("IsWindowsExecName")
	}
}

func TestReadDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "ro.txt"), nil, 0o444)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	linksOK := runtime.GOOS != "windows"
	if linksOK {
		os.Symlink("sub", filepath.Join(dir, "tosub"))
		os.Symlink("nowhere", filepath.Join(dir, "broken"))
		os.WriteFile(filepath.Join(dir, "run.sh"), nil, 0o755)
	}
	items, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Item{}
	for i, it := range items {
		if it.Order != i {
			t.Errorf("Order of %s = %d want %d", it.Name, it.Order, i)
		}
		by[it.Name] = it
	}
	if f := by["f.txt"]; f.Size != 5 || f.IsDir() || f.ModTime.IsZero() || f.Dir != dir || f.DirSize != -1 {
		t.Errorf("f.txt: %+v", f)
	}
	if s := by["sub"]; !s.IsDir() || s.Size != 0 {
		t.Errorf("sub: %+v", s)
	}
	if r := by["ro.txt"]; !r.Attr.Has(AttrReadOnly) {
		t.Errorf("ro.txt not read-only: %v", r.Attr)
	}
	if linksOK {
		if h := by[".hidden"]; !h.Attr.Has(AttrHidden) {
			t.Error(".hidden not hidden")
		}
		l := by["tosub"]
		if !l.Attr.Has(AttrLink|AttrDir) || l.LinkTarget != "sub" || l.Attr.Has(AttrBroken) {
			t.Errorf("tosub: %+v", l)
		}
		if b := by["broken"]; !b.Attr.Has(AttrLink | AttrBroken) {
			t.Errorf("broken: %+v", b)
		}
		if x := by["run.sh"]; !x.Attr.Has(AttrExec) {
			t.Error("run.sh not exec")
		}
		if by["f.txt"].Owner == "" {
			t.Error("owner empty")
		}
	}
	if runtime.GOOS == "darwin" && by["f.txt"].CreateTime.IsZero() {
		t.Error("darwin birth time missing")
	}
	if _, err := ReadDir(filepath.Join(dir, "nope")); err == nil {
		t.Error("missing dir should fail")
	}
}
