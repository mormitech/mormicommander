package main

// The instant file operations of F5–F8 that are not queued jobs: reading what the user typed as the target of a copy
// or move (Total Commander's rules: a folder, a new name, or a rename mask, relative to the source folder), making a
// folder (F7, nested "a/b/c" and date placeholders), renaming in place (Shift+F6), and whether a trash exists. They are
// methods of Jobs (feature_jobs.go), so the page finds the whole area under window.go.main.Jobs. Local paths go
// straight to the disk; paths of a registered file system (ftp://, archives once their feature registers them) go
// through the vfs helpers, which say plainly when that file system can't do it.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mormicommander/internal/fileops"
	"mormicommander/internal/undo"
	"mormicommander/internal/vfs"
)

// FileopsTarget is the copy/move dialog's target, read the way Total Commander reads it.
type FileopsTarget struct {
	Dir        string `json:"dir"`        // the folder the items go into
	Rename     string `json:"rename"`     // a new name (one item) or a mask like *.bak; "" keeps the names
	DirExists  bool   `json:"dirExists"`  // false: the folder will have to be made
	SameVolume bool   `json:"sameVolume"` // the folder is on the same volume as from (a move is then a rename)
}

// ParseTarget reads the target typed in the copy/move dialog for the given number of items. A relative path is taken
// from the folder from (the source panel), "~" is the home folder.
func (j *Jobs) ParseTarget(input string, sources int, from string) (FileopsTarget, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return FileopsTarget{}, errors.New("Type the folder to copy to.")
	}
	r := vfsRouter(j.app)
	in = fileopsExpandHome(in)
	if real, ok := branchReal(in); ok {
		// the other panel shows a branch view (feature_branch.go): its files are real, so is the folder it lists
		in = real + fileopsTrailingSep(in)
	}
	if !strings.Contains(in, "://") && !filepath.IsAbs(in) && from != "" {
		if strings.Contains(from, "://") {
			if r == nil {
				return FileopsTarget{}, fmt.Errorf("%s is not a full path.", input)
			}
			in = r.Join(from, in) + fileopsTrailingSep(in)
		} else {
			in = filepath.Join(from, in) + fileopsTrailingSep(in)
		}
	}
	if strings.Contains(in, "://") || fileopsArchiveTarget(r, in) {
		// a server folder or a folder inside an archive: the vfs router says what is there (VfsJobs or the network
		// transfers do the copy)
		return fileopsParseVFS(r, in, sources, from)
	}
	if !filepath.IsAbs(in) {
		return FileopsTarget{}, fmt.Errorf("%s is not a full path.", input)
	}
	dir, rename := fileops.ParseTarget(in, sources)
	t := FileopsTarget{Dir: dir, Rename: rename}
	// the folder, or the nearest part of it that exists, must be a folder
	for p := dir; ; p = filepath.Dir(p) {
		if fi, err := os.Stat(p); err == nil {
			if !fi.IsDir() {
				return t, fmt.Errorf("%s is a file, not a folder.", p)
			}
			t.DirExists = p == dir
			break
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	if from != "" {
		t.SameVolume = fileops.SameVolume(from, dir)
	}
	return t, nil
}

// fileopsArchiveTarget says whether the typed target is in an archive: a path below an archive file, or the archive
// file itself written as a folder (with a separator at the end, as the copy dialog fills it in for a panel that shows
// the archive's root).
func fileopsArchiveTarget(r *vfs.Router, in string) bool {
	if r == nil || in == "" {
		return false
	}
	if r.Kind(in) == vfs.KindArchive {
		return true
	}
	if fileopsTrailingSep(in) == "" {
		return false
	}
	return fileopsIsArchiveFile(r, r.Clean(in))
}

// fileopsIsArchiveFile says whether p is a local file a panel can open as an archive.
func fileopsIsArchiveFile(r *vfs.Router, p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && (r.Archives.IsArchive(filepath.Base(p)) || vfsEngineOf(p) != "")
}

// fileopsParseVFS reads a copy/move target on a server or in an archive with TC's rules (see fileops.ParseTarget).
func fileopsParseVFS(r *vfs.Router, in string, sources int, from string) (FileopsTarget, error) {
	var fsys vfs.FS = r
	if r == nil {
		fsys = vfs.For(in, nil)
	}
	if fsys == nil {
		return FileopsTarget{}, fmt.Errorf("%s can't be a target: nothing here serves that kind of address.", in)
	}
	isDir := func(p string) (exists, dir bool) {
		if r != nil && !strings.Contains(p, "://") && r.Kind(p) == vfs.KindLocal {
			// the archive file itself: its root
			if fi, err := os.Stat(p); err == nil {
				return true, fi.IsDir() || fileopsIsArchiveFile(r, p)
			}
			return false, false
		}
		e, err := fsys.Stat(p)
		if err != nil {
			return false, false
		}
		if e.Flags&vfs.FlagDir == 0 && r != nil && r.Kind(p) == vfs.KindArchive {
			// an archive inside an archive that a panel has entered: its root, as the panel shows it (board #143)
			if ai, err := r.Archives.FolderInfo(p); err == nil && ai.Nested && ai.Inner == "" && ai.Root == p {
				return true, true
			}
		}
		return true, e.Flags&vfs.FlagDir != 0
	}
	t := FileopsTarget{}
	clean := fsys.Clean(in)
	if ex, d := isDir(clean); fileopsTrailingSep(in) != "" || (ex && d) {
		t.Dir = clean
	} else {
		base := fileopsLastPart(clean)
		if base != "" && (strings.ContainsAny(base, "*?") || sources == 1) {
			t.Dir, t.Rename = fsys.Parent(clean), base
		} else {
			t.Dir = clean
		}
	}
	ex, d := isDir(t.Dir)
	if ex && !d {
		return t, fmt.Errorf("%s is a file, not a folder.", t.Dir)
	}
	t.DirExists = ex
	// a move within one archive or one server is a rename there
	if from != "" && r != nil {
		t.SameVolume = vfsSamePlace(r, fsys.Join(from, "x"), fsys.Join(t.Dir, "x"))
	}
	return t, nil
}

// fileopsLastPart is the last name of a path or URL.
func fileopsLastPart(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// fileopsVFSPath says whether a path a panel shows (or an item in it) must go through the vfs router: a server URL or
// a place inside an archive. folder: p is a folder a panel shows, so the archive file itself counts as its root.
func fileopsVFSPath(a *App, p string, folder bool) (*vfs.Router, bool) {
	r := vfsRouter(a)
	if strings.Contains(p, "://") {
		return r, true
	}
	if r == nil {
		return nil, false
	}
	if folder {
		return r, vfsPathKind(r, p) == "archive"
	}
	return r, r.Kind(p) == vfs.KindArchive
}

// fileopsTrailingSep keeps a separator the user typed at the end (it says "this is a folder"), which Join drops.
func fileopsTrailingSep(in string) string {
	if strings.HasSuffix(in, "/") || strings.HasSuffix(in, string(filepath.Separator)) {
		return string(filepath.Separator)
	}
	return ""
}

func fileopsExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[1:]) + fileopsTrailingSep(p)
		}
	}
	return p
}

// SameVolume says whether two paths are on the same volume (a move between them is an instant rename).
func (j *Jobs) SameVolume(a, b string) bool { return fileops.SameVolume(a, b) }

// TrashSupported says whether this system has a trash the app can use (false: deleting is always permanent).
func (j *Jobs) TrashSupported() bool { return fileops.TrashSupported() }

// MkDir makes the folder name inside parent and returns the name of the first new folder (for the cursor). name may
// hold several levels ("a/b/c") and date placeholders: [Y] year, [M] month, [D] day, [h] hour, [m] minute, [s]
// second. An absolute name is made where it says.
func (j *Jobs) MkDir(parent, name string) (string, error) {
	p, err := j.mkDir(parent, name)
	if err == nil {
		opLog(j.app, "mkdir", "New folder: "+p)
	}
	return p, err
}

func (j *Jobs) mkDir(parent, name string) (string, error) {
	if j.app.mock != nil {
		return "", errors.New("This is the demo: its files are not real, so no folder can be made.")
	}
	name = strings.TrimSpace(fileopsPlaceholders(name, time.Now()))
	if name == "" {
		return "", errors.New("Type a name for the new folder.")
	}
	if r, ok := fileopsVFSPath(j.app, parent, true); ok {
		if why := vfsArchiveReadOnly(r, parent, true); why != "" {
			return "", errors.New(why) // (board #143)
		}
		return fileopsMkdirVFS(j.app, r, parent, name)
	}
	name = fileopsExpandHome(name)
	full := name
	if !filepath.IsAbs(name) {
		full = filepath.Join(parent, name)
	}
	full = filepath.Clean(full)
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == filepath.Separator }) {
		if part == ".." && !filepath.IsAbs(name) {
			continue // "../x" makes a folder next to this one, as in TC
		}
		if err := fileopsCheckName(part); err != nil {
			return "", err
		}
	}
	if fi, err := os.Stat(full); err == nil {
		if fi.IsDir() {
			return "", fmt.Errorf("The folder %s is already there.", filepath.Base(full))
		}
		return "", fmt.Errorf("There is already a file named %s.", filepath.Base(full))
	}
	// the first level that is not there yet, for the undo journal (undoing removes the levels this made, no more)
	firstNew := full
	for p := filepath.Dir(full); p != filepath.Dir(p); p = filepath.Dir(p) {
		if _, err := os.Lstat(p); err == nil {
			break
		}
		firstNew = p
	}
	if err := os.MkdirAll(full, 0o777); err != nil {
		return "", fmt.Errorf("Can't make the folder %s: %s.", filepath.Base(full), fileopsPlain(err))
	}
	j.app.undo.record(undo.MkDir, undo.Item{From: firstNew, To: full, Dir: true})
	// the first level that is new and sits directly in parent gets the cursor
	if rel, err := filepath.Rel(parent, full); err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
		return strings.Split(rel, string(filepath.Separator))[0], nil
	}
	return "", nil
}

// fileopsMkdirVFS makes a folder on a server or inside an archive (through the panels' router when there is one).
func fileopsMkdirVFS(a *App, r *vfs.Router, parent, name string) (string, error) {
	var fsys vfs.FS = vfs.For(parent, a.fs)
	if r != nil {
		fsys = r
		parent = r.Clean(parent)
	}
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return "", errors.New("Type a name for the new folder.")
	}
	for _, part := range parts {
		if err := fileopsCheckName(part); err != nil {
			return "", err
		}
	}
	// one level at a time ("a/b/c"): levels that are already there are fine, except the last one
	p := parent
	for i, part := range parts {
		p = fsys.Join(p, part)
		last := i == len(parts)-1
		err := vfs.Mkdir(context.Background(), fsys, p)
		if err == nil || (!last && errors.Is(err, fs.ErrExist)) {
			continue
		}
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("The folder %s is already there.", part)
		}
		if errors.Is(err, vfs.ErrUnsupported) || errors.Is(err, vfs.ErrReadOnly) {
			return "", errors.New("Folders can't be made here.")
		}
		return "", fmt.Errorf("Can't make the folder %s: %s", part, vfsErrText(err))
	}
	return parts[0], nil
}

// fileopsPlaceholders puts the date and time into [Y] [M] [D] [h] [m] [s] (the Multi-Rename tool's letters).
func fileopsPlaceholders(s string, t time.Time) string {
	if !strings.Contains(s, "[") {
		return s
	}
	r := strings.NewReplacer(
		"[Y]", fmt.Sprintf("%04d", t.Year()), "[M]", fmt.Sprintf("%02d", int(t.Month())), "[D]", fmt.Sprintf("%02d", t.Day()),
		"[h]", fmt.Sprintf("%02d", t.Hour()), "[m]", fmt.Sprintf("%02d", t.Minute()), "[s]", fmt.Sprintf("%02d", t.Second()),
		"[y]", fmt.Sprintf("%02d", t.Year()%100),
	)
	return r.Replace(s)
}

// fileopsCheckName refuses names the system can't hold.
func fileopsCheckName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("%q can't be the name of a file or folder.", name)
	}
	if strings.ContainsRune(name, 0) {
		return errors.New("A name can't contain a zero character.")
	}
	if runtime.GOOS == "windows" {
		if strings.ContainsAny(name, `<>:"|?*`) {
			return fmt.Errorf("%s: a name can't contain < > : \" | ? or *.", name)
		}
		if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
			return fmt.Errorf("%s: a name can't end with a dot or a space on Windows.", name)
		}
	}
	return nil
}

// Rename renames the file or folder at path to newName in the same folder (Shift+F6) and returns the new path. A
// change of case only ("a.txt" to "A.txt") works on case-insensitive disks too.
func (j *Jobs) Rename(path, newName string) (string, error) {
	p, err := j.rename(path, newName)
	if err == nil {
		opLog(j.app, "rename", "Rename: "+path+" -> "+p)
	}
	return p, err
}

func (j *Jobs) rename(path, newName string) (string, error) {
	if j.app.mock != nil {
		return "", errors.New("This is the demo: its files are not real, so they can't be renamed.")
	}
	if newName == "" {
		return "", errors.New("Type the new name.")
	}
	if strings.ContainsAny(newName, `/\`) {
		return "", errors.New("A name can't contain / or \\. To move the file to another folder, use F6.")
	}
	if r, ok := fileopsVFSPath(j.app, path, false); ok {
		var fsys vfs.FS = vfs.For(path, j.app.fs)
		if r != nil {
			fsys = r
			path = r.Clean(path)
		}
		if err := fileopsCheckName(newName); err != nil {
			return "", err
		}
		if why := vfsArchiveReadOnly(r, path, false); why != "" {
			return "", errors.New(why) // (board #143)
		}
		dst := fsys.Join(fsys.Parent(path), newName)
		if err := vfs.Rename(context.Background(), fsys, path, dst); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return "", fmt.Errorf("There is already a file or folder named %s.", newName)
			}
			if errors.Is(err, vfs.ErrUnsupported) || errors.Is(err, vfs.ErrReadOnly) {
				return "", errors.New("Files can't be renamed here.")
			}
			return "", fmt.Errorf("Can't rename %s: %s", fileopsLastPart(path), vfsErrText(err))
		}
		notesInstance.moved(path, dst, true) // the note and colour label go along (feature_notes.go)
		return dst, nil
	}
	if err := fileopsCheckName(newName); err != nil {
		return "", err
	}
	dst, err := fileops.RenameItem(path, newName)
	if err != nil {
		if errors.Is(err, fileops.ErrExists) {
			return "", fmt.Errorf("There is already a file or folder named %s.", newName)
		}
		return "", fmt.Errorf("Can't rename %s: %s.", filepath.Base(path), fileopsPlain(err))
	}
	if src := filepath.Clean(path); dst != src {
		j.app.undo.record(undo.Rename, undo.Item{From: src, To: dst})
		notesInstance.moved(src, dst, true) // the note and colour label go along (feature_notes.go)
	}
	return dst, nil
}

// fileopsPlain says what went wrong in a few words, without the Go error's path prefix.
func fileopsPlain(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		err = le.Err
	}
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	case errors.Is(err, fs.ErrNotExist):
		return "it is not there any more"
	case errors.Is(err, fs.ErrExist):
		return "the name is taken"
	}
	return err.Error()
}

// fileopsSentence makes an engine message a sentence: capital first letter, full stop at the end.
func fileopsSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "?") && !strings.HasSuffix(s, "!") {
		s += "."
	}
	return s
}
