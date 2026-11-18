//go:build windows

package main

import "golang.org/x/sys/windows"

// setVolumeLabel changes the label of the drive whose root is root (`E:\`), for RenameVolume (F-713).
var setVolumeLabel = func(root, name string) error {
	r, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return err
	}
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	return windows.SetVolumeLabel(r, n)
}
