// Windows details of the comparison. NTFS and FAT ignore case in names, so CaseAuto matches "Readme.TXT" with
// "README.txt" the way Explorer and TC do. A file is hidden when it carries FILE_ATTRIBUTE_HIDDEN (dotfiles are not
// special on Windows), which matches what the panel hides when "show hidden files" is off.
package dircompare

import (
	"io/fs"
	"syscall"
)

const defaultCaseInsensitive = true

func isHidden(_ string, info fs.FileInfo) bool {
	if a, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return a.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
	}
	return false
}
