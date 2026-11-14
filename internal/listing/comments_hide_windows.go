// This file marks descript.ion hidden on Windows after a save, the way Total Commander creates it: the comment file
// sits in every commented folder and would otherwise show up in Explorer and in panels that hide hidden files. The
// other attributes the file has are kept; a failure is ignored because the comments themselves were saved.

//go:build windows

package listing

import "syscall"

func hideCommentFile(path string) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	a, err := syscall.GetFileAttributes(p)
	if err != nil || a&syscall.FILE_ATTRIBUTE_HIDDEN != 0 {
		return
	}
	_ = syscall.SetFileAttributes(p, a|syscall.FILE_ATTRIBUTE_HIDDEN)
}
