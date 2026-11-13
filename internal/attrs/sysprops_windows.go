// On Windows the native properties window is the Explorer property sheet,
// opened with SHObjectProperties. The sheet is modeless and runs on the
// calling thread's message loop, so the UI should call this from its main
// (window) thread; Wails runs bound methods on goroutines, so the caller is
// expected to dispatch it to the UI thread when that matters.
package attrs

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modshell32             = windows.NewLazySystemDLL("shell32.dll")
	procSHObjectProperties = modshell32.NewProc("SHObjectProperties")
)

const shopFilePath = 0x2 // SHOP_FILEPATH

func openSystemProperties(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, _ := procSHObjectProperties.Call(0, shopFilePath, uintptr(unsafe.Pointer(p)), 0)
	if r == 0 {
		return errors.New("could not open the Windows properties window")
	}
	return nil
}
