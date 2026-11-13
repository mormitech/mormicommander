//go:build darwin || linux

package fileops

// Tests that extended attributes travel with a copy (and stay behind with NoAttrs). File systems without user
// attributes (some Linux tmpfs setups) skip the test.

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestCopyXattrs(t *testing.T) {
	src := t.TempDir()
	f := filepath.Join(src, "f")
	write(t, f, "x")
	name := "user.mormi.test"
	if err := unix.Setxattr(f, name, []byte("tag"), 0); err != nil {
		t.Skip("no extended attributes here:", err)
	}
	q := NewQueue(nil)
	dst := t.TempDir()
	start(t, q, Request{Kind: Copy, Sources: []string{f}, Target: dst})
	buf := make([]byte, 16)
	n, err := unix.Getxattr(filepath.Join(dst, "f"), name, buf)
	if err != nil || string(buf[:n]) != "tag" {
		t.Fatalf("attribute not copied: %q %v", buf[:max(n, 0)], err)
	}
	dst = t.TempDir()
	start(t, q, Request{Kind: Copy, Sources: []string{f}, Target: dst, NoAttrs: true})
	if _, err := unix.Getxattr(filepath.Join(dst, "f"), name, buf); err == nil {
		t.Error("attribute copied despite NoAttrs")
	}
}
