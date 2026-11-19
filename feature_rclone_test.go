package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mormicommander/internal/rclone/rclonetest"
	"mormicommander/internal/vfs"
)

// rcloneNet is a Net whose rclone is the fake (internal/rclone/rclonetest): remotes are folders, nothing goes online.
func rcloneNet(t *testing.T, remotes ...string) (*Net, rclonetest.Fake) {
	t.Helper()
	f := rclonetest.New(t, remotes...)
	n, _ := testNet(t, nil)
	n.rclone.program = func() string { return f.Bin }
	n.rclone.env = f.Env()
	return n, f
}

func TestRcloneURL(t *testing.T) {
	u, err := netParseURL("rclone://My Drive/Photos/../Docs")
	if err != nil || u.Host != "My Drive" || u.Path != "/Docs" || u.key() != "rclone://My Drive" || u.String() != "rclone://My Drive/Docs" {
		t.Fatalf("parse: %+v %v", u, err)
	}
	for _, bad := range []string{"rclone://:s3,env_auth=true:bucket/x", "rclone://-flag/x", "rclone:///x"} {
		if _, err := netParseURL(bad); err == nil {
			t.Errorf("%s should be refused", bad)
		}
	}
	if !netIsRemote("rclone://box/") || netKeyFor("rclone://box/a/b") != "rclone://box" {
		t.Error("rclone addresses are remote places of the network layer")
	}
	r := newNetRclone(nil)
	if r.Parent("rclone://box/a/b") != "rclone://box/a" || r.Parent("rclone://box/") != "" || r.Join("rclone://box/a", "c d") != "rclone://box/a/c d" || r.Root("rclone://box/a") != "rclone://box/" {
		t.Error("path helpers")
	}
}

// Nothing starts rclone but the user's own actions: listing the remotes or opening one. Calls on a remote that is not
// open fail without running it.
func TestRcloneNothingRunsUntilOpened(t *testing.T) {
	n, f := rcloneNet(t, "box")
	os.WriteFile(filepath.Join(f.Root, "box", "a.txt"), []byte("x"), 0o644)
	if _, err := n.rclone.Stat("rclone://box/a.txt"); err == nil {
		t.Error("Stat on a remote nobody opened must fail")
	}
	if _, err := vfs.OpenFile(t.Context(), n.rclone, "rclone://box/a.txt"); err == nil {
		t.Error("Open on a remote nobody opened must fail")
	}
	if err := n.rclone.RemoveAll(t.Context(), "rclone://box/a.txt"); err == nil {
		t.Error("delete on a remote nobody opened must fail")
	}
	if calls := f.Calls(); len(calls) != 0 {
		t.Fatalf("rclone ran by itself: %v", calls)
	}
	if len(n.Sessions()) != 0 {
		t.Fatal("no session before opening")
	}
	rs, err := n.RcloneRemotes()
	if err != nil || len(rs) != 1 || rs[0].Name != "box" || rs[0].URL != "rclone://box/" {
		t.Fatalf("remotes: %+v %v", rs, err)
	}
	if calls := f.Calls(); len(calls) != 1 || !strings.Contains(calls[0], "listremotes") {
		t.Fatalf("listing remotes runs listremotes only: %v", calls)
	}
}

func TestRcloneNotInstalled(t *testing.T) {
	n, _ := testNet(t, nil)
	n.rclone.program = func() string { return "" }
	t.Setenv("PATH", t.TempDir())
	if _, err := n.RcloneRemotes(); err == nil || !strings.Contains(err.Error(), "rclone was not found") {
		t.Fatalf("err = %v", err)
	}
	if _, err := n.Connect("rclone://box/"); err == nil || !strings.Contains(err.Error(), "rclone was not found") {
		t.Fatalf("connect err = %v", err)
	}
}

func TestRclonePanelAndTransfers(t *testing.T) {
	n, f := rcloneNet(t, "box", "other")
	os.MkdirAll(filepath.Join(f.Root, "box", "docs", "sub"), 0o755)
	os.WriteFile(filepath.Join(f.Root, "box", "docs", "a.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(f.Root, "box", "docs", "sub", "b.txt"), []byte("bee"), 0o644)

	if _, err := n.Connect("rclone://nobox/"); err == nil {
		t.Fatal("a remote that is not configured must fail to open")
	}
	res, err := n.Connect("rclone://box/docs/a.txt") // a file opens its folder
	if err != nil || res.Path != "rclone://box/docs" {
		t.Fatalf("connect: %+v %v", res, err)
	}
	before := len(f.Calls())
	l, err := n.rclone.List(t.Context(), res.Path)
	if err != nil || len(l.Entries) != 2 || l.Entries[0].Name != "a.txt" || l.Entries[0].Size != 5 || l.Entries[1].Flags&vfs.FlagDir == 0 || l.Parent != "rclone://box/" {
		t.Fatalf("list: %+v %v", l, err)
	}
	if len(f.Calls()) != before {
		t.Error("the panel's first listing after connecting reuses the connect listing")
	}
	if _, err := n.rclone.List(t.Context(), "rclone://box/docs/a.txt"); !errors.Is(err, vfs.ErrNotFolder) {
		t.Errorf("listing a file: %v", err)
	}
	if st := n.Status(res.Path); !st.Connected || st.Kind != "rclone" {
		t.Fatalf("status %+v", st)
	}
	if e, err := n.rclone.Stat("rclone://box/docs/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing stat: %+v %v", e, err)
	}

	// download a file and a folder
	local := t.TempDir()
	id, err := n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{res.Path + "/a.txt", res.Path + "/sub"}, Target: local})
	if err != nil {
		t.Fatal(err)
	}
	if p := waitJob(t, n, id, nil); p.State != "done" {
		t.Fatalf("download: %+v", p)
	}
	if got, _ := os.ReadFile(filepath.Join(local, "sub", "b.txt")); string(got) != "bee" {
		t.Fatal("folder not downloaded")
	}

	// upload a folder
	os.MkdirAll(filepath.Join(local, "up", "deep"), 0o755)
	os.WriteFile(filepath.Join(local, "up", "deep", "c.txt"), []byte("sea"), 0o644)
	id, _ = n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{filepath.Join(local, "up")}, Target: res.Path})
	if p := waitJob(t, n, id, nil); p.State != "done" {
		t.Fatalf("upload: %+v", p)
	}
	if got, _ := os.ReadFile(filepath.Join(f.Root, "box", "docs", "up", "deep", "c.txt")); string(got) != "sea" {
		t.Fatal("upload missing")
	}

	// remote to remote: one copyto per file, rclone to rclone (server-side when the provider can)
	if _, err := n.Connect("rclone://other/"); err != nil {
		t.Fatal(err)
	}
	id, _ = n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{res.Path + "/a.txt"}, Target: "rclone://other/"})
	if p := waitJob(t, n, id, nil); p.State != "done" {
		t.Fatalf("remote copy: %+v", p)
	}
	if got, _ := os.ReadFile(filepath.Join(f.Root, "other", "a.txt")); string(got) != "hello" {
		t.Fatal("remote copy missing")
	}
	var direct bool
	for _, c := range f.Calls() {
		if strings.Contains(c, "copyto\tbox:docs/a.txt\tother:a.txt") {
			direct = true
		}
		if strings.Contains(c, "\tcat\t") || strings.Contains(c, "\trcat\t") {
			t.Errorf("remote to remote went through this computer: %s", c)
		}
	}
	if !direct {
		t.Errorf("no direct copyto between the remotes: %v", f.Calls())
	}

	// a move within one remote is a rename (moveto)
	id, _ = n.StartJob(NetJobRequest{Kind: "move", Sources: []string{res.Path + "/up"}, Target: res.Path + "/sub"})
	if p := waitJob(t, n, id, nil); p.State != "done" {
		t.Fatalf("move: %+v", p)
	}
	if _, err := os.Stat(filepath.Join(f.Root, "box", "docs", "sub", "up", "deep", "c.txt")); err != nil {
		t.Fatal("moved folder missing")
	}
	if _, err := os.Stat(filepath.Join(f.Root, "box", "docs", "up")); err == nil {
		t.Fatal("source still there after move")
	}

	// rename, make folder, delete (file, empty folder, full folder)
	r := n.router()
	if err := r.Rename(t.Context(), "rclone://other/a.txt", "rclone://other/b.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.Rename(t.Context(), "rclone://box/docs/a.txt", "rclone://box/docs/sub"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("rename onto an existing name: %v", err)
	}
	if err := r.Mkdir(t.Context(), "rclone://other/new"); err != nil {
		t.Fatal(err)
	}
	if err := r.Mkdir(t.Context(), "rclone://other/new"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("mkdir of an existing folder: %v", err)
	}
	if err := r.Remove(t.Context(), "rclone://other/new"); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveAll(t.Context(), "rclone://other/b.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveAll(t.Context(), "rclone://box/docs/sub"); err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(filepath.Join(f.Root, "other")); len(es) != 0 {
		t.Errorf("other still holds %d entries", len(es))
	}
	if _, err := os.Stat(filepath.Join(f.Root, "box", "docs", "sub")); err == nil {
		t.Error("purged folder still there")
	}
	if err := r.RemoveAll(t.Context(), "rclone://box/"); err == nil {
		t.Error("the root of a remote must not be deletable")
	}

	// disconnect closes the remote: calls on it no longer start rclone
	if err := n.Disconnect(res.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := n.rclone.Stat("rclone://box/docs/a.txt"); err == nil {
		t.Error("a closed remote must not answer")
	}
}
