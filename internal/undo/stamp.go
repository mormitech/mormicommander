package undo

import (
	"io/fs"
	"path/filepath"
)

// stampLimit is the most entries TreeStamp walks; a bigger copied folder gets no stamp and is undone as before.
const stampLimit = 200000

// TreeStamp describes a copied folder right after the copy: how many entries it holds (itself included) and the
// newest modification time among them (Unix milliseconds). Adding, removing or changing anything inside changes one of
// the two (a folder's own time changes when an entry is added or removed), so an undo can tell that the folder holds
// more than the copy and leave it alone. ok is false when the folder can't be walked or is too big.
func TreeStamp(dir string) (count, newest int64, ok bool) {
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		if count > stampLimit {
			return filepath.SkipAll
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if m := fi.ModTime().UnixMilli(); m > newest {
			newest = m
		}
		return nil
	})
	return count, newest, err == nil && count > 0 && count <= stampLimit
}
