//go:build darwin || linux

package fileops

// Extended attributes hold things the plain file doesn't: on macOS the Finder tags and colour labels, the "downloaded
// from the internet" quarantine flag, and the old resource forks (com.apple.ResourceFork); on Linux user.* attributes
// and ACLs. They are copied one by one with the l* calls so a link's own attributes are read, never its target's.
// Attributes the target refuses (no permission, or a file system without support) are left out quietly; other errors
// are reported to the caller, which logs them.

import (
	"bytes"
	"errors"

	"golang.org/x/sys/unix"
)

func copyXattrs(src, dst string) error {
	names, err := listXattrs(src)
	if err != nil || len(names) == 0 {
		return ignorable(err)
	}
	var first error
	for _, name := range names {
		val, err := getXattr(src, name)
		if err != nil {
			if first == nil {
				first = ignorable(err)
			}
			continue
		}
		if err := unix.Lsetxattr(lp(dst), name, val, 0); err != nil && first == nil {
			first = ignorable(err)
		}
	}
	return first
}

func listXattrs(p string) ([]string, error) {
	for {
		n, err := unix.Llistxattr(lp(p), nil)
		if err != nil || n == 0 {
			return nil, err
		}
		buf := make([]byte, n)
		n2, err := unix.Llistxattr(lp(p), buf)
		if errors.Is(err, unix.ERANGE) {
			continue // the list grew between the two calls
		}
		if err != nil {
			return nil, err
		}
		var out []string
		for _, b := range bytes.Split(buf[:n2], []byte{0}) {
			if len(b) > 0 {
				out = append(out, string(b))
			}
		}
		return out, nil
	}
}

func getXattr(p, name string) ([]byte, error) {
	for {
		n, err := unix.Lgetxattr(lp(p), name, nil)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, n)
		n2, err := unix.Lgetxattr(lp(p), name, buf)
		if errors.Is(err, unix.ERANGE) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return buf[:n2], nil
	}
}

// ignorable turns the errors that only mean "this file system or user can't do that" into nil.
func ignorable(err error) error {
	if err == nil || errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EPERM) ||
		errors.Is(err, unix.EACCES) {
		return nil
	}
	return err
}
