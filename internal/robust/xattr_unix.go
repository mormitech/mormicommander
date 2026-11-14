//go:build darwin || linux

// Extended attributes for Replace on macOS and Linux: Finder tags and colour labels, the quarantine flag and resource
// forks on macOS, user.* attributes and ACLs on Linux. They are copied one by one from the old file to the new one.
// An attribute the system refuses is skipped, since keeping the archive update matters more than any one attribute.
package robust

import (
	"bytes"
	"errors"

	"golang.org/x/sys/unix"
)

func copyXattrs(src, dst string) {
	for _, name := range listXattrs(src) {
		if val, ok := getXattr(src, name); ok {
			_ = unix.Lsetxattr(dst, name, val, 0)
		}
	}
}

func listXattrs(p string) []string {
	for tries := 0; tries < 8; tries++ {
		n, err := unix.Llistxattr(p, nil)
		if err != nil || n <= 0 {
			return nil
		}
		buf := make([]byte, n)
		n2, err := unix.Llistxattr(p, buf)
		if errors.Is(err, unix.ERANGE) {
			continue // the list grew between the two calls
		}
		if err != nil {
			return nil
		}
		var out []string
		for _, b := range bytes.Split(buf[:n2], []byte{0}) {
			if len(b) > 0 {
				out = append(out, string(b))
			}
		}
		return out
	}
	return nil
}

func getXattr(p, name string) ([]byte, bool) {
	for tries := 0; tries < 8; tries++ {
		n, err := unix.Lgetxattr(p, name, nil)
		if err != nil {
			return nil, false
		}
		buf := make([]byte, n)
		n2, err := unix.Lgetxattr(p, name, buf)
		if errors.Is(err, unix.ERANGE) {
			continue
		}
		if err != nil {
			return nil, false
		}
		return buf[:n2], true
	}
	return nil, false
}
