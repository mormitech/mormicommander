package main

// A server's listing is not trusted: a hostile (or broken) SFTP server can answer READDIR with a name such as
// "../escaped.txt", and the transfer job must not turn that into a local path outside the target folder. The server
// here is a fake file system registered for its own scheme, so no network is involved.

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mormicommander/internal/vfs"
)

// hostileFS serves one folder, evilsrv://h/dir, whose listing holds the names in names. Every file downloads as
// "payload".
type hostileFS struct {
	mu    sync.Mutex
	names []string
}

func (h *hostileFS) List(_ context.Context, p string) (vfs.Listing, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Clean(p) != "evilsrv://h/dir" {
		return vfs.Listing{}, &fs.PathError{Op: "list", Path: p, Err: fs.ErrNotExist}
	}
	var es []vfs.Entry
	for _, n := range h.names {
		es = append(es, vfs.Entry{Name: n, Size: 7})
	}
	return vfs.Listing{Path: "evilsrv://h/dir", Entries: es}, nil
}

func (h *hostileFS) Stat(p string) (vfs.Entry, error) {
	if h.Clean(p) == "evilsrv://h/dir" {
		return vfs.Entry{Name: "dir", Flags: vfs.FlagDir, Mode: uint32(fs.ModeDir | 0o755)}, nil
	}
	return vfs.Entry{Name: path.Base(p), Size: 7}, nil
}

func (h *hostileFS) Clean(p string) string {
	rest := strings.TrimPrefix(p, "evilsrv://h")
	return "evilsrv://h" + path.Clean("/"+rest)
}
func (h *hostileFS) Parent(p string) string {
	return "evilsrv://h" + path.Dir(strings.TrimPrefix(h.Clean(p), "evilsrv://h"))
}
func (h *hostileFS) Root(string) string { return "evilsrv://h/" }
func (h *hostileFS) Join(dir, name string) string {
	return strings.TrimSuffix(dir, "/") + "/" + name
}

func (h *hostileFS) Download(_ context.Context, _, local string, _ vfs.TransferOptions) (int64, error) {
	return 7, os.WriteFile(local, []byte("payload"), 0o644)
}

func (h *hostileFS) Upload(context.Context, string, string, vfs.TransferOptions) (int64, error) {
	return 0, vfs.ErrUnsupported
}

var hostileSrv = &hostileFS{}

func init() { vfs.Register("evilsrv", hostileSrv) }

// A listing name that climbs out of the folder ("../escaped.txt", "a/../../deep.txt") fails the copy instead of being
// written next to the copied folder.
func TestNetJobRefusesClimbingServerName(t *testing.T) {
	hostileSrv.mu.Lock()
	hostileSrv.names = []string{"ok.txt", "../escaped.txt", "a/../../deep.txt"}
	hostileSrv.mu.Unlock()
	n, _ := testNet(t, nil)
	target := t.TempDir()
	id, err := n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{"evilsrv://h/dir"}, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	p := waitJob(t, n, id, nil)
	for _, outside := range []string{filepath.Join(target, "escaped.txt"), filepath.Join(target, "deep.txt")} {
		if _, err := os.Stat(outside); err == nil {
			t.Errorf("a server name was written outside the copied folder: %s", outside)
		}
	}
	if p.State != "failed" || len(p.Errors) == 0 {
		t.Fatalf("the copy of a folder with a climbing name should fail, got %+v", p)
	}
}
