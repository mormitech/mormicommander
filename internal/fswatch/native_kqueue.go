//go:build darwin || freebsd || netbsd || openbsd || dragonfly

// Native backend for macOS and the BSDs, built on kqueue. FSEvents would need cgo, which this project avoids so that
// every package cross-compiles, and kqueue is what fsnotify uses on these systems too. kqueue watches open file
// descriptors, not paths: a descriptor on the folder reports when entries are created, removed or renamed, but not
// when an existing file grows. So the backend also opens up to Options.FileWatchLimit regular files of the folder
// (on macOS with O_EVTONLY, which does not keep the volume from being ejected) and keeps that set in step with the
// folder listing. Comparing successive listings also tells which names appeared or disappeared, so Events can carry
// names even though kqueue itself does not. A descriptor whose file was deleted is closed at once so the disk space
// is released. A self-pipe registered in the same kqueue wakes the loop for stop, which is more reliable than
// closing a kqueue another thread is blocked on.
package fswatch

import (
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	dirNotes  = unix.NOTE_WRITE | unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_REVOKE | unix.NOTE_ATTRIB | unix.NOTE_EXTEND | unix.NOTE_LINK
	fileNotes = unix.NOTE_WRITE | unix.NOTE_EXTEND | unix.NOTE_ATTRIB | unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_REVOKE
)

type kqueueBackend struct {
	path   string
	report func(raw)
	limit  int

	kq     int
	dirfd  int
	pr, pw int
	files  map[string]int // name -> fd
	byFD   map[int]string
	names  map[string]bool // last listing (all entries)

	startFiles int

	once sync.Once
	done chan struct{}
}

func startNativeBackend(path string, opts Options, report func(raw)) (backend, error) {
	b := &kqueueBackend{path: path, report: report, limit: opts.FileWatchLimit, kq: -1, dirfd: -1, pr: -1, pw: -1,
		files: map[string]int{}, byFD: map[int]string{}, done: make(chan struct{})}
	if err := b.open(); err != nil {
		b.closeAll()
		return nil, err
	}
	b.startFiles = len(b.files)
	go b.loop()
	return b, nil
}

func (b *kqueueBackend) open() error {
	var err error
	if b.kq, err = unix.Kqueue(); err != nil {
		return err
	}
	unix.CloseOnExec(b.kq)
	if b.dirfd, err = unix.Open(b.path, evtOpenFlags|unix.O_DIRECTORY|unix.O_CLOEXEC, 0); err != nil {
		return err
	}
	var p [2]int
	if err = unix.Pipe(p[:]); err != nil {
		return err
	}
	b.pr, b.pw = p[0], p[1]
	unix.CloseOnExec(b.pr)
	unix.CloseOnExec(b.pw)
	_ = unix.SetNonblock(b.pr, true)
	_ = unix.SetNonblock(b.pw, true)

	ch := make([]unix.Kevent_t, 2)
	unix.SetKevent(&ch[0], b.dirfd, unix.EVFILT_VNODE, unix.EV_ADD|unix.EV_CLEAR|unix.EV_ENABLE)
	ch[0].Fflags = dirNotes
	unix.SetKevent(&ch[1], b.pr, unix.EVFILT_READ, unix.EV_ADD|unix.EV_ENABLE)
	if _, err = unix.Kevent(b.kq, ch, nil, nil); err != nil {
		return err
	}
	b.names, _ = b.sync()
	return nil
}

// sync rereads the folder listing, opens watches for new regular files (within the limit), closes watches for
// names that disappeared, and returns the names added or removed since the previous listing.
func (b *kqueueBackend) sync() (map[string]bool, []string) {
	cur := map[string]bool{}
	var regular []string
	f, err := os.Open(b.path)
	if err != nil {
		return b.names, nil
	}
	ents, _ := f.ReadDir(-1)
	f.Close()
	for _, de := range ents {
		cur[de.Name()] = true
		if de.Type().IsRegular() {
			regular = append(regular, de.Name())
		}
	}

	var changed []string
	for n := range cur {
		if !b.names[n] {
			changed = append(changed, n)
		}
	}
	for n := range b.names {
		if !cur[n] {
			changed = append(changed, n)
		}
	}
	for n, f := range b.files {
		if !cur[n] {
			b.unwatchFile(n, f)
		}
	}
	if b.limit > 0 {
		for _, n := range regular {
			if len(b.files) >= b.limit {
				break
			}
			if _, ok := b.files[n]; ok {
				continue
			}
			b.watchFile(n)
		}
	}
	return cur, changed
}

func (b *kqueueBackend) watchFile(name string) {
	fd, err := unix.Open(b.path+"/"+name, evtOpenFlags|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return
	}
	ch := make([]unix.Kevent_t, 1)
	unix.SetKevent(&ch[0], fd, unix.EVFILT_VNODE, unix.EV_ADD|unix.EV_CLEAR|unix.EV_ENABLE)
	ch[0].Fflags = fileNotes
	if _, err := unix.Kevent(b.kq, ch, nil, nil); err != nil {
		unix.Close(fd)
		return
	}
	b.files[name] = fd
	b.byFD[fd] = name
}

// unwatchFile closes a file descriptor; closing removes its kevents automatically.
func (b *kqueueBackend) unwatchFile(name string, fd int) {
	unix.Close(fd)
	delete(b.files, name)
	delete(b.byFD, fd)
}

func (b *kqueueBackend) loop() {
	defer close(b.done)
	defer b.closeFDs()
	evs := make([]unix.Kevent_t, 64)
	for {
		n, err := unix.Kevent(b.kq, nil, evs, nil)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			b.report(raw{Terminal: true, Err: err})
			return
		}
		dirChanged := false
		var names []string
		for _, ev := range evs[:n] {
			fd := int(ev.Ident)
			switch {
			case fd == b.pr:
				return
			case fd == b.dirfd:
				if ev.Fflags&(unix.NOTE_DELETE|unix.NOTE_RENAME|unix.NOTE_REVOKE) != 0 {
					b.report(raw{Terminal: true})
					return
				}
				dirChanged = true
			default:
				name, ok := b.byFD[fd]
				if !ok {
					continue
				}
				names = append(names, name)
				if ev.Fflags&(unix.NOTE_DELETE|unix.NOTE_RENAME|unix.NOTE_REVOKE) != 0 {
					b.unwatchFile(name, fd)
					dirChanged = true
				}
			}
		}
		if dirChanged {
			var changed []string
			b.names, changed = b.sync()
			names = append(names, changed...)
			if len(changed) == 0 && len(names) == 0 {
				// Attributes of the folder or a rename over an existing name: we cannot tell what changed.
				b.report(raw{})
			}
		}
		for _, nm := range names {
			b.report(raw{Name: nm})
		}
	}
}

func (b *kqueueBackend) stop() {
	b.once.Do(func() {
		if b.pw >= 0 {
			_, _ = unix.Write(b.pw, []byte{0})
		}
		<-b.done
		// The write end is closed only here, after the loop has exited, so a stop after the loop ended by itself
		// never writes to a descriptor number that was closed and reused.
		if b.pw >= 0 {
			unix.Close(b.pw)
			b.pw = -1
		}
	})
	<-b.done
}

// closeAll releases everything; used when start fails before the loop runs.
func (b *kqueueBackend) closeAll() {
	b.closeFDs()
	if b.pw >= 0 {
		unix.Close(b.pw)
		b.pw = -1
	}
}

// closeFDs releases everything except the pipe's write end, which belongs to stop.
func (b *kqueueBackend) closeFDs() {
	for n, fd := range b.files {
		b.unwatchFile(n, fd)
	}
	for _, fd := range []*int{&b.dirfd, &b.kq, &b.pr} {
		if *fd >= 0 {
			unix.Close(*fd)
			*fd = -1
		}
	}
}

// watchedFiles reports how many per-file watches were opened at start; used by tests. The live map belongs to the
// loop goroutine, so it is not read here.
func (b *kqueueBackend) watchedFiles() int { return b.startFiles }
