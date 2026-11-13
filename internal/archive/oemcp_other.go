//go:build !windows

// macOS and Linux have no OEM code page, so we guess it from the user's
// locale: a Hungarian user most likely receives old zips made on Hungarian
// Windows machines (CP852), a Russian user Russian ones (CP866), and so on.
// The locale comes from LC_ALL, LC_CTYPE or LANG; when none is set (GUI apps
// on macOS often start without them) or the language is unknown, we fall back
// to CP437, the code page the zip specification prescribes.

package archive

import "os"

// DefaultOEMEncoding returns the encoding name the "auto" name encoding
// uses for non-UTF-8 archives on this computer.
func DefaultOEMEncoding() string {
	for _, v := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if s := os.Getenv(v); s != "" && s != "C" && s != "POSIX" {
			return oemForLanguage(s)
		}
	}
	return "cp437"
}
