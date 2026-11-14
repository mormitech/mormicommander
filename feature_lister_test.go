package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mormicommander/internal/launch"
	"mormicommander/internal/vfs"
)

func newTestLister() *Lister {
	return &Lister{app: &App{fs: vfs.Local{}}, sessions: map[int]*listerSession{}, sizes: map[string]context.CancelFunc{}}
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestListerOpenMoveFind(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("x", i%7))
		if i == 150 {
			b.WriteString(" árvíztűrő needle")
		}
		b.WriteString("\n")
	}
	p := writeTemp(t, "a.txt", b.String())
	l := newTestLister()
	info, err := l.Open(p, ListerOptions{Mode: "text", Wrap: true, Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info.ID)
	if info.Suggest != "text" || info.Encoding != "utf-8" {
		t.Fatalf("suggest %q, encoding %q", info.Suggest, info.Encoding)
	}
	page, err := l.Move(info.ID, 0, "top", 0, 10)
	if err != nil || len(page.Rows) != 10 || page.Rows[0].T != "line x" || page.Line != 1 {
		t.Fatalf("first page: %+v %v", page, err)
	}
	down, _ := l.Move(info.ID, page.Top, "rows", 3, 10)
	if down.Line != 4 {
		t.Fatalf("after 3 rows down the top line is %d", down.Line)
	}
	end, _ := l.Move(info.ID, 0, "end", 0, 10)
	if !end.AtEnd || len(end.Rows) != 10 || end.Percent != 100 {
		t.Fatalf("end page: atEnd %v rows %d percent %v", end.AtEnd, len(end.Rows), end.Percent)
	}
	m, err := l.Find(info.ID, ListerQuery{Pattern: "ÁRVÍZTŰRŐ"}, 0)
	if err != nil || !m.Found {
		t.Fatalf("find: %+v %v", m, err)
	}
	txt, _ := l.Text(info.ID, m.Offset, m.Offset+m.Length, false)
	if txt != "árvíztűrő" {
		t.Fatalf("found text %q", txt)
	}
	if m2, _ := l.Find(info.ID, ListerQuery{Pattern: "needle"}, m.Offset+m.Length+20); m2.Found {
		t.Fatalf("found past the only match: %+v", m2)
	}
	back, _ := l.Find(info.ID, ListerQuery{Pattern: "needle", Backward: true}, int64(len(b.String())))
	if !back.Found {
		t.Fatal("backward search found nothing")
	}
	rev, _ := l.Move(info.ID, 0, "reveal", float64(back.Offset), 10)
	if rev.Rows[0].O > back.Offset || rev.Rows[len(rev.Rows)-1].E <= back.Offset {
		t.Fatalf("reveal did not bring offset %d onto the page (%d..%d)", back.Offset, rev.Rows[0].O, rev.Rows[len(rev.Rows)-1].E)
	}
	off, err := l.LineOffset(info.ID, 150)
	if err != nil {
		t.Fatal(err)
	}
	pg, _ := l.Move(info.ID, 0, "offset", float64(off), 5)
	if !strings.HasSuffix(pg.Rows[0].T, "needle") {
		t.Fatalf("line 150 is %q", pg.Rows[0].T)
	}
	if _, err := l.LineOffset(info.ID, 999); err == nil || !strings.Contains(err.Error(), "only 200 lines") {
		t.Fatalf("line past the end: %v", err)
	}
	if _, err := l.Find(info.ID, ListerQuery{Pattern: "(", Regex: true}, 0); err == nil {
		t.Fatal("a broken regular expression was accepted")
	}
}

func TestListerModesAndSave(t *testing.T) {
	p := writeTemp(t, "b.bin", "Hello\x00\x01\x02 World, this is binary data that goes on for a while.")
	l := newTestLister()
	info, err := l.Open(p, ListerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info.ID)
	if !info.Binary {
		t.Fatal("binary data not seen as binary")
	}
	if _, err := l.SetOptions(info.ID, ListerOptions{Mode: "hex"}); err != nil {
		t.Fatal(err)
	}
	page, _ := l.Move(info.ID, 0, "home", 0, 5)
	if !strings.HasPrefix(page.Rows[0].H, "48 65 6C 6C 6F 00") || page.Rows[0].E-page.Rows[0].O != 16 {
		t.Fatalf("hex row: %+v", page.Rows[0])
	}
	if h, _ := l.Text(info.ID, 0, 5, true); h != "48 65 6C 6C 6F" {
		t.Fatalf("copy as hex %q", h)
	}
	if m, _ := l.Find(info.ID, ListerQuery{Pattern: "00 01 02", Hex: true}, 0); !m.Found || m.Offset != 5 {
		t.Fatalf("hex search: %+v", m)
	}
	dest := filepath.Join(filepath.Dir(p), "part.bin")
	if n, err := l.SaveRange(info.ID, 0, 5, dest, false); err != nil || n != 5 {
		t.Fatalf("save: %d %v", n, err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "Hello" {
		t.Fatalf("saved %q", got)
	}
	if _, err := l.SaveRange(info.ID, 0, 3, dest, false); err == nil {
		t.Fatal("an existing file was overwritten without permission")
	}
	if _, err := l.SaveRange(info.ID, 0, 3, p, true); err == nil {
		t.Fatal("the file was saved over itself")
	}
	if _, err := l.SetOptions(info.ID, ListerOptions{Encoding: "no-such"}); err == nil {
		t.Fatal("an unknown encoding was accepted")
	}
}

func TestListerHTMLAsTextAndUTF16(t *testing.T) {
	p := writeTemp(t, "c.html", "<html><head><title>T</title><script>evil()</script></head><body><p>Hello <b>there</b></p></body></html>")
	l := newTestLister()
	info, err := l.Open(p, ListerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info.ID)
	if info.Suggest != "html" {
		t.Fatalf("suggest %q", info.Suggest)
	}
	got, err := l.SetOptions(info.ID, ListerOptions{Mode: "text", AsText: "html"})
	if err != nil || got.Options.AsText != "html" {
		t.Fatalf("as text: %+v %v", got, err)
	}
	page, _ := l.Move(info.ID, 0, "home", 0, 20)
	var all []string
	for _, r := range page.Rows {
		all = append(all, r.T)
	}
	if s := strings.Join(all, "\n"); !strings.Contains(s, "Hello there") || strings.Contains(s, "evil") {
		t.Fatalf("HTML as text: %q", s)
	}
	doc, _ := l.Document(info.ID)
	if !strings.Contains(doc, "<b>there</b>") {
		t.Fatalf("document %q", doc)
	}

	u := writeTemp(t, "u.txt", "\xff\xfeH\x00\xe9\x00\n\x00")
	info2, err := l.Open(u, ListerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info2.ID)
	if info2.Encoding != "utf-16le" {
		t.Fatalf("UTF-16 file detected as %q", info2.Encoding)
	}
	page2, _ := l.Move(info2.ID, 0, "home", 0, 3)
	if page2.Rows[0].T != "Hé" {
		t.Fatalf("UTF-16 row %q", page2.Rows[0].T)
	}
}

func TestListerErrorsAndReload(t *testing.T) {
	l := newTestLister()
	dir := t.TempDir()
	if _, err := l.Open(filepath.Join(dir, "none.txt"), ListerOptions{}); err == nil || !strings.Contains(err.Error(), "is not there") {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := l.Open(dir, ListerOptions{}); err == nil || !strings.Contains(err.Error(), "is a folder") {
		t.Fatalf("folder: %v", err)
	}
	p := writeTemp(t, "log.txt", "one\n")
	info, err := l.Open(p, ListerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("two\n")
	f.Close()
	c, err := l.Reload(info.ID)
	if err != nil || !c.Grown || c.Size != 8 {
		t.Fatalf("reload: %+v %v", c, err)
	}
	page, _ := l.Move(info.ID, 0, "end", 0, 5)
	if len(page.Rows) != 2 || page.Rows[1].T != "two" {
		t.Fatalf("after reload: %+v", page.Rows)
	}
	if l.Sessions() != 1 {
		t.Fatalf("sessions %d", l.Sessions())
	}
	l.Close(info.ID)
	if l.Sessions() != 0 {
		t.Fatal("closing did not end the session")
	}
	if _, err := l.Move(info.ID, 0, "top", 0, 5); err == nil {
		t.Fatal("a closed session still answers")
	}
	r, err := l.FolderSize("t", filepath.Dir(p))
	if err != nil || r.Files != 1 || r.Bytes != 8 {
		t.Fatalf("folder size: %+v %v", r, err)
	}
}

func TestListerDemoRefuses(t *testing.T) {
	a := &App{mock: vfs.NewMock()}
	l := &Lister{app: a, sessions: map[int]*listerSession{}, sizes: map[string]context.CancelFunc{}}
	if _, err := l.Open("/x", ListerOptions{}); err == nil || !strings.Contains(err.Error(), "demo") {
		t.Fatalf("demo open: %v", err)
	}
	e := &ListerEdit{app: a}
	if _, err := e.Edit("/x", "", launch.ParamContext{}); err == nil || !strings.Contains(err.Error(), "demo") {
		t.Fatalf("demo edit: %v", err)
	}
	e2 := &ListerEdit{app: &App{}}
	if err := e2.check("ftp://host/x"); err == nil {
		t.Fatal("a remote file was accepted for editing")
	}
}
