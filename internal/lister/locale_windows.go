//go:build windows

// Default code pages on Windows. Windows keeps two system code pages: the
// ANSI one used by GUI programs (GetACP) and the OEM one used by the console
// (GetOEMCP). Total Commander's A and S keys switch between exactly these
// two, so we ask the system instead of guessing from the language.

package lister

import "golang.org/x/sys/windows"

var procGetOEMCP = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetOEMCP")

// DefaultANSI returns the system's ANSI code page (GetACP).
func DefaultANSI() Encoding {
	if e, ok := EncodingForCodepage(int(windows.GetACP())); ok && e != UTF8 && !e.IsUTF16() {
		return e
	}
	return Win1252
}

// DefaultOEM returns the system's OEM (DOS console) code page (GetOEMCP).
func DefaultOEM() Encoding {
	if err := procGetOEMCP.Find(); err == nil {
		cp, _, _ := procGetOEMCP.Call()
		if e, ok := EncodingForCodepage(int(cp)); ok && e != UTF8 {
			return e
		}
	}
	return DOS437
}
