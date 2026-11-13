// These tests check the text produced for TC's copy-names commands (names, full paths, tab-separated details with
// "<DIR>" for folders, forward-slash and URL variants, folder paths with a trailing separator) and the parsing of
// clipboard text back into names for "load selection from clipboard" and the multi-rename tool.

package clipfiles

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFormatText(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix paths")
	}
	when := time.Date(2026, 11, 13, 9, 5, 7, 0, time.UTC)
	entries := []Entry{
		{Path: "/home/u/a.txt", Size: 1234, ModTime: when, Mode: 0o644},
		{Path: "/home/u/sub dir", IsDir: true, ModTime: when, Mode: os.ModeDir | 0o755},
		{Path: "/home/u/x", Name: "shown", Size: 0, Attr: "custom"},
	}
	lf := Format{Newline: "\n"}
	tests := []struct {
		kind TextKind
		f    Format
		want string
	}{
		{Names, lf, "a.txt\nsub dir\nshown"},
		{Names, Format{Newline: "\r\n", DirSuffix: true, TrailingNewline: true}, "a.txt\r\nsub dir/\r\nshown\r\n"},
		{FullNames, lf, "/home/u/a.txt\n/home/u/sub dir\n/home/u/x"},
		{NetNames, lf, "/home/u/a.txt\n/home/u/sub dir\n/home/u/x"},
		{UnixPaths, Format{Newline: "\n", DirSuffix: true}, "/home/u/a.txt\n/home/u/sub dir/\n/home/u/x"},
		{URLs, lf, "file:///home/u/a.txt\nfile:///home/u/sub%20dir\nfile:///home/u/x"},
		{Details, lf, "a.txt\t1234\t2026-11-13 09:05:07\t-rw-r--r--\n" +
			"sub dir\t<DIR>\t2026-11-13 09:05:07\tdrwxr-xr-x\n" +
			"shown\t0\t\tcustom"},
		{FullDetails, Format{Newline: "\n", DateLayout: "02.01.2006 15:04", DirSizeText: "Folder"},
			"/home/u/a.txt\t1234\t13.11.2026 09:05\t-rw-r--r--\n" +
				"/home/u/sub dir\tFolder\t13.11.2026 09:05\tdrwxr-xr-x\n" +
				"/home/u/x\t0\t\tcustom"},
	}
	for _, tt := range tests {
		if got := FormatText(tt.kind, entries, tt.f); got != tt.want {
			t.Errorf("kind %d:\n got %q\nwant %q", tt.kind, got, tt.want)
		}
	}
	if got := FormatText(Names, nil, lf); got != "" {
		t.Errorf("no entries gave %q", got)
	}
	nl := "\n"
	if runtime.GOOS == "windows" {
		nl = "\r\n"
	}
	if got := FormatText(Names, entries[:2], Format{}); got != "a.txt"+nl+"sub dir" {
		t.Errorf("default newline: %q", got)
	}
}

func TestCommandKind(t *testing.T) {
	for cm, want := range map[string]TextKind{
		"cm_CopyNamesToClip": Names, "cm_CopyFullNamesToClip": FullNames, "cm_CopyDetailsToClip": Details,
		"cm_CopyFullDetailsToClip": FullDetails, "cm_CopyNetNamesToClip": NetNames,
	} {
		if got, ok := CommandKind(cm); !ok || got != want {
			t.Errorf("CommandKind(%s) = %v, %v", cm, got, ok)
		}
	}
	if _, ok := CommandKind("cm_Nope"); ok {
		t.Error("unknown command accepted")
	}
}

func TestStatPaths(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	es := StatPaths([]string{f, sub, filepath.Join(dir, "missing")})
	if es[0].Size != 5 || es[0].IsDir || es[0].ModTime.IsZero() || es[0].Attr == "" {
		t.Errorf("file entry: %+v", es[0])
	}
	if !es[1].IsDir {
		t.Errorf("folder entry: %+v", es[1])
	}
	if es[2].Path == "" || !es[2].ModTime.IsZero() {
		t.Errorf("missing entry: %+v", es[2])
	}
	line := FormatText(Details, es[:1], Format{})
	if !strings.HasPrefix(line, "f.txt\t5\t") {
		t.Errorf("details line %q", line)
	}
}

func TestFolderPath(t *testing.T) {
	sep := string(filepath.Separator)
	root := sep
	if runtime.GOOS == "windows" {
		root = `C:\`
	}
	dir := filepath.Join(root, "a", "b")
	tests := []struct {
		in    string
		trail bool
		want  string
	}{
		{dir, false, dir},
		{dir, true, dir + sep},
		{dir + sep, false, dir},
		{root, true, root},
		{root, false, root},
	}
	for _, tt := range tests {
		if got := FolderPath(tt.in, tt.trail); got != tt.want {
			t.Errorf("FolderPath(%q, %v) = %q, want %q", tt.in, tt.trail, got, tt.want)
		}
	}
}

func TestLines(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"\n", []string{""}},
		{"a\nb", []string{"a", "b"}},
		{"a\r\n\r\nb\r\n", []string{"a", "", "b"}},
		{"a\rb", []string{"a", "b"}},
	}
	for _, tt := range tests {
		if got := Lines(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Lines(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseNameList(t *testing.T) {
	in := "a.txt\r\n\r\n  \"quoted name.txt\"  \nfile:///tmp/x%20y\nb.txt\t123\t2026-01-01\t-a--\n\"\"\n"
	want := []string{"a.txt", "quoted name.txt", filepath.FromSlash("/tmp/x y"), "b.txt"}
	if got := ParseNameList(in); !reflect.DeepEqual(got, want) {
		t.Errorf("ParseNameList = %q, want %q", got, want)
	}
}

func TestSelectionNames(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	list := []string{"a.txt", filepath.Join(dir, "b.txt"), filepath.Join(other, "c.txt"), "sub" + string(filepath.Separator), ""}
	want := []string{"a.txt", "b.txt", "sub"}
	if got := SelectionNames(list, dir); !reflect.DeepEqual(got, want) {
		t.Errorf("SelectionNames = %q, want %q", got, want)
	}
}
