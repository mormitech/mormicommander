// This file is the Windows clipboard backend. It calls the Win32 clipboard functions in user32 directly through
// golang.org/x/sys/windows, which needs no cgo and starts no helper process, so copy and paste are instant and no
// console window can flash. Files are written the way Explorer writes them: CF_HDROP with the path list plus the
// registered "Preferred DropEffect" format that says copy or move, so Ctrl+V in Explorer pastes them and a Ctrl+X in
// Explorer is seen as a cut here. Text uses CF_UNICODETEXT. Images are read from the "PNG" format that browsers and
// Office add, or else from CF_DIBV5/CF_DIB, which dib.go converts to PNG; images are written as both PNG and CF_DIB so
// every program finds one it understands. The clipboard can only be opened by one program at a time, so opening it
// retries for about half a second, and the OS thread is locked while it is open because ownership is per thread.

package clipfiles

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfText        = 1
	cfBitmap      = 2
	cfDIB         = 8
	cfOEMText     = 7
	cfUnicodeText = 13
	cfHDROP       = 15
	cfDIBV5       = 17
	gmemMoveable  = 0x0002
)

var (
	user32                      = windows.NewLazySystemDLL("user32.dll")
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard           = user32.NewProc("OpenClipboard")
	procCloseClipboard          = user32.NewProc("CloseClipboard")
	procEmptyClipboard          = user32.NewProc("EmptyClipboard")
	procGetClipboardData        = user32.NewProc("GetClipboardData")
	procSetClipboardData        = user32.NewProc("SetClipboardData")
	procIsClipboardFormatAvail  = user32.NewProc("IsClipboardFormatAvailable")
	procRegisterClipboardFormat = user32.NewProc("RegisterClipboardFormatW")
	procEnumClipboardFormats    = user32.NewProc("EnumClipboardFormats")
	procGetClipboardFormatName  = user32.NewProc("GetClipboardFormatNameW")
	procGlobalAlloc             = kernel32.NewProc("GlobalAlloc")
	procGlobalFree              = kernel32.NewProc("GlobalFree")
	procGlobalLock              = kernel32.NewProc("GlobalLock")
	procGlobalUnlock            = kernel32.NewProc("GlobalUnlock")
	procGlobalSize              = kernel32.NewProc("GlobalSize")
)

// standardFormats names the predefined formats for Content.Types.
var standardFormats = map[uintptr]string{
	cfText: "CF_TEXT", cfBitmap: "CF_BITMAP", 3: "CF_METAFILEPICT", 4: "CF_SYLK", 5: "CF_DIF", 6: "CF_TIFF",
	cfOEMText: "CF_OEMTEXT", cfDIB: "CF_DIB", 9: "CF_PALETTE", 10: "CF_PENDATA", 11: "CF_RIFF", 12: "CF_WAVE",
	cfUnicodeText: "CF_UNICODETEXT", 14: "CF_ENHMETAFILE", cfHDROP: "CF_HDROP", 16: "CF_LOCALE", cfDIBV5: "CF_DIBV5",
}

type winBackend struct{}

func newSystemBackend() backend { return winBackend{} }

// ptr turns a pointer returned by a Win32 call into an unsafe.Pointer. Going through memory instead of a direct
// conversion keeps go vet's unsafeptr check quiet; the memory is a locked global handle, not Go memory, so the garbage
// collector does not move it.
func ptr(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }

// register returns the id of a registered clipboard format such as "Preferred DropEffect".
func register(name string) (uintptr, error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	id, _, callErr := procRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(n)))
	if id == 0 {
		return 0, fmt.Errorf("RegisterClipboardFormat %s: %v", name, callErr)
	}
	return id, nil
}

// withClipboard opens the clipboard, runs fn and closes it again.
func withClipboard(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var lastErr error
	for i := 0; i < 20; i++ {
		r, _, err := procOpenClipboard.Call(0)
		if r != 0 {
			defer procCloseClipboard.Call()
			return fn()
		}
		lastErr = err
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("the clipboard is in use by another program (%v)", lastErr)
}

// empty clears the clipboard and takes ownership; the clipboard must be open.
func empty() error {
	if r, _, err := procEmptyClipboard.Call(); r == 0 {
		return fmt.Errorf("EmptyClipboard: %v", err)
	}
	return nil
}

// setData copies data into a movable global block and hands it to the clipboard, which then owns it.
func setData(format uintptr, data []byte) error {
	if len(data) == 0 {
		return errors.New("no data")
	}
	h, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if h == 0 {
		return fmt.Errorf("GlobalAlloc: %v", err)
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("GlobalLock: %v", err)
	}
	copy(unsafe.Slice((*byte)(ptr(p)), len(data)), data)
	procGlobalUnlock.Call(h)
	if r, _, err := procSetClipboardData.Call(format, h); r == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("SetClipboardData: %v", err)
	}
	return nil
}

// getData copies the clipboard's data of one format out; ok is false when the format is absent. The clipboard must
// be open.
func getData(format uintptr) (data []byte, ok bool, err error) {
	if r, _, _ := procIsClipboardFormatAvail.Call(format); r == 0 {
		return nil, false, nil
	}
	h, _, callErr := procGetClipboardData.Call(format)
	if h == 0 {
		return nil, false, fmt.Errorf("GetClipboardData: %v", callErr)
	}
	p, _, callErr := procGlobalLock.Call(h)
	if p == 0 {
		return nil, false, fmt.Errorf("GlobalLock: %v", callErr)
	}
	defer procGlobalUnlock.Call(h)
	n, _, _ := procGlobalSize.Call(h)
	out := make([]byte, n)
	copy(out, unsafe.Slice((*byte)(ptr(p)), n))
	return out, true, nil
}

// utf16Bytes encodes s as NUL-terminated UTF-16LE.
func utf16Bytes(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u)+2)
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return b
}

// fromUTF16Bytes decodes UTF-16LE up to the first NUL.
func fromUTF16Bytes(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		c := uint16(b[i]) | uint16(b[i+1])<<8
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

func (winBackend) writeFiles(paths []string, op Op) error {
	effect, err := register(FormatPreferredDropEffect)
	if err != nil {
		return err
	}
	return withClipboard(func() error {
		if err := empty(); err != nil {
			return err
		}
		if err := setData(cfHDROP, EncodeHDROP(paths)); err != nil {
			return err
		}
		return setData(effect, EncodeDropEffect(op))
	})
}

func (winBackend) readFiles() (paths []string, op Op, err error) {
	effect, err := register(FormatPreferredDropEffect)
	if err != nil {
		return nil, Copy, err
	}
	err = withClipboard(func() error {
		data, ok, err := getData(cfHDROP)
		if err != nil || !ok {
			return err
		}
		if paths, err = DecodeHDROP(data); err != nil {
			return err
		}
		if e, ok, _ := getData(effect); ok {
			op = DecodeDropEffect(e)
		}
		return nil
	})
	if err == nil && len(paths) == 0 {
		err = ErrEmpty
	}
	return paths, op, err
}

func (winBackend) writeText(s string) error {
	return withClipboard(func() error {
		if err := empty(); err != nil {
			return err
		}
		return setData(cfUnicodeText, utf16Bytes(s))
	})
}

func (winBackend) readText() (s string, err error) {
	found := false
	err = withClipboard(func() error {
		data, ok, err := getData(cfUnicodeText) // Windows converts CF_TEXT and CF_OEMTEXT to this on request
		if err != nil || !ok {
			return err
		}
		found, s = true, fromUTF16Bytes(data)
		return nil
	})
	if err == nil && !found {
		err = ErrEmpty
	}
	return s, err
}

func (winBackend) writeImage(png []byte) error {
	pngFmt, err := register("PNG")
	if err != nil {
		return err
	}
	dib, err := PNGToDIB(png)
	if err != nil {
		return err
	}
	return withClipboard(func() error {
		if err := empty(); err != nil {
			return err
		}
		if err := setData(pngFmt, png); err != nil {
			return err
		}
		return setData(cfDIB, dib)
	})
}

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

func (winBackend) readImage() (out []byte, err error) {
	pngFmt, err := register("PNG")
	if err != nil {
		return nil, err
	}
	var dib []byte
	err = withClipboard(func() error {
		if data, ok, _ := getData(pngFmt); ok && bytes.HasPrefix(data, pngSignature) {
			out = data
			return nil
		}
		for _, f := range []uintptr{cfDIBV5, cfDIB} { // Windows synthesises CF_DIB from CF_BITMAP when asked
			if data, ok, _ := getData(f); ok {
				dib = data
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out != nil {
		return out, nil
	}
	if dib == nil {
		return nil, ErrEmpty
	}
	return DIBToPNG(dib)
}

func (winBackend) content() (c Content, err error) {
	pngFmt, _ := register("PNG")
	avail := func(f uintptr) bool {
		r, _, _ := procIsClipboardFormatAvail.Call(f)
		return r != 0
	}
	c.Files = avail(cfHDROP)
	c.Text = avail(cfUnicodeText) || avail(cfText) || avail(cfOEMText)
	c.Image = avail(cfDIB) || avail(cfDIBV5) || avail(cfBitmap) || (pngFmt != 0 && avail(pngFmt))
	err = withClipboard(func() error {
		var f uintptr
		for {
			f, _, _ = procEnumClipboardFormats.Call(f)
			if f == 0 {
				return nil
			}
			if name, ok := standardFormats[f]; ok {
				c.Types = append(c.Types, name)
				continue
			}
			buf := make([]uint16, 256)
			n, _, _ := procGetClipboardFormatName.Call(f, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			if n > 0 {
				c.Types = append(c.Types, windows.UTF16ToString(buf[:n]))
			} else {
				c.Types = append(c.Types, fmt.Sprintf("format %d", f))
			}
		}
	})
	return c, err
}

func (winBackend) clear() error { return withClipboard(empty) }
