//go:build windows

// Native file drags on Windows (board issue #108, feature_system_dragout.go). The files are handed to the shell the
// way Explorer drags them: a shell data object of their item IDs (SHCreateDataObject, which carries CF_HDROP and the
// shell formats) and SHDoDragDrop with the shell's own drop source, so Explorer and other programs receive real files.
// Other programs are only offered a copy, so a drop never moves a file out of its panel; a drop back inside the window
// arrives as an ordinary file drop and the page picks copy / move / link from the modifier keys.
//
// SHDoDragDrop runs a modal loop on the UI thread until the drop, so it is started there without waiting (Start
// answers at once, as on macOS); the end is passed back through dragEnded. Nothing here goes online or deletes files.
package main

import (
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const nativeDragSupported = true

var (
	ddShell32              = syscall.NewLazyDLL("shell32.dll")
	ddOle32                = syscall.NewLazyDLL("ole32.dll")
	ddUser32               = syscall.NewLazyDLL("user32.dll")
	procILCreateFromPathW  = ddShell32.NewProc("ILCreateFromPathW")
	procILFindLastID       = ddShell32.NewProc("ILFindLastID")
	procILFree             = ddShell32.NewProc("ILFree")
	procSHCreateDataObject = ddShell32.NewProc("SHCreateDataObject")
	procSHDoDragDrop       = ddShell32.NewProc("SHDoDragDrop")
	procOleInitialize      = ddOle32.NewProc("OleInitialize")
	procGetAsyncKeyState   = ddUser32.NewProc("GetAsyncKeyState")
	procGetSystemMetrics   = ddUser32.NewProc("GetSystemMetrics")
)

// IID_IDataObject {0000010E-0000-0000-C000-000000000046}
var iidIDataObject = syscall.GUID{Data1: 0x0000010E, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}

const (
	vkLButton    = 0x01
	vkRButton    = 0x02
	vkShift      = 0x10
	vkControl    = 0x11
	vkMenu       = 0x12
	vkLWin       = 0x5B
	vkRWin       = 0x5C
	smSwapButton = 23
)

func keyDown(vk uintptr) bool {
	r, _, _ := procGetAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}

// leftButtonDown: the (logical) left mouse button is held now.
func leftButtonDown() bool {
	if s, _, _ := procGetSystemMetrics.Call(smSwapButton); s != 0 {
		return keyDown(vkRButton)
	}
	return keyDown(vkLButton)
}

// nativeFileDrag starts a native drag of the files from the window; false when it could not start (no window, or the
// mouse button already let go). The drag itself runs on the UI thread after this returns.
func nativeFileDrag(w *application.WebviewWindow, paths []string) bool {
	if w == nil || len(paths) == 0 || !leftButtonDown() {
		return false
	}
	hwnd := uintptr(w.NativeWindow())
	if hwnd == 0 {
		return false
	}
	application.InvokeAsync(func() { dragEnded(shellDrag(hwnd, paths)) })
	return true
}

// shellDrag makes the shell data object of the files and runs the drag; it answers the operation bits of the drop
// (0 when it was cancelled or could not run).
func shellDrag(hwnd uintptr, paths []string) int {
	procOleInitialize.Call(0) // (already initialised by WebView2 as an STA; OLE wants its own count for drags)

	var pidls []uintptr
	defer func() {
		for _, p := range pidls {
			procILFree.Call(p)
		}
	}()
	for _, p := range paths {
		u, err := syscall.UTF16PtrFromString(p)
		if err != nil {
			return 0
		}
		pidl, _, _ := procILCreateFromPathW.Call(uintptr(unsafe.Pointer(u)))
		if pidl == 0 {
			return 0
		}
		pidls = append(pidls, pidl)
	}

	// Files of one folder (always so for a panel) are given relative to it, as Explorer does; otherwise as full item
	// IDs relative to the desktop.
	var folder uintptr
	items := make([]uintptr, len(pidls))
	copy(items, pidls)
	if dir, same := commonDir(paths); same {
		u, err := syscall.UTF16PtrFromString(dir)
		if err == nil {
			if f, _, _ := procILCreateFromPathW.Call(uintptr(unsafe.Pointer(u))); f != 0 {
				folder = f
				defer procILFree.Call(f)
				for i, p := range pidls {
					last, _, _ := procILFindLastID.Call(p)
					items[i] = last
				}
			}
		}
	}

	var obj unsafe.Pointer
	hr, _, _ := procSHCreateDataObject.Call(folder, uintptr(len(items)), uintptr(unsafe.Pointer(&items[0])), 0,
		uintptr(unsafe.Pointer(&iidIDataObject)), uintptr(unsafe.Pointer(&obj)))
	if int32(hr) < 0 || obj == nil {
		return 0
	}
	defer comRelease(obj)

	var effect uint32
	hr, _, _ = procSHDoDragDrop.Call(hwnd, uintptr(obj), 0, dropEffectCopy, uintptr(unsafe.Pointer(&effect)))
	if int32(hr) < 0 || hr == 0x00040101 { // failed, or DRAGDROP_S_CANCEL
		return 0
	}
	return dragOpFromDropEffect(effect)
}

// comRelease calls IUnknown::Release on a COM object.
func comRelease(obj unsafe.Pointer) {
	vtbl := (*[3]uintptr)(*(*unsafe.Pointer)(obj))
	syscall.SyscallN(vtbl[2], uintptr(obj))
}

// heldModifiers reads the modifier keys held now.
func heldModifiers() DragModifiers {
	return DragModifiers{
		ShiftKey: keyDown(vkShift),
		CtrlKey:  keyDown(vkControl),
		AltKey:   keyDown(vkMenu),
		MetaKey:  keyDown(vkLWin) || keyDown(vkRWin),
	}
}
