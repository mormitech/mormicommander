//go:build darwin || linux

// Extended attributes on macOS and Linux through the l* system calls, so a link's own attributes are used, never its
// target's. A file system without extended attributes (ENOTSUP) reads as a file with none.
package xattr

import (
	"bytes"
	"errors"

	"golang.org/x/sys/unix"
)

const supported = true

func listNames(p string) ([]string, error) {
	for tries := 0; tries < 8; tries++ {
		n, err := unix.Llistxattr(p, nil)
		if err != nil {
			if isAbsent(err) {
				return nil, nil
			}
			return nil, err
		}
		if n <= 0 {
			return nil, nil
		}
		buf := make([]byte, n)
		n2, err := unix.Llistxattr(p, buf)
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
	return nil, unix.ERANGE
}

// valueSize is the size of an attribute's value, or -1 when the file has no such attribute.
func valueSize(p, name string) int {
	n, err := unix.Lgetxattr(p, name, nil)
	if err != nil {
		return -1
	}
	return n
}

func removeName(p, name string) error { return unix.Lremovexattr(p, name) }

// isAbsent says whether an error only means "no such attribute here" or "this file system has none".
func isAbsent(err error) bool {
	return errors.Is(err, errNoAttr) || errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP)
}

// getValue reads an attribute's value; ok false when the file has no such attribute.
func getValue(p, name string) ([]byte, bool, error) {
	for tries := 0; tries < 8; tries++ {
		n, err := unix.Lgetxattr(p, name, nil)
		if err != nil {
			if isAbsent(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		buf := make([]byte, n)
		if n == 0 {
			return buf, true, nil
		}
		n2, err := unix.Lgetxattr(p, name, buf)
		if errors.Is(err, unix.ERANGE) {
			continue // the value grew between the two calls
		}
		if err != nil {
			if isAbsent(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		return buf[:n2], true, nil
	}
	return nil, false, unix.ERANGE
}

func setValue(p, name string, val []byte) error { return unix.Lsetxattr(p, name, val, 0) }
