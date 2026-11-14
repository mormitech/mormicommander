// This file exposes the process umask (see umask_unix.go) to the archive
// engines, so every unpacker treats the permission bits stored in an archive
// the same way: the bits the umask forbids are dropped, exactly as tar,
// unzip and Archive Utility do for a normal user.
package arcapi

import "io/fs"

// Umask returns the permission bits newly created files must not get. It is
// 0 on systems without a umask.
func Umask() fs.FileMode { return processUmask }

// ApplyUmask removes the umask bits from the permission bits of m and keeps
// the type bits (folder, link, ...) as they are.
func ApplyUmask(m fs.FileMode) fs.FileMode { return m &^ processUmask }
