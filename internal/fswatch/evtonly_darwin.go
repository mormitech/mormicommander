// Open flags for kqueue watches on macOS. O_EVTONLY opens a descriptor that is only good for event notification;
// unlike a normal read-only descriptor it does not keep a volume busy, so a USB stick or disk image whose folder is
// shown in a panel can still be ejected. That matters for a file manager, which keeps watches open for as long as a
// panel shows the folder.
package fswatch

import "golang.org/x/sys/unix"

const evtOpenFlags = unix.O_EVTONLY
