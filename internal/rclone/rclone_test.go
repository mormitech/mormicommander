package rclone_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mormicommander/internal/rclone"
	"mormicommander/internal/rclone/rclonetest"
)

func client(t *testing.T, remotes ...string) (*rclone.Client, rclonetest.Fake) {
	f := rclonetest.New(t, remotes...)
	return &rclone.Client{Bin: f.Bin, Env: f.Env()}, f
}

func TestValidRemote(t *testing.T) {
	for _, ok := range []string{"gdrive", "My Box", "s3.eu-west", "a_b+c@d"} {
		if !rclone.ValidRemote(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", ":s3,env_auth", "a:b", "a/b", `a\b`, "-x", " x", "x "} {
		if rclone.ValidRemote(bad) {
			t.Errorf("%q should be refused", bad)
		}
	}
	if got := rclone.Arg("box", "/docs/a.txt"); got != "box:docs/a.txt" {
		t.Errorf("Arg = %q", got)
	}
	if got := rclone.Arg("box", "/"); got != "box:" {
		t.Errorf("Arg root = %q", got)
	}
}

func TestParseRemotesAndErrorText(t *testing.T) {
	rs := rclone.ParseRemotes([]byte("gdrive:              drive\ns3:  s3\n\nbad\n:x:\n"))
	if len(rs) != 2 || rs[0].Name != "gdrive" || rs[0].Type != "drive" || rs[1].Name != "s3" {
		t.Errorf("remotes = %+v", rs)
	}
	if got := rclone.ErrorText([]byte("2026/01/02 10:11:12 NOTICE: x\n2026/01/02 10:11:12 ERROR : a.txt: Failed to copy: 403\n")); got != "a.txt: Failed to copy: 403" {
		t.Errorf("plain = %q", got)
	}
	if got := rclone.ErrorText([]byte(`{"level":"info","msg":"hi"}` + "\n" + `{"level":"error","msg":"boom"}` + "\n")); got != "boom" {
		t.Errorf("json = %q", got)
	}
}

func TestFind(t *testing.T) {
	f := rclonetest.New(t)
	if p, err := rclone.Find(f.Bin); err != nil || p != f.Bin {
		t.Errorf("Find(configured) = %q, %v", p, err)
	}
	if _, err := rclone.Find(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("a missing configured program must be an error")
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := rclone.Find(""); !errors.Is(err, rclone.ErrNotFound) {
		t.Errorf("no rclone on PATH: %v", err)
	}
}

func TestClientOperations(t *testing.T) {
	c, f := client(t, "box", "other")
	ctx := context.Background()
	rs, err := c.Remotes(ctx)
	if err != nil || len(rs) != 2 || rs[0].Name != "box" {
		t.Fatalf("Remotes = %+v, %v", rs, err)
	}
	if err := c.Mkdir(ctx, "box", "/docs/sub"); err != nil {
		t.Fatal(err)
	}
	w, err := c.Rcat(ctx, "box", "/docs/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(w, "hello")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	items, err := c.List(ctx, "box", "/docs")
	if err != nil || len(items) != 2 || items[0].Name != "a.txt" || items[0].Size != 5 || !items[1].IsDir {
		t.Fatalf("List = %+v, %v", items, err)
	}
	it, err := c.Stat(ctx, "box", "/docs/a.txt")
	if err != nil || it.Size != 5 || it.IsDir {
		t.Fatalf("Stat = %+v, %v", it, err)
	}
	if _, err := c.Stat(ctx, "box", "/docs/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing Stat: %v", err)
	}
	r, err := c.Cat(ctx, "box", "/docs/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	r.Close()
	if err != nil || string(b) != "hello" {
		t.Fatalf("Cat = %q, %v", b, err)
	}
	if r, err := c.Cat(ctx, "box", "/nope"); err == nil {
		if _, err := io.ReadAll(r); err == nil {
			t.Error("reading a missing file must fail")
		}
		r.Close()
	}
	var last int64
	if err := c.CopyTo(ctx, rclone.Arg("box", "/docs/a.txt"), rclone.Arg("other", "/b.txt"), func(d, _ int64) { last = d }); err != nil {
		t.Fatal(err)
	}
	if last != 5 {
		t.Errorf("progress = %d", last)
	}
	local := filepath.Join(t.TempDir(), "c.txt")
	if err := c.CopyTo(ctx, rclone.Arg("other", "/b.txt"), local, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(local); string(b) != "hello" {
		t.Errorf("download = %q", b)
	}
	if err := c.MoveTo(ctx, rclone.Arg("other", "/b.txt"), rclone.Arg("other", "/d.txt")); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteFile(ctx, "other", "/d.txt"); err != nil {
		t.Fatal(err)
	}
	if err := c.Rmdir(ctx, "box", "/docs"); err == nil {
		t.Error("rmdir of a full folder must fail")
	}
	if err := c.Purge(ctx, "box", "/docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.Root, "box", "docs")); !os.IsNotExist(err) {
		t.Error("purge left the folder")
	}
	for _, call := range f.Calls() {
		if !strings.HasPrefix(call, "--ask-password=false\t") {
			t.Errorf("every call must refuse to ask for a config password: %q", call)
		}
	}
}

func TestClientError(t *testing.T) {
	c, _ := client(t, "box")
	c.Env = append(c.Env, "FAKE_RCLONE_FAIL=lsjson")
	_, err := c.List(context.Background(), "box", "/")
	if err == nil || !strings.Contains(err.Error(), "403 Forbidden") {
		t.Errorf("error = %v", err)
	}
	if (&rclone.Client{}).Mkdir(context.Background(), "box", "/") != rclone.ErrNotFound {
		t.Error("a client without a program must say rclone was not found")
	}
}
