package main

// Putting back items picked in a trash folder (F-501, cm_RestoreFromTrash) and the folder cm_GoToTrash opens. Where an
// item came from is read from the trash's own record (.trashinfo on Linux, $I on Windows, internal/trash Original);
// the macOS Trash keeps none this app can read, so there the undo journal is asked for an item this app trashed. The
// move itself is trash.Restore: never over anything, and only out of a trash folder of the old place's volume. These
// are methods of Jobs, so the page reaches them as go.Jobs.*; they are not in the AI tool allow-list.

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"mormicommander/internal/trash"
	"mormicommander/internal/undo"
)

// TrashItem is one item picked in a trash folder: where it is, and where it came from ("" when that is not known).
type TrashItem struct {
	Path     string `json:"path"`
	Original string `json:"original"`
	Error    string `json:"error,omitempty"` // why it was not put back (RestoreFromTrash)
}

// TrashFolder is the trash folder of the home volume ("" when the system has none the app knows).
func (j *Jobs) TrashFolder() string { return trash.Folder() }

// TrashOriginals says where each trashed item came from.
func (j *Jobs) TrashOriginals(paths []string) []TrashItem {
	out := make([]TrashItem, 0, len(paths))
	for _, p := range paths {
		it := TrashItem{Path: p}
		if o, err := j.trashOriginal(p); err == nil {
			it.Original = o
		} else {
			it.Error = err.Error()
		}
		out = append(out, it)
	}
	return out
}

// RestoreFromTrash puts each trashed item back where it came from (worked out again here, never taken from the
// page). An item whose old place is taken again stays in the trash.
func (j *Jobs) RestoreFromTrash(paths []string) []TrashItem {
	out := make([]TrashItem, 0, len(paths))
	for _, p := range paths {
		it := TrashItem{Path: p}
		o, err := j.trashOriginal(p)
		if err == nil {
			it.Original = o
			if err = trash.Restore(p, o); err == nil {
				opLog(j.app, "delete", "Put back from the trash: "+o)
			}
		}
		if err != nil {
			it.Error = err.Error()
		}
		out = append(out, it)
	}
	return out
}

func (j *Jobs) trashOriginal(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", errors.New("only full paths can be put back")
	}
	o, err := trash.Original(p)
	if err == nil {
		return o, nil
	}
	var jr *undo.Journal
	if j.app != nil && j.app.undo != nil {
		jr = j.app.undo.j
	}
	if o, ok := journalOriginal(jr, p); ok {
		return o, nil
	}
	return "", err
}

// journalOriginal finds the old place of a trashed item in the undo journal (an item this app moved to the trash).
func journalOriginal(jr *undo.Journal, trashed string) (string, bool) {
	if jr == nil {
		return "", false
	}
	trashed = filepath.Clean(trashed)
	list := jr.List()
	for i := len(list) - 1; i >= 0; i-- { // newest first: the same trash name may have been used before
		if list[i].Kind != undo.Trash {
			continue
		}
		for _, it := range list[i].Items {
			if it.To != "" && sameTrashPath(it.To, trashed) && filepath.IsAbs(it.From) {
				return filepath.Clean(it.From), true
			}
		}
	}
	return "", false
}

// sameTrashPath compares two paths the way the system does (without regard to case on Windows).
func sameTrashPath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
