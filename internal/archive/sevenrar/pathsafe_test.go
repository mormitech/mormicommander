// These tests pin down the entry-name cleaning and the Windows name rules,
// the lexical containment check, the "no writing through links" folder
// creation and the rules for which stored symbolic links may be created.

package sevenrar

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCleanEntryName(t *testing.T) {
	cases := []struct {
		in     string
		clean  string
		unsafe bool
	}{
		{"a/b/c.txt", "a/b/c.txt", false},
		{"a\\b\\c.txt", "a/b/c.txt", false},
		{"/etc/passwd", "etc/passwd", false},
		{"//server/share/x", "server/share/x", false},
		{`\\server\share\x`, "server/share/x", false},
		{"C:\\Windows\\x", "Windows/x", false},
		{"c:x", "x", false},
		{"./a/./b/", "a/b", false},
		{"dir/", "dir", false},
		{"../x", "../x", true},
		{"a/../../x", "a/../../x", true},
		{"a/..", "a/..", true},
		{"..\\x", "../x", true},
		{"a\x00b", "a_b", true},
		{"/", "", false},
		{"", "", false},
		{"...", "...", false},
		{"a..b/c", "a..b/c", false},
	}
	for _, c := range cases {
		clean, unsafe := cleanEntryName(c.in)
		if clean != c.clean || unsafe != c.unsafe {
			t.Errorf("cleanEntryName(%q) = %q,%v want %q,%v", c.in, clean, unsafe, c.clean, c.unsafe)
		}
	}
}

func TestLegalComponentWindows(t *testing.T) {
	cases := map[string]string{
		"normal.txt":    "normal.txt",
		"a:b?c*d.txt":   "a_b_c_d.txt",
		`quo"te<>|`:     "quo_te___",
		"trailing. . ":  "trailing",
		"CON":           "_CON",
		"con.txt":       "_con.txt",
		"LPT1.log":      "_LPT1.log",
		"CONSOLE":       "CONSOLE",
		"...":           "_",
		"tab\there":     "tab_here",
		"árvíztűrő.txt": "árvíztűrő.txt",
	}
	for in, want := range cases {
		if got := legalComponent(in, true); got != want {
			t.Errorf("legalComponent(%q) = %q, want %q", in, got, want)
		}
		if got := legalComponent(in, false); got != in {
			t.Errorf("legalComponent(%q, unix) = %q", in, got)
		}
	}
	if got := localPath("a/b:c/CON", true); got != `a\b_c\_CON` {
		t.Errorf("localPath = %q", got)
	}
}

func TestTargetPath(t *testing.T) {
	dest := t.TempDir()
	if p, err := targetPath(dest, "a/b.txt"); err != nil || p != filepath.Join(dest, "a", "b.txt") {
		t.Errorf("targetPath = %q, %v", p, err)
	}
	for _, bad := range []string{"", "../x", "a/../../x"} {
		if _, err := targetPath(dest, bad); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("targetPath(%q) err = %v", bad, err)
		}
	}
}

func TestWithin(t *testing.T) {
	sep := string(filepath.Separator)
	cases := []struct {
		dest, p string
		want    bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b", "/a/b/c", true},
		{"/a/b", "/a/bc", false},
		{"/a/b", "/a", false},
		{"/a/b", "/a/b/../c", false},
		{"/a/b", "/a/b/..c", true},
	}
	for _, c := range cases {
		d, p := filepath.FromSlash(c.dest), filepath.FromSlash(c.p)
		if got := within(d, p); got != c.want {
			t.Errorf("within(%s, %s) = %v, want %v (sep %s)", d, p, got, c.want, sep)
		}
	}
}

func TestMkdirNoFollow(t *testing.T) {
	dest := t.TempDir()
	if err := mkdirNoFollow(dest, filepath.Join(dest, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(dest, "a", "b", "c")); err != nil || !st.IsDir() {
		t.Fatalf("not created: %v", err)
	}
	if err := mkdirNoFollow(dest, filepath.Join(dest, "..", "x"), 0o755); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("outside: %v", err)
	}
	writeFile(t, filepath.Join(dest, "file"), []byte("x"))
	if err := mkdirNoFollow(dest, filepath.Join(dest, "file", "sub"), 0o755); err == nil {
		t.Error("through a file: no error")
	}
	if runtime.GOOS == "windows" {
		return
	}
	other := t.TempDir()
	os.Symlink(other, filepath.Join(dest, "link"))
	if err := mkdirNoFollow(dest, filepath.Join(dest, "link", "sub"), 0o755); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("through link: %v", err)
	}
	if _, err := os.Stat(filepath.Join(other, "sub")); err == nil {
		t.Error("created through link")
	}
	// dest itself may be a link chosen by the user.
	linkedDest := filepath.Join(t.TempDir(), "dl")
	os.Symlink(other, linkedDest)
	if err := mkdirNoFollow(linkedDest, filepath.Join(linkedDest, "ok"), 0o755); err != nil {
		t.Errorf("linked dest: %v", err)
	}
}

func TestSymlinkTargetOK(t *testing.T) {
	dest := t.TempDir()
	os.MkdirAll(filepath.Join(dest, "d", "e"), 0o755)
	cases := []struct {
		link, target string
		want         bool
	}{
		{"l", "file", true},
		{"l", "d/e/file", true},
		{"d/l", "../file", true},
		{"d/e/l", "../../file", true},
		{"d/e/l", "../../../file", false},
		{"l", "../file", false},
		{"l", "/etc/passwd", false},
		{"l", `C:\Windows`, false},
		{"l", "d/../file", false},
		{"d/l", "x/../../..", false},
		{"l", ".", true},
		{"l", "", false},
		{"l", "a\x00b", false},
		{"d/l", `..\file`, true},
	}
	for _, c := range cases {
		if got := symlinkTargetOK(dest, c.link, c.target, nil); got != c.want {
			t.Errorf("symlinkTargetOK(%s -> %s) = %v, want %v", c.link, c.target, got, c.want)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	os.Symlink(t.TempDir(), filepath.Join(dest, "foreign"))
	if symlinkTargetOK(dest, "l", "foreign/x", nil) {
		t.Error("through foreign link allowed")
	}
	if !symlinkTargetOK(dest, "l", "foreign/x", map[string]bool{"foreign": true}) {
		t.Error("through our own link refused")
	}
}
