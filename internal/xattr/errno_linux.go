// Linux says "no such attribute" with ENODATA.
package xattr

import "golang.org/x/sys/unix"

const errNoAttr = unix.ENODATA
