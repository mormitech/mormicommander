//go:build !windows

// Default code pages on macOS and Linux. These systems use UTF-8
// everywhere, so the "ANSI" and "ASCII" code pages only matter for old files
// that came from Windows or DOS machines. We guess them from the user's
// language setting in the environment.

package lister

import "os"

func localeString() string {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" && v != "C" && v != "POSIX" {
			return v
		}
	}
	return ""
}

// DefaultANSI returns the 8-bit code page used for the Lister's ANSI mode.
func DefaultANSI() Encoding {
	a, _ := codepagesForLocale(localeString())
	return a
}

// DefaultOEM returns the DOS code page used for the Lister's ASCII mode.
func DefaultOEM() Encoding {
	_, o := codepagesForLocale(localeString())
	return o
}
