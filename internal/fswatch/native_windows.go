// Native backend for Windows, built on ReadDirectoryChangesW, the same API Total Commander uses for its automatic
// refresh. The folder is opened with full sharing (so other programs can still rename and delete inside it) and read
// with overlapped I/O; the loop waits on the I/O event and on a stop event, so stop can cancel a pending read cleanly
// with CancelIoEx. The filter covers names, sizes, write times and attributes, which are the columns a panel shows.
// The API returns exact entry names. A zero-length result or ERROR_NOTIFY_ENUM_DIR means the buffer overflowed and a
// full reread is needed. Any other failure (typically ERROR_ACCESS_DENIED once the folder is being deleted) ends the
// backend and the watcher checks whether the folder still exists.
package fswatch

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const winNotifyFilter = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
	windows.FILE_NOTIFY_CHANGE_ATTRIBUTES | windows.FILE_NOTIFY_CHANGE_SIZE | windows.FILE_NOTIFY_CHANGE_LAST_WRITE |
	windows.FILE_NOTIFY_CHANGE_CREATION

type winBackend struct {
	report func(raw)
	h      windows.Handle
	ioEv   windows.Handle
	stopEv windows.Handle
	buf    []uint32 // DWORD aligned, as the API requires
	ov     *windows.Overlapped
	once   sync.Once
	done   chan struct{}
}

func startNativeBackend(path string, _ Options, report func(raw)) (backend, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, err
	}
	ioEv, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	stopEv, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(ioEv)
		windows.CloseHandle(h)
		return nil, err
	}
	// 64 KiB is the largest buffer that works for folders on network shares.
	b := &winBackend{report: report, h: h, ioEv: ioEv, stopEv: stopEv, buf: make([]uint32, 16*1024),
		ov: &windows.Overlapped{HEvent: ioEv}, done: make(chan struct{})}
	if err := b.issue(); err != nil {
		b.closeAll()
		return nil, err
	}
	go b.loop()
	return b, nil
}

// issue starts one overlapped read of change records.
func (b *winBackend) issue() error {
	_ = windows.ResetEvent(b.ioEv)
	*b.ov = windows.Overlapped{HEvent: b.ioEv}
	return windows.ReadDirectoryChanges(b.h, (*byte)(unsafe.Pointer(&b.buf[0])), uint32(len(b.buf)*4), false,
		winNotifyFilter, nil, b.ov, 0)
}

func (b *winBackend) loop() {
	defer close(b.done)
	defer b.closeLoopHandles()
	for {
		r, err := windows.WaitForMultipleObjects([]windows.Handle{b.ioEv, b.stopEv}, false, windows.INFINITE)
		if err != nil {
			b.cancel()
			b.report(raw{Terminal: true, Err: err})
			return
		}
		if r == windows.WAIT_OBJECT_0+1 {
			b.cancel()
			return
		}
		var n uint32
		err = windows.GetOverlappedResult(b.h, b.ov, &n, false)
		switch {
		case err == windows.ERROR_NOTIFY_ENUM_DIR || (err == nil && n == 0):
			b.report(raw{Overflow: true})
		case err != nil:
			b.report(raw{Terminal: true, Err: err})
			return
		default:
			b.parse(n)
		}
		if err := b.issue(); err != nil {
			b.report(raw{Terminal: true, Err: err})
			return
		}
	}
}

// parse walks the FILE_NOTIFY_INFORMATION records in the first n bytes of the buffer.
func (b *winBackend) parse(n uint32) {
	base := unsafe.Pointer(&b.buf[0])
	var off uint32
	for off+12 <= n {
		rec := (*windows.FileNotifyInformation)(unsafe.Add(base, off))
		chars := rec.FileNameLength / 2
		name := windows.UTF16ToString(unsafe.Slice(&rec.FileName, chars))
		b.report(raw{Name: name})
		if rec.NextEntryOffset == 0 {
			break
		}
		off += rec.NextEntryOffset
	}
}

// cancel aborts a pending read and waits until the kernel no longer uses the buffer.
func (b *winBackend) cancel() {
	if err := windows.CancelIoEx(b.h, b.ov); err == nil {
		var n uint32
		_ = windows.GetOverlappedResult(b.h, b.ov, &n, true)
	}
}

func (b *winBackend) stop() {
	b.once.Do(func() {
		_ = windows.SetEvent(b.stopEv)
		<-b.done
		// The stop event is closed only here, after the loop exited, so a stop after the loop ended by itself
		// never signals a closed (and possibly reused) handle.
		windows.CloseHandle(b.stopEv)
	})
	<-b.done
}

// closeLoopHandles releases the directory handle and the I/O event when the loop ends.
func (b *winBackend) closeLoopHandles() {
	windows.CloseHandle(b.h)
	windows.CloseHandle(b.ioEv)
}

// closeAll releases everything; used when start fails before the loop runs.
func (b *winBackend) closeAll() {
	b.closeLoopHandles()
	windows.CloseHandle(b.stopEv)
}
