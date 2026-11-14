// macOS says "no such attribute" with ENOATTR.
package xattr

import "golang.org/x/sys/unix"

const errNoAttr = unix.ENOATTR
