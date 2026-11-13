// Package listing turns a raw directory listing into the rows a file panel shows. It is UI-free, so it builds, tests and
// cross-compiles on its own, and the Wails layer only has to hand it entries and draw the strings it returns.
//
// This file defines Item, the one record every other part of the package works on. A panel gets its entries from many
// places (the local disk, archives, FTP, SFTP), and each source knows a different subset of facts about a file, so
// Item carries every field a Total Commander column can show (three dates, attributes, owner, link target, comment,
// computed folder size) and leaves the unknown ones at their zero value. FromFileInfo fills an Item from the local file
// system, with the platform-specific parts (creation and access time, owner, Windows attribute bits) living in the
// stat_*.go files.
package listing

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Attr is a set of attribute bits. It merges the Windows attribute word with the Unix facts a panel cares about, so
// one attribute string formatter and one filter can serve every platform.
type Attr uint32

// Attribute bits. The first group mirrors FILE_ATTRIBUTE_* on Windows; the second group is derived from the file type
// and permission bits everywhere.
const (
	AttrReadOnly   Attr = 1 << iota // Windows read-only, or no write bit for anyone on Unix
	AttrHidden                      // Windows hidden, macOS UF_HIDDEN, or a dotfile on Unix
	AttrSystem                      // Windows system
	AttrArchive                     // Windows archive ("needs backup")
	AttrCompressed                  // NTFS compressed, or macOS UF_COMPRESSED
	AttrEncrypted                   // NTFS encrypted (EFS)
	AttrTemporary                   // Windows temporary
	AttrOffline                     // Windows offline / cloud placeholder not on disk
	AttrSparse                      // NTFS sparse file
	AttrDir                         // a folder (or a link to one)
	AttrLink                        // a symbolic link, junction or other reparse point
	AttrBroken                      // a link whose target does not exist
	AttrExec                        // runnable: a Unix x bit, or an executable extension on Windows
)

// Has reports whether every bit of b is set.
func (a Attr) Has(b Attr) bool { return a&b == b }

// Item is one entry of a folder as the listing engine sees it. Times are zero when the source does not know them, and
// DirSize is -1 for a folder whose size has not been calculated yet (TC shows <DIR> until Space or Alt+Shift+Enter).
type Item struct {
	Name       string         // name in the folder, never a path; ".." for the parent entry
	Size       int64          // bytes for files; ignored for folders (see DirSize)
	DirSize    int64          // computed folder size in bytes, or -1 when not calculated
	ModTime    time.Time      // last write ("writedate")
	CreateTime time.Time      // creation / birth time ("creationdate"), zero where the OS does not record it
	AccessTime time.Time      // last access ("accessdate")
	Attr       Attr           // attribute bits
	Mode       fs.FileMode    // Unix type and permission bits; on Windows only the read-only and dir bits mean much
	Owner      string         // owner user name (Unix) or empty
	Group      string         // owning group name (Unix) or empty
	LinkTarget string         // where a symbolic link points, as stored in the link
	Comment    string         // descript.ion comment, filled by ApplyComments
	Dir        string         // the folder the item lives in; used by the tc.path and tc.fullpath fields
	Extra      map[string]any // values from field providers (image size, audio tags); read by registered fields
	Order      int            // position in the raw listing, used for "unsorted" and as the last tie breaker
}

// IsDir reports whether the item is a folder or a link to one.
func (it *Item) IsDir() bool { return it.Attr&AttrDir != 0 }

// IsUp reports whether the item is the synthetic ".." entry.
func (it *Item) IsUp() bool { return it.Name == ".." }

// UpItem is the ".." entry TC shows at the top of every folder except a root.
func UpItem(dir string) Item {
	return Item{Name: "..", Attr: AttrDir, Mode: fs.ModeDir | 0o755, DirSize: -1, Dir: dir, Order: -1}
}

// SplitExt splits a name into base and extension the way TC does: the extension is the text after the last dot, and a
// name whose only dot is the first character (".bashrc") has no extension, which matches how Unix users read
// dotfiles. Folders are split too, because TC shows "dir.d" with the extension column filled when the "show
// extension of folders" option is on; callers that do not want that use SplitExtFor.
func SplitExt(name string) (base, ext string) {
	if name == ".." || name == "." {
		return name, ""
	}
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || i == len(name)-1 {
		return name, ""
	}
	return name[:i], name[i+1:]
}

// SplitExtFor splits the item's name, keeping folder names whole unless dirExt is set.
func SplitExtFor(it *Item, dirExt bool) (base, ext string) {
	if it.IsDir() && !dirExt {
		return it.Name, ""
	}
	return SplitExt(it.Name)
}

// winExecExts are the extensions Windows runs directly; TC's "programs only" filter (Ctrl+F11) uses this list.
var winExecExts = map[string]bool{"exe": true, "com": true, "bat": true, "cmd": true, "pif": true}

// IsWindowsExecName reports whether a name has an extension Windows treats as a program.
func IsWindowsExecName(name string) bool {
	_, ext := SplitExt(name)
	return winExecExts[strings.ToLower(ext)]
}

// FromFileInfo builds an Item for the entry named fi.Name() in dir. It does not follow a link for the item's own
// facts (a link is listed as itself), but it does stat the target so a link to a folder sorts and opens as a folder
// and a broken link can be marked.
func FromFileInfo(dir string, fi fs.FileInfo) Item {
	it := Item{
		Name:    fi.Name(),
		Size:    fi.Size(),
		DirSize: -1,
		ModTime: fi.ModTime(),
		Mode:    fi.Mode(),
		Dir:     dir,
	}
	m := fi.Mode()
	if m.IsDir() {
		it.Attr |= AttrDir
		it.Size = 0
	}
	if m&fs.ModeSymlink != 0 {
		it.Attr |= AttrLink
		full := filepath.Join(dir, fi.Name())
		if t, err := os.Readlink(full); err == nil {
			it.LinkTarget = t
		}
		if ti, err := os.Stat(full); err != nil {
			it.Attr |= AttrBroken
		} else if ti.IsDir() {
			it.Attr |= AttrDir
			it.Size = 0
		}
	}
	if m.Perm()&0o222 == 0 {
		it.Attr |= AttrReadOnly
	}
	if m.IsRegular() && m.Perm()&0o111 != 0 {
		it.Attr |= AttrExec
	}
	if strings.HasPrefix(it.Name, ".") && it.Name != ".." {
		it.Attr |= AttrHidden
	}
	fillPlatform(&it, fi)
	return it
}

// ReadDir lists dir from the local disk into Items in file system order (Order is set to the position), which is what
// the "unsorted" sort mode shows. It deliberately avoids os.ReadDir, which sorts by name. Entries that vanish between
// the directory read and the stat are skipped.
func ReadDir(dir string) ([]Item, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	des, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(des))
	for _, de := range des {
		fi, err := de.Info()
		if err != nil {
			continue
		}
		it := FromFileInfo(dir, fi)
		it.Order = len(items)
		items = append(items, it)
	}
	return items, nil
}
