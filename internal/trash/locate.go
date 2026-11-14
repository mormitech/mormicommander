package trash

// Finding an item in the trash after it was moved there, and putting it back (board issue #13, the undo journal). The
// system calls this package uses (Finder, NSFileManager, gio, trash-put, the Windows shell) do not say where they put
// an item, and on macOS the Finder script is left exactly as it is, so the place is found afterwards instead: a move
// to the trash is a rename on the same volume, so the item keeps its identity (device and inode on macOS and Linux,
// volume and file index on Windows). Before notes that identity while the item is still in its folder; Locate then
// looks through the trash folders of that volume for the entry that is the same file, checking only names that can
// belong to it (the name's stem, which every system keeps when it renames an item to avoid a clash: "x 2.txt" in the
// macOS Trash, "x.2.txt" from gio, "x_1.txt" from trash-cli), plus the "$R..." names of the Windows Recycle Bin. When
// that finds nothing, the record the trash keeps of the old place is asked: the freedesktop.org ".trashinfo" file on
// Linux (gio copies an item to the home trash when its own volume has no trash, which gives it a new identity), and
// the "$I..." file of the Recycle Bin.
//
// Restore moves an item back to its old place and removes that record, so the trash does not list it any more. It
// never overwrites: when something is at the old place again, it refuses with ErrOccupied, and it only takes items
// from the trash folders of the old place's volume, so a hand-edited journal can't make it move anything else.
//
// The trash folders come from trashDirs, which the tests replace with temporary folders, so they never read or change
// the real trash. Everything here is plain Go that works on every system (the .trashinfo and $I readers included,
// so their tests run everywhere); only the list of trash folders is per platform (locate_<os>.go).

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

// Mark is what Locate needs to recognise an item once it is in the trash: its old path, its identity, and when it was
// taken (a record of the old place written before that is about an older delete of the same name).
type Mark struct {
	path string
	info os.FileInfo
	at   time.Time
}

// Path is the item's old place.
func (m Mark) Path() string { return m.path }

// Before notes the identity of path just before it goes to the trash. ok is false when the item can't be read.
func Before(path string) (Mark, bool) {
	fi, err := os.Lstat(path)
	if err != nil {
		return Mark{}, false
	}
	// On Windows the file index behind os.SameFile is read lazily, by opening the path again; after the move that path
	// is gone, so it is read now, while the item is still there. Elsewhere this costs nothing.
	os.SameFile(fi, fi)
	return Mark{path: filepath.Clean(path), info: fi, at: time.Now()}, true
}

// trashDirs lists the folders that hold trashed items for an item whose old place is path: the folders in which the
// item itself lies once it is trashed (the macOS Trash or a volume's .Trashes/<uid>, a freedesktop.org "files"
// folder, a Recycle Bin folder of one user). The tests replace it.
var trashDirs = platformTrashDirs

// Locate says where the item noted by m is in the trash now. ok is false when it can't be found: the trash can't be
// read (on macOS the app may lack the permission to read the Trash), or the item is not there.
func Locate(m Mark) (string, bool) {
	if m.info == nil {
		return "", false
	}
	dirs := trashDirs(m.path)
	prefix := namePrefix(filepath.Base(m.path))
	for _, dir := range dirs {
		for _, n := range readNames(dir) {
			if !strings.HasPrefix(n, prefix) && !strings.HasPrefix(n, "$R") {
				continue
			}
			p := filepath.Join(dir, n)
			if fi, err := os.Lstat(p); err == nil && os.SameFile(fi, m.info) {
				return p, true
			}
		}
	}
	for _, dir := range dirs {
		if p, ok := byRecord(dir, m.path, m.at); ok {
			return p, true
		}
	}
	return "", false
}

// namePrefix is the part of a name that every trash keeps when it renames an item to avoid a clash: the name up to
// its first dot ("report" for "report.tar.gz"), or the whole name of a dot file.
func namePrefix(name string) string {
	if i := strings.IndexByte(name, '.'); i > 0 {
		return name[:i]
	}
	return name
}

// readNames lists the names in dir without looking at each entry; nil when it can't be read.
func readNames(dir string) []string {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()
	names, _ := f.Readdirnames(-1)
	return names
}

// recordSlack is how much older than the mark a record of the old place may say it is: the records keep whole
// seconds, and the clocks of the helper and of this program are read at slightly different moments.
const recordSlack = 2 * time.Second

// byRecord finds a trashed item by the record its trash keeps of the old place (.trashinfo or $I), taking the newest
// record of original written since the mark.
func byRecord(dir, original string, since time.Time) (string, bool) {
	since = since.Add(-recordSlack)
	best, bestAt := "", time.Time{}
	consider := func(item string, at time.Time) {
		if at.Before(since) || (best != "" && !at.After(bestAt)) {
			return
		}
		if _, err := os.Lstat(item); err == nil {
			best, bestAt = item, at
		}
	}
	if filepath.Base(dir) == "files" {
		info := filepath.Join(filepath.Dir(dir), "info")
		top := xdgTopDir(dir)
		for _, n := range readNames(info) {
			if !strings.HasSuffix(n, ".trashinfo") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(info, n))
			if err != nil {
				continue
			}
			p, at, err := parseTrashInfo(b, top)
			if err == nil && samePath(p, original) {
				consider(filepath.Join(dir, strings.TrimSuffix(n, ".trashinfo")), at)
			}
		}
	}
	for _, n := range readNames(dir) {
		if !strings.HasPrefix(n, "$I") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		p, at, err := parseRecycleInfo(b)
		if err == nil && samePath(p, original) {
			consider(filepath.Join(dir, "$R"+n[2:]), at)
		}
	}
	return best, best != ""
}

// samePath compares two paths the way the system does (without regard to case on Windows).
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// xdgHomeTrash is the "files" folder of the freedesktop.org home trash ($XDG_DATA_HOME/Trash/files, by default
// ~/.local/share/Trash/files). It is used on Linux; it lives here so its tests run everywhere.
func xdgHomeTrash() string {
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" || !filepath.IsAbs(data) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "Trash", "files")
}

// xdgTopDir is the folder the relative paths of a freedesktop.org trash are relative to: for a volume's
// "$top/.Trash/$uid/files" and "$top/.Trash-$uid/files" it is $top. The home trash keeps full paths, so its value
// does not matter.
func xdgTopDir(files string) string {
	root := filepath.Dir(files) // $top/.Trash/$uid or $top/.Trash-$uid
	if filepath.Base(filepath.Dir(root)) == ".Trash" {
		return filepath.Dir(filepath.Dir(root))
	}
	return filepath.Dir(root)
}

// parseTrashInfo reads a freedesktop.org .trashinfo file: the old place (Path, percent-encoded, relative to top when
// it is not a full path) and when the item was trashed (DeletionDate, local time).
func parseTrashInfo(b []byte, top string) (string, time.Time, error) {
	var path, date string
	inSection := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inSection = line == "[Trash Info]"
			continue
		}
		if !inSection {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "Path":
			path = strings.TrimSpace(v)
		case "DeletionDate":
			date = strings.TrimSpace(v)
		}
	}
	if path == "" {
		return "", time.Time{}, errors.New("no Path in the trash record")
	}
	p, err := url.PathUnescape(path)
	if err != nil {
		return "", time.Time{}, err
	}
	p = filepath.FromSlash(p)
	if !filepath.IsAbs(p) {
		p = filepath.Join(top, p)
	}
	at, err := time.ParseInLocation("2006-01-02T15:04:05", date, time.Local)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("bad DeletionDate %q", date)
	}
	return p, at, nil
}

// parseRecycleInfo reads the "$I..." file the Windows Recycle Bin keeps next to each trashed "$R..." item: the old
// place and when the item was recycled. Version 1 (Windows Vista to 8.1) has the path in a fixed field of 260 UTF-16
// characters; version 2 (Windows 10 and later) gives its length first. Both start with the version, the size and the
// deletion time as a FILETIME, all little-endian 64-bit numbers.
func parseRecycleInfo(b []byte) (string, time.Time, error) {
	if len(b) < 24 {
		return "", time.Time{}, errors.New("the Recycle Bin record is too short")
	}
	version := binary.LittleEndian.Uint64(b[0:8])
	at := fileTime(binary.LittleEndian.Uint64(b[16:24]))
	var chars []byte
	switch version {
	case 1:
		chars = b[24:min(len(b), 24+520)]
	case 2:
		if len(b) < 28 {
			return "", time.Time{}, errors.New("the Recycle Bin record is too short")
		}
		n := int(binary.LittleEndian.Uint32(b[24:28]))
		if n < 0 || 28+2*n > len(b) {
			return "", time.Time{}, errors.New("the Recycle Bin record is cut short")
		}
		chars = b[28 : 28+2*n]
	default:
		return "", time.Time{}, fmt.Errorf("unknown Recycle Bin record version %d", version)
	}
	u := make([]uint16, 0, len(chars)/2)
	for i := 0; i+1 < len(chars); i += 2 {
		c := binary.LittleEndian.Uint16(chars[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	if len(u) == 0 {
		return "", time.Time{}, errors.New("no path in the Recycle Bin record")
	}
	return string(utf16.Decode(u)), at, nil
}

// fileTime turns a Windows FILETIME (100-nanosecond steps since 1601) into a time.
func fileTime(ft uint64) time.Time {
	const unixEpoch = 116444736000000000 // 1970-01-01 as a FILETIME
	if ft < unixEpoch {
		return time.Time{}
	}
	d := ft - unixEpoch
	return time.Unix(int64(d/10000000), int64(d%10000000)*100)
}

// ErrOccupied is returned by Restore when something is at the item's old place again; nothing is moved then.
var ErrOccupied = errors.New("something else is at its old place now")

// ErrNotInTrash is returned by Restore for a path that is not in a trash folder of the old place's volume.
var ErrNotInTrash = errors.New("it is not in the trash of that disk")

// Restore puts the trashed item back at original, never over anything, and removes the trash's record of it. The
// folder that held it is made again when it is gone.
func Restore(trashed, original string) error {
	if !filepath.IsAbs(trashed) || !filepath.IsAbs(original) {
		return errors.New("only full paths can be put back")
	}
	trashed, original = filepath.Clean(trashed), filepath.Clean(original)
	if !inTrash(trashed, original) {
		return ErrNotInTrash
	}
	if _, err := os.Lstat(trashed); err != nil {
		return err
	}
	if _, err := os.Lstat(original); err == nil {
		return ErrOccupied
	}
	if err := os.MkdirAll(filepath.Dir(original), 0o777); err != nil {
		return err
	}
	if err := os.Rename(trashed, original); err != nil {
		return err
	}
	if rec := recordOf(trashed); rec != "" {
		// The item is back either way; a record left behind only shows a stale line in the trash.
		os.Remove(rec)
	}
	return nil
}

// inTrash says whether trashed lies directly in one of the trash folders for original.
func inTrash(trashed, original string) bool {
	dir := filepath.Dir(trashed)
	for _, d := range trashDirs(original) {
		if samePath(d, dir) {
			return true
		}
	}
	return false
}

// recordOf is the record a trash keeps of a trashed item: "../info/<name>.trashinfo" for a freedesktop.org trash, the
// "$I..." file next to a "$R..." item of the Recycle Bin, "" for the macOS Trash (Finder keeps its "Put Back" notes in
// the Trash's .DS_Store, which is left alone).
func recordOf(trashed string) string {
	dir, name := filepath.Dir(trashed), filepath.Base(trashed)
	switch {
	case filepath.Base(dir) == "files":
		return filepath.Join(filepath.Dir(dir), "info", name+".trashinfo")
	case strings.HasPrefix(name, "$R"):
		return filepath.Join(dir, "$I"+name[2:])
	}
	return ""
}
