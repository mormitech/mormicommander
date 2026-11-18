package main

// Editing a file on a server (F-593): F4 downloads the file to a temporary copy, each save of the copy asks (Remote
// set) and the answer uploads it. The server is an in-memory file system registered as "editmem://", so the test needs
// no network.

import (
	"context"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"mormicommander/internal/vfs"
)

type editMemFS struct {
	mu    sync.Mutex
	files map[string]string
}

func (m *editMemFS) List(_ context.Context, p string) (vfs.Listing, error) {
	return vfs.Listing{Path: m.Clean(p)}, nil
}
func (m *editMemFS) Stat(p string) (vfs.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p = m.Clean(p)
	if s, ok := m.files[p]; ok {
		return vfs.Entry{Name: path.Base(p), Size: int64(len(s)), MTime: 1700000000000, Mode: 0o644}, nil
	}
	if p == "editmem://h/pub" {
		return vfs.Entry{Name: "pub", Flags: vfs.FlagDir, Mode: uint32(fs.ModeDir | 0o755)}, nil
	}
	return vfs.Entry{}, fs.ErrNotExist
}
func (*editMemFS) Clean(p string) string { return strings.TrimRight(p, "/") }
func (*editMemFS) Parent(p string) string {
	if i := strings.LastIndex(p, "/"); i > len("editmem://") {
		return p[:i]
	}
	return ""
}
func (*editMemFS) Root(string) string           { return "editmem://h" }
func (*editMemFS) Join(dir, name string) string { return strings.TrimRight(dir, "/") + "/" + name }
func (m *editMemFS) Download(_ context.Context, p, local string, _ vfs.TransferOptions) (int64, error) {
	m.mu.Lock()
	s, ok := m.files[m.Clean(p)]
	m.mu.Unlock()
	if !ok {
		return 0, fs.ErrNotExist
	}
	return int64(len(s)), os.WriteFile(local, []byte(s), 0o644)
}
func (m *editMemFS) Upload(_ context.Context, local, p string, _ vfs.TransferOptions) (int64, error) {
	b, err := os.ReadFile(local)
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	m.files[m.Clean(p)] = string(b)
	m.mu.Unlock()
	return int64(len(b)), nil
}

var editMem = &editMemFS{files: map[string]string{"editmem://h/pub/readme.txt": "hello"}}

func init() { vfs.Register("editmem", editMem) }

func TestServerEditDownloadAskUpload(t *testing.T) {
	r := arcEditRouter(t)
	file := "editmem://h/pub/readme.txt"
	if !vfsServerKind(r, file) || vfsServerKind(r, t.TempDir()) || vfsServerKind(nil, file) {
		t.Fatal("vfsServerKind: wrong kind")
	}
	if _, _, err := serverEditPrepare(r, "editmem://h/pub"); err == nil || !strings.Contains(err.Error(), "folder") {
		t.Fatalf("a folder was taken for editing: %v", err)
	}
	if _, _, err := serverEditPrepare(r, "editmem://h/pub/none.txt"); err == nil {
		t.Fatal("a missing file was taken for editing")
	}
	local, folder, err := serverEditPrepare(r, file)
	if err != nil {
		t.Fatal(err)
	}
	if folder != "editmem://h/pub" {
		t.Fatalf("folder = %q", folder)
	}
	if b, _ := os.ReadFile(local); string(b) != "hello" {
		t.Fatalf("copy has %q", b)
	}
	var asked []arcEditChange
	w := newArcEditWatcher(func(l, e string) error {
		_, err := r.Upload(context.Background(), l, r.Clean(e), vfs.TransferOptions{})
		return err
	}, func(c arcEditChange) { asked = append(asked, c) })
	id, err := w.addRemote(file, folder, local)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.scan(); len(got) != 0 {
		t.Fatalf("asked before any save: %+v", got)
	}
	arcEditSave(t, local, "hello, edited", time.Now().Add(time.Hour))
	got := w.scan()
	if len(got) != 1 || !got[0].Remote || got[0].Archive != folder || got[0].Entry != file || got[0].Name != "readme.txt" || len(asked) != 1 {
		t.Fatalf("scan after save: %+v", got)
	}
	// "Don't update": the server keeps its file; the next save asks again
	if err := w.answer(id, false); err != nil {
		t.Fatal(err)
	}
	if s, _ := editMem.Stat(file); s.Size != 5 {
		t.Fatal("declined change was uploaded")
	}
	arcEditSave(t, local, "third", time.Now().Add(2*time.Hour))
	if got := w.scan(); len(got) != 1 {
		t.Fatalf("second save not asked: %+v", got)
	}
	if err := w.answer(id, true); err != nil {
		t.Fatal(err)
	}
	editMem.mu.Lock()
	s := editMem.files[file]
	editMem.mu.Unlock()
	if s != "third" {
		t.Fatalf("server has %q after Update", s)
	}
	// an upload that fails names the server folder
	w2 := newArcEditWatcher(func(string, string) error { return fs.ErrPermission }, nil)
	id2, _ := w2.addRemote(file, folder, local)
	arcEditSave(t, local, "fourth", time.Now().Add(3*time.Hour))
	w2.scan()
	if err := w2.answer(id2, true); err == nil || !strings.Contains(err.Error(), "upload") || !strings.Contains(err.Error(), folder) {
		t.Fatalf("failed upload: %v", err)
	}
}
