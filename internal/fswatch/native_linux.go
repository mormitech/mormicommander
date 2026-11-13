// Native backend for Linux, built on inotify. One inotify instance per watched folder keeps the code simple and the
// backends independent; a commander watches only a handful of folders (one per panel and tab in view), far below the
// default limit of 128 instances per user. inotify reports the name of every entry that is created, deleted, moved,
// written or has its attributes changed, so Events carry exact names. A queue overflow means events were lost, so it
// becomes a full reread. When the folder itself is deleted, moved or its filesystem unmounted, the backend stops and
// the watcher decides between "gone" and "restart". A self-pipe polled next to the inotify descriptor wakes the loop
// for stop without relying on close semantics of a descriptor another thread is reading.
package fswatch

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

const inotifyMask = unix.IN_CREATE | unix.IN_DELETE | unix.IN_MOVED_FROM | unix.IN_MOVED_TO | unix.IN_MODIFY |
	unix.IN_ATTRIB | unix.IN_CLOSE_WRITE | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF | unix.IN_ONLYDIR | unix.IN_EXCL_UNLINK

type inotifyBackend struct {
	report func(raw)
	ifd    int
	pr, pw int
	once   sync.Once
	done   chan struct{}
}

func startNativeBackend(path string, _ Options, report func(raw)) (backend, error) {
	ifd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	if _, err := unix.InotifyAddWatch(ifd, path, inotifyMask); err != nil {
		unix.Close(ifd)
		return nil, err
	}
	var p [2]int
	if err := unix.Pipe2(p[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		unix.Close(ifd)
		return nil, err
	}
	b := &inotifyBackend{report: report, ifd: ifd, pr: p[0], pw: p[1], done: make(chan struct{})}
	go b.loop()
	return b, nil
}

func (b *inotifyBackend) loop() {
	defer close(b.done)
	defer func() {
		unix.Close(b.ifd)
		unix.Close(b.pr)
	}()
	buf := make([]byte, 64*1024)
	fds := []unix.PollFd{{Fd: int32(b.ifd), Events: unix.POLLIN}, {Fd: int32(b.pr), Events: unix.POLLIN}}
	for {
		fds[0].Revents, fds[1].Revents = 0, 0
		_, err := unix.Poll(fds, -1)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			b.report(raw{Terminal: true, Err: err})
			return
		}
		if fds[1].Revents != 0 {
			return
		}
		n, err := unix.Read(b.ifd, buf)
		if err == unix.EAGAIN || err == unix.EINTR {
			continue
		}
		if err != nil || n <= 0 {
			b.report(raw{Terminal: true, Err: err})
			return
		}
		if b.parse(buf[:n]) {
			return
		}
	}
}

// parse handles a buffer of inotify events and returns true when the watch has ended.
func (b *inotifyBackend) parse(buf []byte) bool {
	for off := 0; off+unix.SizeofInotifyEvent <= len(buf); {
		ev := (*unix.InotifyEvent)(unsafe.Pointer(&buf[off]))
		nameStart := off + unix.SizeofInotifyEvent
		nameEnd := nameStart + int(ev.Len)
		if nameEnd > len(buf) {
			break
		}
		name := string(buf[nameStart:nameEnd])
		for i := 0; i < len(name); i++ {
			if name[i] == 0 {
				name = name[:i]
				break
			}
		}
		off = nameEnd
		switch {
		case ev.Mask&unix.IN_Q_OVERFLOW != 0:
			b.report(raw{Overflow: true})
		case ev.Mask&(unix.IN_DELETE_SELF|unix.IN_MOVE_SELF|unix.IN_UNMOUNT|unix.IN_IGNORED) != 0:
			b.report(raw{Terminal: true})
			return true
		default:
			b.report(raw{Name: name})
		}
	}
	return false
}

func (b *inotifyBackend) stop() {
	b.once.Do(func() {
		_, _ = unix.Write(b.pw, []byte{0})
		<-b.done
		unix.Close(b.pw)
	})
	<-b.done
}
