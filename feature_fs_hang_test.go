package main

// Tests of the panels' listing against a folder that never answers (issue #16): a fake file system whose List blocks
// on a channel, ignoring its context, as a readdir on an SMB/NFS share whose server is gone does. Esc (CancelList) must
// bring the answer back at once, and repeated tries must not pile up blocked reads.

import (
	"context"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/vfs"
)

// deadShareFS is the local file system, except that listing /dead-share blocks until release is closed.
type deadShareFS struct {
	vfs.Local
	release chan struct{}
	started chan struct{}
}

func (d *deadShareFS) List(ctx context.Context, p string) (vfs.Listing, error) {
	if strings.HasPrefix(p, "/dead-share") {
		d.started <- struct{}{}
		<-d.release // a system call does not look at ctx
		return vfs.Listing{}, context.Canceled
	}
	return d.Local.List(ctx, p)
}

func TestFSListCancelOnDeadShare(t *testing.T) {
	dead := &deadShareFS{release: make(chan struct{}), started: make(chan struct{}, 16)}
	defer close(dead.release)
	f := &FS{app: &App{fs: dead}, gen: map[string]listing{}}

	errc := make(chan error, 1)
	go func() { _, err := f.List("left", "/dead-share"); errc <- err }()
	<-dead.started
	f.CancelList("left")
	select {
	case err := <-errc:
		if err == nil || !strings.Contains(err.Error(), "Stopped reading") {
			t.Fatalf("err = %v, want Stopped reading", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("List kept waiting for the dead share after CancelList")
	}

	// the other panel lists a working folder while the dead read is still hanging
	if _, err := f.List("right", t.TempDir()); err != nil {
		t.Fatalf("other panel: %v", err)
	}

	// more tries on the same dead folder: after a few hang, the next answers "not responding" at once
	for i := 0; i < 3; i++ {
		go func() { _, _ = f.List("t", "/dead-share") }()
		<-dead.started
	}
	start := time.Now()
	_, err := f.List("left", "/dead-share")
	if err == nil || !strings.Contains(err.Error(), "not responding") {
		t.Fatalf("err = %v, want not responding", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("refusal was not immediate")
	}
}
