// Tests for the format-neutral helpers: path cleaning (the zip-slip guard),
// the folder index, Total Commander masks, conflict answers, code pages and
// the generic extraction, testing, fs.FS and source-collection code. A small
// in-memory Archive stands in for real formats so malicious names can be
// tested without crafting binary files.
package archive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// memArchive is a minimal Archive over in-memory records.
type memArchive struct {
	ix   *Index
	data map[int][]byte
	bad  map[int]bool // entries whose content fails the checksum
}

func newMem(files map[string]string, extra ...Entry) *memArchive {
	var recs []Entry
	data := map[int][]byte{}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	mt := time.Date(2020, 5, 6, 7, 8, 10, 0, time.Local)
	for _, n := range names {
		id := len(recs)
		isDir := strings.HasSuffix(n, "/")
		recs = append(recs, Entry{ID: id, RawName: n, IsDir: isDir, Size: int64(len(files[n])), ModTime: mt, Mode: 0o644})
		data[id] = []byte(files[n])
	}
	for _, e := range extra {
		e.ID = len(recs)
		data[e.ID] = []byte(e.Comment)
		e.Size = int64(len(e.Comment))
		e.Comment = ""
		recs = append(recs, e)
	}
	return &memArchive{ix: NewIndex(recs), data: data, bad: map[int]bool{}}
}

func (m *memArchive) Format() string                    { return "mem" }
func (m *memArchive) Path() string                      { return "mem.zip" }
func (m *memArchive) Comment() string                   { return "" }
func (m *memArchive) Entries() []Entry                  { return m.ix.Entries() }
func (m *memArchive) ReadDir(d string) ([]Entry, error) { return m.ix.ReadDir(d) }
func (m *memArchive) Stat(n string) (Entry, error)      { return m.ix.Stat(n) }
func (m *memArchive) Close() error                      { return nil }
func (m *memArchive) Open(e Entry) (io.ReadCloser, error) {
	if m.bad[e.ID] {
		return io.NopCloser(io.MultiReader(bytes.NewReader(m.data[e.ID]), errReader{ErrChecksum})), nil
	}
	return io.NopCloser(bytes.NewReader(m.data[e.ID])), nil
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestCleanPath(t *testing.T) {
	tests := []struct {
		raw      string
		want     string
		insecure bool
	}{
		{"a/b/c.txt", "a/b/c.txt", false},
		{"a\\b\\c.txt", "a/b/c.txt", false},
		{"./a/./b/", "a/b", false},
		{"/etc/passwd", "etc/passwd", true},
		{"../../evil", "evil", true},
		{"a/../../b", "b", true},
		{"a/b/../c", "a/c", true},
		{"C:\\Windows\\x.dll", "Windows/x.dll", true},
		{"c:x", "x", true},
		{"//server/share/f", "server/share/f", true},
		{"", "", false},
		{"..", "", true},
		{"árvíztűrő/tükör.txt", "árvíztűrő/tükör.txt", false},
	}
	for _, tt := range tests {
		got, ins := CleanPath(tt.raw)
		if got != tt.want || ins != tt.insecure {
			t.Errorf("CleanPath(%q) = %q,%v want %q,%v", tt.raw, got, ins, tt.want, tt.insecure)
		}
	}
}

func TestIndex(t *testing.T) {
	m := newMem(map[string]string{
		"a/b/c.txt": "c", "a/d.txt": "d", "top.txt": "t", "empty/": "", "../x.txt": "x",
	})
	root, err := m.ReadDir("")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range root {
		names = append(names, e.Name)
	}
	if got := strings.Join(names, ","); got != "a,empty,top.txt,x.txt" {
		t.Fatalf("root = %s", got)
	}
	a, _ := m.Stat("a")
	if !a.IsDir || !a.Implied || a.ID != -1 {
		t.Errorf("a should be implied folder: %+v", a)
	}
	e, _ := m.Stat("empty")
	if !e.IsDir || e.Implied {
		t.Errorf("empty should be explicit folder: %+v", e)
	}
	x, _ := m.Stat("x.txt")
	if !x.Insecure || x.RawName != "../x.txt" {
		t.Errorf("x.txt should be flagged insecure: %+v", x)
	}
	if _, err := m.Stat("nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(nope) err = %v", err)
	}
	if _, err := m.ReadDir("top.txt"); err == nil {
		t.Error("ReadDir on a file should fail")
	}
	kids, _ := m.ReadDir("a/")
	if len(kids) != 2 || kids[0].Name != "b" || kids[1].Name != "d.txt" {
		t.Errorf("ReadDir(a) = %+v", kids)
	}
	if n := len(m.ix.Under("a")); n != 4 { // a, a/b, a/b/c.txt, a/d.txt
		t.Errorf("Under(a) = %d entries", n)
	}
	if !IsUnder("a/b", "a") || IsUnder("ab", "a") || !IsUnder("x", "") {
		t.Error("IsUnder wrong")
	}
}

func TestMask(t *testing.T) {
	tests := []struct {
		mask, name string
		want       bool
	}{
		{"", "anything", true},
		{"*.*", "noext", true},
		{"*.txt", "a.TXT", true},
		{"*.txt", "a.txt.bak", false},
		{"*.txt *.doc", "x.doc", true},
		{"*.txt;*.doc", "x.doc", true},
		{"*.txt | old*", "old.txt", false},
		{"*.txt | old*", "new.txt", true},
		{"| *.bak", "x.bak", false},
		{"| *.bak", "x.txt", true},
		{"a?c", "abc", true},
		{"a?c", "abbc", false},
		{"*b*", "abc", true},
		{"readme.*", "readme", true},
		{"readme.*", "readme.md", true},
		{"\"my file.txt\"", "my file.txt", true},
		{"*.tar.gz", "x.tar.gz", true},
		{"ár*", "ÁRVÍZ.txt", true},
	}
	for _, tt := range tests {
		if got := ParseMask(tt.mask).Match(tt.name); got != tt.want {
			t.Errorf("Mask(%q).Match(%q) = %v", tt.mask, tt.name, got)
		}
	}
}

func TestFreeName(t *testing.T) {
	taken := map[string]bool{"a.txt": true, "a (2).txt": true, ".profile": true}
	ex := func(n string) bool { return taken[n] }
	for in, want := range map[string]string{"a.txt": "a (3).txt", "b.txt": "b.txt", ".profile": ".profile (2)"} {
		if got := FreeName(in, ex); got != want {
			t.Errorf("FreeName(%q) = %q want %q", in, got, want)
		}
	}
}

func TestConflictResolver(t *testing.T) {
	older := ConflictInfo{Path: "x", Existing: FileStat{ModTime: time.Unix(100, 0)}, Incoming: FileStat{ModTime: time.Unix(200, 0)}}
	newer := ConflictInfo{Path: "x", Existing: FileStat{ModTime: time.Unix(300, 0)}, Incoming: FileStat{ModTime: time.Unix(200, 0)}}
	calls := 0
	ask := func(action ConflictAction) ConflictFunc {
		return func(ConflictInfo) ConflictDecision {
			calls++
			return ConflictDecision{Action: action, NewName: "n.txt"}
		}
	}
	tests := []struct {
		name   string
		policy ConflictPolicy
		fn     ConflictFunc
		info   ConflictInfo
		want   []Resolution // two consecutive conflicts
		calls  int
	}{
		{"no callback skips", PolicyAsk, nil, older, []Resolution{ResolveSkip, ResolveSkip}, 0},
		{"overwrite", PolicyOverwrite, nil, older, []Resolution{ResolveOverwrite, ResolveOverwrite}, 0},
		{"older only, older", PolicyOverwriteOlder, nil, older, []Resolution{ResolveOverwrite, ResolveOverwrite}, 0},
		{"older only, newer", PolicyOverwriteOlder, nil, newer, []Resolution{ResolveSkip, ResolveSkip}, 0},
		{"ask once for all", PolicyAsk, ask(ActOverwriteAll), older, []Resolution{ResolveOverwrite, ResolveOverwrite}, 1},
		{"ask each time", PolicyAsk, ask(ActSkip), older, []Resolution{ResolveSkip, ResolveSkip}, 2},
		{"skip all", PolicyAsk, ask(ActSkipAll), older, []Resolution{ResolveSkip, ResolveSkip}, 1},
		{"rename", PolicyAsk, ask(ActRename), older, []Resolution{ResolveRename, ResolveRename}, 2},
		{"cancel", PolicyAsk, ask(ActCancel), older, []Resolution{ResolveCancel, ResolveCancel}, 2},
		{"auto rename all", PolicyAsk, ask(ActAutoRenameAll), older, []Resolution{ResolveRename, ResolveRename}, 1},
	}
	for _, tt := range tests {
		calls = 0
		c := NewConflictResolver(tt.policy, tt.fn)
		for i, w := range tt.want {
			if got, _ := c.Resolve(tt.info, func(string) bool { return false }); got != w {
				t.Errorf("%s: call %d = %v want %v", tt.name, i, got, w)
			}
		}
		if calls != tt.calls {
			t.Errorf("%s: dialog shown %d times, want %d", tt.name, calls, tt.calls)
		}
	}
	// Rename with an illegal name is treated as skip.
	c := NewConflictResolver(PolicyAsk, func(ConflictInfo) ConflictDecision {
		return ConflictDecision{Action: ActRename, NewName: "../x"}
	})
	if r, _ := c.Resolve(older, nil); r != ResolveSkip {
		t.Errorf("rename to ../x = %v", r)
	}
}

func TestSafeJoin(t *testing.T) {
	dest := t.TempDir()
	ok := []string{"a.txt", "a/b.txt", "../a.txt", "/abs/x", "C:\\x\\y"}
	for _, rel := range ok {
		p, err := SafeJoin(dest, rel)
		if err != nil {
			t.Errorf("SafeJoin(%q) err %v", rel, err)
			continue
		}
		if !strings.HasPrefix(p, dest+string(filepath.Separator)) {
			t.Errorf("SafeJoin(%q) = %q escapes", rel, p)
		}
	}
	for _, rel := range []string{"", "..", "/", "./."} {
		if _, err := SafeJoin(dest, rel); !errors.Is(err, ErrInsecurePath) {
			t.Errorf("SafeJoin(%q) err = %v, want ErrInsecurePath", rel, err)
		}
	}
}

func TestLinkStaysInside(t *testing.T) {
	tests := []struct {
		rel, target string
		want        bool
	}{
		{"a/link", "b.txt", true},
		{"a/link", "../b.txt", true},
		{"a/link", "../../b.txt", false},
		{"link", "/etc/passwd", false},
		{"link", "C:\\Windows", false},
		{"a/b/link", "../../x/./y", true},
		{"link", "", false},
		// ".." after a name can climb through a link made earlier.
		{"y", "x/up/..", false},
		{"a/l", "b/../c", false},
		{"a/l", "../a/../b", false},
	}
	for _, tt := range tests {
		if got := linkStaysInside(tt.rel, tt.target); got != tt.want {
			t.Errorf("linkStaysInside(%q,%q) = %v", tt.rel, tt.target, got)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	if runtime.GOOS == "windows" {
		for in, want := range map[string]string{"a:b?.txt": "a_b_.txt", "CON": "_CON", "con.txt": "_con.txt", "x. ": "x", "ok": "ok"} {
			if got := SanitizeName(in); got != want {
				t.Errorf("SanitizeName(%q) = %q want %q", in, got, want)
			}
		}
		return
	}
	if got := SanitizeName("a:b?\x00"); got != "a:b?_" {
		t.Errorf("SanitizeName = %q", got)
	}
}

func TestCodepages(t *testing.T) {
	// "árvíztűrő" in CP852 and in Windows-1250.
	cp852 := []byte{0xa0, 'r', 'v', 0xa1, 'z', 't', 0xfb, 'r', 0x8b}
	w1250 := []byte{0xe1, 'r', 'v', 0xed, 'z', 't', 0xfb, 'r', 0xf5}
	if got := NewNameDecoder("cp852", nil)(cp852); got != "árvíztűrő" {
		t.Errorf("cp852 = %q", got)
	}
	if got := NewNameDecoder("ibm852", nil)(cp852); got != "árvíztűrő" {
		t.Errorf("ibm852 alias = %q", got)
	}
	if got := NewNameDecoder("cp1250", nil)(w1250); got != "árvíztűrő" {
		t.Errorf("cp1250 = %q", got)
	}
	if got := NewNameDecoder("cp437", nil)([]byte{0x81}); got != "ü" {
		t.Errorf("cp437 = %q", got)
	}
	if got := NewNameDecoder("utf-8", nil)([]byte("é")); got != "é" {
		t.Errorf("utf8 = %q", got)
	}
	// Auto: valid UTF-8 samples mean UTF-8.
	if got := NewNameDecoder("auto", [][]byte{[]byte("é")})([]byte("é")); got != "é" {
		t.Errorf("auto utf8 = %q", got)
	}
	// Auto with invalid UTF-8 uses the locale's OEM page.
	if runtime.GOOS != "windows" {
		t.Setenv("LC_ALL", "hu_HU.UTF-8")
		if got := NewNameDecoder("", [][]byte{cp852})(cp852); got != "árvíztűrő" {
			t.Errorf("auto hu = %q", got)
		}
	}
	if _, ok := LookupEncoding("klingon"); ok {
		t.Error("unknown encoding accepted")
	}
	if len(Encodings()) < 20 || Encodings()[0] != "utf-8" {
		t.Errorf("Encodings = %v", Encodings())
	}
}

func TestOEMForLanguage(t *testing.T) {
	for in, want := range map[string]string{
		"hu_HU.UTF-8": "cp852", "ru_RU": "cp866", "de_DE@euro": "cp850", "en_US.UTF-8": "cp437",
		"ja_JP": "shift_jis", "zh_TW": "big5", "zh_CN.GB18030": "gbk", "C": "cp437", "nb-NO": "cp865",
	} {
		if got := oemForLanguage(in); got != want {
			t.Errorf("oemForLanguage(%q) = %q want %q", in, got, want)
		}
	}
	for cp, want := range map[uint32]string{852: "cp852", 1250: "windows-1250", 932: "shift_jis", 12345: "cp437"} {
		if got := oemForCodePage(cp); got != want {
			t.Errorf("oemForCodePage(%d) = %q", cp, got)
		}
	}
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			tgt, _ := os.Readlink(p)
			out[rel] = "-> " + filepath.ToSlash(tgt)
		case d.IsDir():
			out[rel+"/"] = ""
		default:
			b, _ := os.ReadFile(p)
			out[rel] = string(b)
		}
		return nil
	})
	return out
}

func TestExtractSelectionAndOptions(t *testing.T) {
	m := newMem(map[string]string{
		"docs/a/x.txt": "x", "docs/a/y.doc": "y", "docs/b.txt": "b", "other.txt": "o", "docs/empty/": "",
	})
	tests := []struct {
		name string
		opts ExtractOptions
		want map[string]string
	}{
		{"all", ExtractOptions{}, map[string]string{
			"docs/": "", "docs/a/": "", "docs/a/x.txt": "x", "docs/a/y.doc": "y", "docs/b.txt": "b", "docs/empty/": "", "other.txt": "o"}},
		{"selected folder relative to base", ExtractOptions{Base: "docs", Paths: []string{"docs/a"}}, map[string]string{
			"a/": "", "a/x.txt": "x", "a/y.doc": "y"}},
		{"base only", ExtractOptions{Base: "docs/a"}, map[string]string{"x.txt": "x", "y.doc": "y"}},
		{"no paths", ExtractOptions{NoPaths: true}, map[string]string{"x.txt": "x", "y.doc": "y", "b.txt": "b", "other.txt": "o"}},
		{"mask", ExtractOptions{Mask: "*.txt | b*"}, map[string]string{"docs/": "", "docs/a/": "", "docs/a/x.txt": "x", "other.txt": "o"}},
	}
	for _, tt := range tests {
		dest := t.TempDir()
		tt.opts.Dest = dest
		res, err := Extract(context.Background(), m, tt.opts)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		got := readTree(t, dest)
		if fmt.Sprint(got) != fmt.Sprint(tt.want) {
			t.Errorf("%s:\n got %v\nwant %v", tt.name, got, tt.want)
		}
		if res.Files != len(res.Written) {
			t.Errorf("%s: Files=%d Written=%d", tt.name, res.Files, len(res.Written))
		}
	}
}

func TestExtractZipSlip(t *testing.T) {
	m := newMem(map[string]string{
		"../../evil.txt": "e", "/abs.txt": "a", "C:\\win.txt": "w", "ok/fine.txt": "f",
	}, Entry{RawName: "ok/escape", Mode: fs.ModeSymlink | 0o777, Comment: "../../../etc"},
		Entry{RawName: "ok/inside", Mode: fs.ModeSymlink | 0o777, Comment: "fine.txt"},
		Entry{RawName: "abslink", Mode: fs.ModeSymlink | 0o777, Comment: "/etc/passwd"})
	parent := t.TempDir()
	dest := filepath.Join(parent, "dest")
	res, err := Extract(context.Background(), m, ExtractOptions{Dest: dest})
	if err != nil {
		t.Fatal(err)
	}
	// Nothing may appear outside dest.
	ents, _ := os.ReadDir(parent)
	if len(ents) != 1 {
		t.Fatalf("files escaped: %v", ents)
	}
	got := readTree(t, dest)
	for _, want := range []string{"evil.txt", "abs.txt", "win.txt", "ok/fine.txt"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing sanitized %s in %v", want, got)
		}
	}
	if _, ok := got["ok/escape"]; ok {
		t.Error("escaping link was created")
	}
	if _, ok := got["abslink"]; ok {
		t.Error("absolute link was created")
	}
	skippedLinks := 0
	for _, s := range res.Skipped {
		if strings.Contains(s.Reason, "outside") {
			skippedLinks++
		}
	}
	if skippedLinks != 2 {
		t.Errorf("skipped = %+v", res.Skipped)
	}
	if runtime.GOOS != "windows" && got["ok/inside"] != "-> fine.txt" {
		t.Errorf("inside link = %q", got["ok/inside"])
	}
}

// TestExtractChainedLinkEscape is a regression test: a link whose target
// climbs through a link made earlier in the same run ("y" -> "x/up/.."
// with "x/up" -> "..") must not be created, nor one stored below such a
// link whose ".." then counts from the wrong folder.
func TestExtractChainedLinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	m := newMem(map[string]string{"x/keep.txt": "k"},
		Entry{RawName: "x/up", Mode: fs.ModeSymlink | 0o777, Comment: ".."},
		Entry{RawName: "y", Mode: fs.ModeSymlink | 0o777, Comment: "x/up/.."},
		Entry{RawName: "x/up/z", Mode: fs.ModeSymlink | 0o777, Comment: ".."})
	parent := t.TempDir()
	dest := filepath.Join(parent, "dest")
	if _, err := Extract(context.Background(), m, ExtractOptions{Dest: dest}); err != nil {
		t.Fatal(err)
	}
	realDest, _ := filepath.EvalSymlinks(dest)
	err := filepath.WalkDir(dest, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.Type()&fs.ModeSymlink == 0 {
			return err
		}
		r, err := filepath.EvalSymlinks(p)
		if err != nil {
			return nil
		}
		if rel, _ := filepath.Rel(realDest, r); rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Errorf("link %s resolves outside dest: %s", p, r)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "y")); err == nil {
		t.Error("chained escaping link y was created")
	}
}

func TestExtractThroughPlantedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	outside := t.TempDir()
	dest := t.TempDir()
	// A previous extraction left a link to elsewhere; a new archive tries to
	// write through it.
	if err := os.Symlink(outside, filepath.Join(dest, "link")); err != nil {
		t.Fatal(err)
	}
	m := newMem(map[string]string{"link/pwned.txt": "x"})
	var failed []string
	_, err := Extract(context.Background(), m, ExtractOptions{Dest: dest, OnError: func(p string, err error) bool {
		if errors.Is(err, ErrInsecurePath) {
			failed = append(failed, p)
		}
		return true
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned.txt")); err == nil {
		t.Fatal("wrote through symlink")
	}
	if len(failed) != 1 {
		t.Errorf("failed = %v", failed)
	}
}

func TestExtractConflicts(t *testing.T) {
	m := newMem(map[string]string{"a.txt": "new", "b.txt": "newb"})
	setup := func() string {
		d := t.TempDir()
		os.WriteFile(filepath.Join(d, "a.txt"), []byte("old"), 0o644)
		os.WriteFile(filepath.Join(d, "b.txt"), []byte("oldb"), 0o444)
		return d
	}
	tests := []struct {
		name   string
		policy ConflictPolicy
		fn     ConflictFunc
		want   map[string]string
		err    error
	}{
		{"skip", PolicySkip, nil, map[string]string{"a.txt": "old", "b.txt": "oldb"}, nil},
		{"overwrite (also read-only)", PolicyOverwrite, nil, map[string]string{"a.txt": "new", "b.txt": "newb"}, nil},
		{"auto rename", PolicyAutoRename, nil, map[string]string{"a.txt": "old", "a (2).txt": "new", "b.txt": "oldb", "b (2).txt": "newb"}, nil},
		{"rename by user", PolicyAsk, func(ci ConflictInfo) ConflictDecision {
			return ConflictDecision{Action: ActRename, NewName: "c-" + filepath.Base(ci.Path)}
		}, map[string]string{"a.txt": "old", "b.txt": "oldb", "c-a.txt": "new", "c-b.txt": "newb"}, nil},
		{"cancel", PolicyAsk, func(ConflictInfo) ConflictDecision { return ConflictDecision{Action: ActCancel} },
			map[string]string{"a.txt": "old", "b.txt": "oldb"}, ErrCanceled},
	}
	for _, tt := range tests {
		d := setup()
		_, err := Extract(context.Background(), m, ExtractOptions{Dest: d, Policy: tt.policy, OnConflict: tt.fn})
		if !errors.Is(err, tt.err) {
			t.Errorf("%s: err = %v", tt.name, err)
		}
		got := readTree(t, d)
		if fmt.Sprint(got) != fmt.Sprint(tt.want) {
			t.Errorf("%s: got %v want %v", tt.name, got, tt.want)
		}
	}
}

func TestExtractErrorsAndCancel(t *testing.T) {
	m := newMem(map[string]string{"a.txt": "aaaa", "b.txt": "bbbb", "c.txt": "cccc"})
	m.bad[1] = true
	d := t.TempDir()
	_, err := Extract(context.Background(), m, ExtractOptions{Dest: d})
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("err = %v", err)
	}
	// No partial file and no temp file left behind.
	got := readTree(t, d)
	if _, ok := got["b.txt"]; ok || len(got) != 1 {
		t.Errorf("after failure: %v", got)
	}
	d = t.TempDir()
	res, err := Extract(context.Background(), m, ExtractOptions{Dest: d, OnError: func(string, error) bool { return true }})
	if err != nil || len(res.Failed) != 1 || res.Files != 2 {
		t.Errorf("continue on error: %v %+v", err, res)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Extract(ctx, m, ExtractOptions{Dest: t.TempDir()}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancel err = %v", err)
	}
}

func TestExtractMetadataAndProgress(t *testing.T) {
	mt := time.Date(2001, 2, 3, 4, 5, 6, 0, time.Local)
	m := &memArchive{ix: NewIndex([]Entry{
		{ID: 0, RawName: "ro.txt", Size: 3, ModTime: mt, Mode: 0o444, DOSAttrs: AttrReadOnly},
		{ID: 1, RawName: "dir/", IsDir: true, ModTime: mt, Mode: fs.ModeDir | 0o755},
		{ID: 2, RawName: "dir/x", Size: 1, ModTime: mt, Mode: 0o755},
	}), data: map[int][]byte{0: []byte("abc"), 2: []byte("x")}, bad: map[int]bool{}}
	d := t.TempDir()
	var last Progress
	calls := 0
	if _, err := Extract(context.Background(), m, ExtractOptions{Dest: d, Progress: func(p Progress) { last = p; calls++ }}); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(filepath.Join(d, "ro.txt"))
	if !st.ModTime().Equal(mt) {
		t.Errorf("mtime %v", st.ModTime())
	}
	if st.Mode().Perm()&0o200 != 0 {
		t.Errorf("read-only not applied: %v", st.Mode())
	}
	dst, _ := os.Stat(filepath.Join(d, "dir"))
	if !dst.ModTime().Equal(mt) {
		t.Errorf("dir mtime %v", dst.ModTime())
	}
	if runtime.GOOS != "windows" {
		xs, _ := os.Stat(filepath.Join(d, "dir", "x"))
		if xs.Mode().Perm() != 0o755 {
			t.Errorf("exec bit lost: %v", xs.Mode())
		}
	}
	if last.Done != 4 || last.Total != 4 || last.Files != 2 || last.TotalFiles != 2 || calls < 3 {
		t.Errorf("progress %+v calls %d", last, calls)
	}
	os.Chmod(filepath.Join(d, "ro.txt"), 0o644)
}

func TestVerify(t *testing.T) {
	m := newMem(map[string]string{"a": "1", "b": "22", "d/c": "333"})
	m.bad[1] = true
	rep, err := Verify(context.Background(), m, VerifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK() || rep.Files != 2 || len(rep.Failed) != 1 || rep.Failed[0].Path != "b" {
		t.Errorf("report %+v", rep)
	}
	rep, _ = Verify(context.Background(), m, VerifyOptions{Paths: []string{"d"}})
	if !rep.OK() || rep.Files != 1 {
		t.Errorf("subset report %+v", rep)
	}
}

func TestFS(t *testing.T) {
	m := newMem(map[string]string{"a/b.txt": "hello", "c.txt": "world", "e/": ""})
	if err := fstest.TestFS(FS(m), "a/b.txt", "c.txt", "e"); err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(FS(m), "a/b.txt")
	if err != nil || string(b) != "hello" {
		t.Errorf("ReadFile = %q %v", b, err)
	}
}

func TestCollectSourcesAndRemove(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	os.MkdirAll(filepath.Join(src, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(src, "skip.bak"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(src, "sub", "c.txt"), []byte("c"), 0o644)
	os.WriteFile(filepath.Join(src, "sub", "deep", "d.txt"), []byte("d"), 0o644)
	archivePath := filepath.Join(src, "out.zip")
	os.WriteFile(archivePath, []byte("PK"), 0o644)
	if runtime.GOOS != "windows" {
		os.Symlink("..", filepath.Join(src, "sub", "loop"))
	}
	paths := func(fs []SourceFile) string {
		var out []string
		for _, f := range fs {
			s := f.ArchivePath
			if f.IsDir {
				s += "/"
			}
			out = append(out, s)
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	tests := []struct {
		name string
		opts PackOptions
		want string
	}{
		{"default", PackOptions{ExcludeMask: "*.bak"}, "src/ src/a.txt src/sub/ src/sub/c.txt src/sub/deep/ src/sub/deep/d.txt"},
		{"no recurse", PackOptions{NoRecurse: true}, "src/ src/a.txt src/skip.bak"},
		{"flatten", PackOptions{FlattenPaths: true, ExcludeMask: "*.bak"}, "a.txt c.txt d.txt"},
	}
	for _, tt := range tests {
		got, err := CollectSources([]AddItem{{Source: src}}, tt.opts, archivePath)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if p := paths(got); p != tt.want {
			t.Errorf("%s:\n got %s\nwant %s", tt.name, p, tt.want)
		}
	}
	// Dest renames the top item; in-memory items need a Dest.
	got, _ := CollectSources([]AddItem{{Source: filepath.Join(src, "a.txt"), Dest: "x/y.txt"}, {Dest: "mem.txt", Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("m")), nil }}}, PackOptions{})
	if paths(got) != "mem.txt x/y.txt" {
		t.Errorf("dest: %s", paths(got))
	}
	if _, err := CollectSources([]AddItem{{Open: func() (io.ReadCloser, error) { return nil, nil }}}, PackOptions{}); err == nil {
		t.Error("in-memory item without Dest accepted")
	}
	// Move: everything packed goes, the excluded file and archive stay.
	files, _ := CollectSources([]AddItem{{Source: src}}, PackOptions{ExcludeMask: "*.bak", StoreSymlinks: true}, archivePath)
	if _, err := RemoveSources(files); err != nil {
		t.Fatal(err)
	}
	left := readTree(t, root)
	if _, ok := left["src/skip.bak"]; !ok {
		t.Errorf("excluded file removed: %v", left)
	}
	if _, ok := left["src/a.txt"]; ok {
		t.Errorf("packed file kept: %v", left)
	}
	if _, ok := left["src/out.zip"]; !ok {
		t.Errorf("archive removed: %v", left)
	}
}

func TestRegistry(t *testing.T) {
	Register(Format{Name: "fake", Extensions: []string{"fk", "tar.fk"}, Magic: func(h []byte) bool { return bytes.HasPrefix(h, []byte("FAKE")) }})
	defer func() {
		regMu.Lock()
		for i, f := range formats {
			if f.Name == "fake" {
				formats = append(formats[:i], formats[i+1:]...)
				break
			}
		}
		regMu.Unlock()
	}()
	if f, ok := ByExtension("X.TAR.FK"); !ok || f.Name != "fake" {
		t.Error("ByExtension failed")
	}
	if !IsArchiveName("a.fk") || IsArchiveName("a.txt") {
		t.Error("IsArchiveName wrong")
	}
	d := t.TempDir()
	p := filepath.Join(d, "noext")
	os.WriteFile(p, []byte("FAKE data"), 0o644)
	if f, ok := Detect(p); !ok || f.Name != "fake" {
		t.Error("Detect by magic failed")
	}
	if _, err := Open(p, OpenOptions{}); !errors.Is(err, ErrFormat) {
		t.Errorf("Open without opener: %v", err)
	}
	if _, ok := Lookup("FAKE"); !ok {
		t.Error("Lookup failed")
	}
}
