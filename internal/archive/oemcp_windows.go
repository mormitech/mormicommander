// On Windows the right code page for legacy archive names is simply the
// system OEM code page, which GetOEMCP in kernel32 reports. It is the same
// value the old DOS-era zip tools used when they wrote the archive on a
// machine with the same language settings.

package archive

import "syscall"

var procGetOEMCP = syscall.NewLazyDLL("kernel32.dll").NewProc("GetOEMCP")

// DefaultOEMEncoding returns the encoding name for this computer's OEM code
// page, used by the "auto" name encoding for non-UTF-8 archives.
func DefaultOEMEncoding() string {
	if err := procGetOEMCP.Find(); err != nil {
		return "cp437"
	}
	r, _, _ := procGetOEMCP.Call()
	name := oemForCodePage(uint32(r))
	if name == "utf-8" {
		return "cp437"
	}
	return name
}
