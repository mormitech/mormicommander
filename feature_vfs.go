package main

// Archives and remote folders inside the panels, the FS side (FEATURES.md section 29): what kind of place a panel
// path is (local, archive, FTP, SFTP), whether a file opens as an archive (Enter by extension, Ctrl+PgDn by content),
// temporary local copies of files inside archives and on servers (Enter, F3), and the password question an encrypted
// archive asks. The question is a page dialog: Go sends a "vfs-password" event and waits for FS.AnswerPassword, so
// control mode sees and answers it like any other dialog.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/archive"
	"mormicommander/internal/archive/arcapi"
	"mormicommander/internal/archive/sevenrar"
	"mormicommander/internal/vfs"
)

// vfsPathKind names the kind of place p is: "local", "archive" (the archive file itself counts when a panel shows
// it), "ftp", "sftp", "other" or "unknown".
func vfsPathKind(r *vfs.Router, p string) string {
	if r == nil {
		return "local"
	}
	k := r.Kind(p)
	if k == vfs.KindLocal {
		_, c := r.Resolve(p)
		if fi, err := os.Stat(c); err == nil && fi.Mode().IsRegular() {
			return "archive"
		}
		return "local"
	}
	return k.String()
}

// vfsIsLocal says whether the file or folder p is on the local disk (an archive file itself is; its entries are not).
// For a folder a panel shows, use vfsPathKind, which counts an archive file as the archive's root.
func vfsIsLocal(r *vfs.Router, p string) bool { return r == nil || r.Kind(p) == vfs.KindLocal }

// PathInfo describes the place a panel shows, for its path bar badge and the commands that depend on it.
type PathInfo struct {
	Kind     string `json:"kind"`     // local, archive, ftp, sftp, results (find://), other, unknown
	Format   string `json:"format"`   // archive format ("zip", "tar.gz", "7z", ...) inside an archive
	Archive  string `json:"archive"`  // the panel path of the archive itself
	Inner    string `json:"inner"`    // the folder inside the archive ("" at its root)
	ReadOnly bool   `json:"readOnly"` // nothing can be written here
	Remote   bool   `json:"remote"`   // on a server (also an archive downloaded from one)
	Nested   bool   `json:"nested"`   // an archive inside another archive or on a server
	// ReadOnlyWhy is the sentence that tells why an archive folder can't be changed ("" when it can), shown before F5,
	// F6, F7 or F8 would change it (board #143)
	ReadOnlyWhy string `json:"readOnlyWhy,omitempty"`
}

// PathInfo tells what kind of place path is: a local folder, the inside of an archive, a server folder.
func (f *FS) PathInfo(path string) PathInfo {
	if f.router == nil {
		return PathInfo{Kind: "local"}
	}
	if findFeedPath(path) {
		// Feed to listbox: the names stand for real files, which every file command works on (not a server, not read-only)
		return PathInfo{Kind: "results"}
	}
	if branchPath(path) {
		// Branch view (feature_branch.go): every name is a real file on this computer, so it is a local place for the
		// commands that ask (quick view, F3, the copy target); the branch feature marks the path bar itself
		return PathInfo{Kind: "local"}
	}
	if networkPath(path) {
		// the Network panel's servers and shares (feature_network_fs.go): nothing can be copied there; a mounted
		// share is a local folder and gets here as one
		return PathInfo{Kind: "network", ReadOnly: true, ReadOnlyWhy: "Servers and shares can't be changed here; open a share first."}
	}
	kind := vfsPathKind(f.router, path)
	info := PathInfo{Kind: kind, Remote: kind == "ftp" || kind == "sftp" || strings.Contains(path, "://")}
	if kind == "archive" {
		// FolderInfo: an archive entered from inside another one is that inner archive here, not a file of the outer one
		if ai, err := f.router.Archives.FolderInfo(f.router.Clean(path)); err == nil {
			info.Format, info.Archive, info.Inner, info.ReadOnly, info.Nested = ai.Format, ai.Root, ai.Inner, ai.ReadOnly, ai.Nested
			info.ReadOnlyWhy = vfsArchiveReadOnly(f.router, path, true)
		}
		return info
	}
	if kind != "local" {
		c := f.router.Caps(path)
		info.ReadOnly = !c.Write
	}
	return info
}

// vfsArchiveReadOnly says why nothing can be changed at p inside an archive, as a sentence, or "" when it can be
// changed (or p is not in an archive). With folder, p is a folder a panel shows (a copy target, where F7 makes a
// folder), so an archive entered from inside another one counts as that inner archive; otherwise p is an entry to
// delete, move or rename, which belongs to the archive that holds it (TC: "This archive type cannot be modified").
func vfsArchiveReadOnly(r *vfs.Router, p string, folder bool) string {
	if r == nil || vfsPathKind(r, p) != "archive" {
		return ""
	}
	p = r.Clean(p)
	var ai vfs.ArchiveInfo
	var err error
	if folder {
		ai, err = r.Archives.FolderInfo(p)
	} else {
		ai, err = r.Archives.Info(p)
	}
	if err != nil || !ai.ReadOnly {
		return ""
	}
	name := vfsBase(ai.Root)
	if ai.Nested {
		return fmt.Sprintf("%s is inside another archive (or on a server), so it can't be changed here. Unpack it, change the files and pack it again.", name)
	}
	return fmt.Sprintf("%s can't be changed: this archive type (%s) is read-only here. Unpack it, or pack a new archive with Alt+F5.",
		name, strings.ToUpper(ai.Format))
}

// IsArchive says whether Enter opens a file of this name as a folder (by its extension, like TC's packers).
func (f *FS) IsArchive(name string) bool {
	if f.router == nil {
		return false
	}
	return f.router.Archives.IsArchive(name)
}

// IsArchiveFile says whether a file can be opened as an archive by its content (Ctrl+PgDn: .jar, .docx, renamed zips).
func (f *FS) IsArchiveFile(path string) bool {
	if f.router == nil {
		return false
	}
	if !vfsIsLocal(f.router, path) {
		return f.router.Archives.IsArchive(filepath.Base(path))
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	return vfsEngineOf(path) != ""
}

var vfsTemp struct {
	mu   sync.Mutex
	dirs []string
}

func init() {
	onShutdown(func(*App) {
		vfsTemp.mu.Lock()
		defer vfsTemp.mu.Unlock()
		for _, d := range vfsTemp.dirs {
			os.RemoveAll(d)
		}
		vfsTemp.dirs = nil
	})
}

// vfsTempDir makes a temporary folder that is removed when the app ends.
func vfsTempDir(prefix string) (string, error) {
	base := ""
	if s, ok := currentSettings(); ok {
		if t := strings.TrimSpace(s.Zip.TempDir); t != "" {
			if fi, err := os.Stat(t); err == nil && fi.IsDir() {
				base = t // (Options > Zip > Temporary folder)
			}
		}
	}
	d, err := os.MkdirTemp(base, "mormicommander-"+prefix+"-")
	if err != nil {
		return "", fmt.Errorf("Can't make a temporary folder: %v", err)
	}
	vfsTemp.mu.Lock()
	vfsTemp.dirs = append(vfsTemp.dirs, d)
	vfsTemp.mu.Unlock()
	return d, nil
}

// LocalCopy gives a local path for a file: the path itself for a local file, else a temporary copy unpacked from the
// archive or downloaded from the server (Enter and F3 inside archives). The copy is removed when the app ends.
func (f *FS) LocalCopy(path string) (string, error) {
	return vfsLocalCopy(f.router, path)
}

// vfsLocalCopy is LocalCopy for any caller that has the router (the Lister opens server files through it, issue #101).
func vfsLocalCopy(r *vfs.Router, path string) (string, error) {
	if r == nil || vfsIsLocal(r, path) {
		return path, nil
	}
	e, err := r.Stat(path)
	if err != nil {
		return "", errors.New(listError(r.Clean(path), err))
	}
	if e.Flags&vfs.FlagDir != 0 {
		return "", fmt.Errorf("%s is a folder, not a file.", e.Name)
	}
	dir, err := vfsTempDir("view")
	if err != nil {
		return "", err
	}
	local := filepath.Join(dir, archive.SanitizeName(e.Name))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if _, err := r.Download(ctx, r.Clean(path), local, vfs.TransferOptions{PreserveTime: true}); err != nil {
		return "", fmt.Errorf("Can't unpack %s: %s", e.Name, vfsErrText(err))
	}
	return local, nil
}

// Mkdir makes a folder anywhere a panel can be: on the disk, inside a writable archive, on a server.
func (f *FS) Mkdir(path string) error {
	if f.router == nil {
		return errors.New("This is the demo: its files are not real.")
	}
	if err := vfs.Mkdir(context.Background(), f.router, f.router.Clean(path)); err != nil {
		return fmt.Errorf("Can't make the folder %s: %s", filepath.Base(path), vfsErrText(err))
	}
	return nil
}

// Rename renames or moves a file or folder within one place (a folder, one archive, one server).
func (f *FS) Rename(from, to string) error {
	if f.router == nil {
		return errors.New("This is the demo: its files are not real.")
	}
	if err := vfs.Rename(context.Background(), f.router, f.router.Clean(from), f.router.Clean(to)); err != nil {
		return fmt.Errorf("Can't rename %s: %s", filepath.Base(from), vfsErrText(err))
	}
	return nil
}

// vfsErrText is an engine or file system error as a short plain sentence fragment.
func vfsErrText(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled), errors.Is(err, archive.ErrCanceled), errors.Is(err, sevenrar.ErrPasswordCancelled):
		return "stopped."
	case errors.Is(err, archive.ErrBadPassword), errors.Is(err, sevenrar.ErrWrongPassword):
		return "the password is wrong."
	case errors.Is(err, archive.ErrPasswordRequired), errors.Is(err, sevenrar.ErrPasswordRequired):
		return "a password is needed."
	case errors.Is(err, archive.ErrChecksum), errors.Is(err, sevenrar.ErrCRC):
		return "checksum error: the data is damaged (or the password is wrong)."
	case errors.Is(err, archive.ErrMissingVolume), errors.Is(err, sevenrar.ErrMissingVolume):
		return "a part of the multi-volume archive is missing."
	case errors.Is(err, vfs.ErrNestedReadOnly):
		return "this archive is inside another archive (or on a server), so it can't be changed here."
	case errors.Is(err, archive.ErrReadOnly), errors.Is(err, vfs.ErrReadOnly), errors.Is(err, arcapi.ErrNotSupported):
		return "this archive can't be changed (its format is read-only here)."
	case errors.Is(err, archive.ErrFormat):
		return "not a valid archive, or it is damaged."
	case errors.Is(err, archive.ErrUnsupportedMethod):
		return "the archive uses a compression method mormiCommander can't unpack."
	case errors.Is(err, vfs.ErrUnsupported):
		return "this can't be done here."
	case errors.Is(err, os.ErrExist):
		return "something with this name is already there."
	case errors.Is(err, os.ErrNotExist):
		return "it is not there."
	case errors.Is(err, os.ErrPermission):
		return "permission denied."
	}
	t := err.Error()
	for _, p := range []string{"archive: ", "sevenrar: ", "vfs: ", "tarfmt: "} {
		t = strings.ReplaceAll(t, p, "")
	}
	return t
}

// ---- password question ----

type vfsPwAnswer struct {
	pw string
	ok bool
}

var vfsPw struct {
	mu      sync.Mutex
	seq     int
	waiting map[int]chan vfsPwAnswer
	ask     sync.Mutex // one question at a time
}

// vfsPasswordQuestion is the "vfs-password" event: the page shows a password dialog and answers with AnswerPassword.
type vfsPasswordQuestion struct {
	ID      int    `json:"id"`
	Archive string `json:"archive"`
	Entry   string `json:"entry"`
	Attempt int    `json:"attempt"`
}

// vfsAskPassword asks the page for an archive's password; ok=false when the user cancelled (or there is no window).
func vfsAskPassword(a *App, arc, entry string, attempt int) (string, bool) {
	if a == nil || a.ctx == nil {
		return "", false
	}
	vfsPw.ask.Lock()
	defer vfsPw.ask.Unlock()
	vfsPw.mu.Lock()
	if vfsPw.waiting == nil {
		vfsPw.waiting = map[int]chan vfsPwAnswer{}
	}
	vfsPw.seq++
	id := vfsPw.seq
	ch := make(chan vfsPwAnswer, 1)
	vfsPw.waiting[id] = ch
	vfsPw.mu.Unlock()
	a.emit("vfs-password", vfsPasswordQuestion{ID: id, Archive: arc, Entry: entry, Attempt: attempt})
	ans := <-ch
	return ans.pw, ans.ok
}

// AnswerPassword answers the password question id (ok=false: the user cancelled).
func (f *FS) AnswerPassword(id int, password string, ok bool) {
	vfsPw.mu.Lock()
	ch := vfsPw.waiting[id]
	delete(vfsPw.waiting, id)
	vfsPw.mu.Unlock()
	if ch != nil {
		ch <- vfsPwAnswer{password, ok}
	}
}
