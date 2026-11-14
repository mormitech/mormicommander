// On Windows the native properties window is the Explorer property sheet,
// opened with SHObjectProperties. The sheet is modeless and relies on the
// calling thread's message loop and COM apartment, so the call is moved to
// the app's UI thread through UIThread (set by the app at start-up); a plain
// goroutine has neither, and the sheet could fail to appear.
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
	return onUIThread(func() error {
		r, _, _ := procSHObjectProperties.Call(0, shopFilePath, uintptr(unsafe.Pointer(p)), 0)
		if r == 0 {
			return errors.New("could not open the Windows properties window")
		}
		return nil
	})
}
