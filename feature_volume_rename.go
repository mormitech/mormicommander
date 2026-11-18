package main

// Volume label edit (F-713, Commands > Rename volume…, cm_RenameVolume): Total Commander's "Volume label" dialog changes
// the label of a drive. Here it is a command of its own next to Volume information, and it only renames a volume the
// drive list shows, by its exact mount point:
//
//   - macOS: diskutil rename <mount point> <name> (the mount point, never a disk identifier typed by hand). The startup
//     disk at "/" is left to Finder.
//   - Windows: SetVolumeLabel on the drive's root (volrename_windows.go).
//   - Linux: not offered (each file system has its own label tool, most need root and an unmounted volume).
//
// Network shares, optical discs and the home folder are never renamed. The page asks first, naming the volume and
// where it is mounted; nothing here is reachable from the AI tools.

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"mormicommander/internal/disk"
)

// volumeLabelMax is the longest label accepted: NTFS allows 32 characters, macOS volume names are longer, but a label
// that long is never what anyone wants on a drive button.
const volumeLabelMax = 32

// renameVolumeVolumes lists the volumes a rename may aim at (a variable so tests never see the real disks).
var renameVolumeVolumes = disk.Volumes

// renameVolumeRun runs a program and returns its combined output (a variable so tests never run diskutil).
var renameVolumeRun = func(name string, args []string) (string, error) {
	cmd := exec.Command(name, args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// checkVolumeLabel says what is wrong with a new volume label ("" when it can be used).
func checkVolumeLabel(name string) string {
	switch {
	case strings.TrimSpace(name) == "":
		return "The new name is empty."
	case utf8.RuneCountInString(name) > volumeLabelMax:
		return fmt.Sprintf("The new name is too long: at most %d characters.", volumeLabelMax)
	case strings.ContainsAny(name, `/\:*?"<>|`):
		return `The new name can't contain / \ : * ? " < > or |.`
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "The new name can't contain control characters."
		}
	}
	return ""
}

// renamableVolume finds the volume mounted exactly at path that may be renamed on goos, or says why not.
func renamableVolume(goos, path string, vols []disk.Volume) (disk.Volume, error) {
	if goos != "darwin" && goos != "windows" {
		return disk.Volume{}, errors.New("This system doesn't let mormiCommander change volume names. Use the disk tool of your desktop or the file system's own label tool.")
	}
	for _, v := range vols {
		if !strings.EqualFold(filepath.Clean(v.Path), filepath.Clean(path)) {
			continue
		}
		switch {
		case v.Stale:
			return v, fmt.Errorf("%s is not responding, so it can't be renamed now.", v.Name)
		case v.Kind != "fixed" && v.Kind != "removable":
			return v, fmt.Errorf("%s can't be renamed here: only disks and removable drives can.", v.Name)
		case goos == "darwin" && v.Path == "/":
			return v, errors.New("The startup disk is renamed in Finder, not here.")
		}
		return v, nil
	}
	return disk.Volume{}, fmt.Errorf("%s is not a volume in the drive list.", path)
}

// renameVolumeCommand is the program that renames the volume mounted at path on goos ("" when it is not done by a
// program). The mount point and the name always travel as arguments of their own.
func renameVolumeCommand(goos, path, name string) (string, []string) {
	if goos == "darwin" {
		return "diskutil", []string{"rename", path, name}
	}
	return "", nil
}

// RenameVolume changes the name (label) of the volume mounted at path to name.
func (f *SystemFiles) RenameVolume(path, name string) error {
	if f.app.mock != nil {
		return errors.New("This is the demo: its volumes are not real, so they can't be renamed.")
	}
	name = strings.TrimSpace(name)
	if msg := checkVolumeLabel(name); msg != "" {
		return errors.New(msg)
	}
	v, err := renamableVolume(runtime.GOOS, path, renameVolumeVolumes())
	if err != nil {
		return err
	}
	if prog, args := renameVolumeCommand(runtime.GOOS, v.Path, name); prog != "" {
		if out, err := renameVolumeRun(prog, args); err != nil {
			if out == "" {
				out = err.Error()
			}
			return fmt.Errorf("Can't rename %s: %s", v.Name, strings.TrimSuffix(out, "."))
		}
		return nil
	}
	if setVolumeLabel == nil {
		return errors.New("This system doesn't let mormiCommander change volume names.")
	}
	if err := setVolumeLabel(v.Path, name); err != nil {
		return fmt.Errorf("Can't rename %s: %v. Changing the label of some drives needs administrator rights.", v.Name, err)
	}
	return nil
}
