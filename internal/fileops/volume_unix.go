//go:build unix

package fileops

// On Unix-like systems every mounted file system has its own device number, reported by stat as st_dev; two paths
// are on the same volume exactly when their device numbers match. This is what TC's "same drive" test becomes here.

import "syscall"

func volumeOf(p string) (string, bool) {
	fi, err := lstat(p)
	if err != nil {
		return "", false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	return "dev:" + uitoa(uint64(st.Dev)), true
}

func uitoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func isCrossDeviceOS(err error) bool { return false }
