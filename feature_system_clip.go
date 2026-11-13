package main

// Desktop integration, the files half: the files clipboard shared with Finder / Explorer / the Linux file managers
// (Ctrl+C / Ctrl+X / Ctrl+V, F-237, F-739), pasting text or an image as a new file (F-740), planning a drop or paste
// as a copy or move job (section 36, refusing a folder into itself, F-721), links made by a drop, and the volume menu
// of the drive bar (properties, eject; section 38). internal/clipfiles, internal/links and internal/dirsize do the work.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"mormicommander/internal/clipfiles"
	"mormicommander/internal/dirsize"
	"mormicommander/internal/links"
)

// SystemClip is what the page calls for the files clipboard (window.go.main.SystemClip).
type SystemClip struct {
	app *App
	// clip is the clipboard used; nil means the system clipboard (tests set a private one).
	clip *clipfiles.Clipboard
}

func (c *SystemClip) board() *clipfiles.Clipboard {
	if c.clip == nil {
		c.clip = clipfiles.New()
	}
	return c.clip
}

var systemClipDemoErr = errors.New("This is the demo: its files are not real, so they can't go to the clipboard.")

// clipErr turns a clipboard error into a sentence.
func clipErr(err error) error {
	switch {
	case errors.Is(err, clipfiles.ErrEmpty):
		return errors.New("The clipboard holds no files.")
	case errors.Is(err, clipfiles.ErrIntoItself):
		return errors.New("You can't copy or move a folder into itself.")
	case errors.Is(err, clipfiles.ErrUnsupported):
		return errors.New("This system's clipboard can't hold files.")
	case errors.Is(err, clipfiles.ErrNoTool):
		return fmt.Errorf("The clipboard can't be used: %v", err)
	}
	return fmt.Errorf("The clipboard can't be used: %v", err)
}

// Copy puts files on the clipboard (Ctrl+C), or marks them to be moved by the next paste (cut, Ctrl+X).
func (c *SystemClip) Copy(paths []string, cut bool) error {
	if c.app.mock != nil {
		return systemClipDemoErr
	}
	if len(paths) == 0 {
		return errors.New("No files are selected.")
	}
	op := clipfiles.Copy
	if cut {
		op = clipfiles.Cut
	}
	if err := c.board().WriteFiles(paths, op); err != nil {
		return clipErr(err)
	}
	return nil
}

// WriteText puts text on the clipboard (a path or a name from the context menu).
func (c *SystemClip) WriteText(text string) error {
	if err := c.board().WriteText(text); err != nil {
		return clipErr(err)
	}
	return nil
}

// SystemClipContent tells what the clipboard holds, so Ctrl+V knows whether to paste files or save text or an image.
type SystemClipContent struct {
	Files bool `json:"files"`
	Text  bool `json:"text"`
	Image bool `json:"image"`
}

// Content tells whether the clipboard holds files, text or an image.
func (c *SystemClip) Content() (SystemClipContent, error) {
	if c.app.mock != nil {
		return SystemClipContent{}, systemClipDemoErr
	}
	ct, err := c.board().Content()
	if err != nil {
		return SystemClipContent{}, clipErr(err)
	}
	return SystemClipContent{Files: ct.Files, Text: ct.Text, Image: ct.Image}, nil
}

// SystemSameDir is a file copied into the folder it is already in, with the free name its copy gets.
type SystemSameDir struct {
	Source  string `json:"source"`
	NewName string `json:"newName"`
}

// SystemTransfer is a paste or a drop turned into file jobs: Sources go to Dest in one job; each of Same is copied
// next to itself under its NewName; Skipped counts files moved into the folder they are already in (nothing to do).
type SystemTransfer struct {
	Sources []string        `json:"sources"`
	Same    []SystemSameDir `json:"same"`
	Dest    string          `json:"dest"`
	Move    bool            `json:"move"`
	Skipped int             `json:"skipped"`
}

// systemTransfer splits a clipboard plan into the jobs the page starts.
func systemTransfer(p clipfiles.Plan) SystemTransfer {
	t := SystemTransfer{Dest: p.Dest, Move: p.Move, Sources: []string{}, Same: []SystemSameDir{}}
	for _, s := range p.Sources {
		if !systemSamePath(filepath.Dir(s), p.Dest) {
			t.Sources = append(t.Sources, s)
			continue
		}
		if p.Move {
			t.Skipped++
			continue
		}
		base := filepath.Base(s)
		ext := ""
		if fi, err := os.Stat(s); err == nil && !fi.IsDir() {
			ext = filepath.Ext(base)
		}
		t.Same = append(t.Same, SystemSameDir{Source: s, NewName: clipfiles.SuggestName(p.Dest, strings.TrimSuffix(base, ext), ext)})
	}
	return t
}

// systemSamePath compares two folders the way the system does (without case on Windows).
func systemSamePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// PlanPaste reads the files on the clipboard and plans pasting them into dest (Ctrl+V).
func (c *SystemClip) PlanPaste(dest string) (SystemTransfer, error) {
	if c.app.mock != nil {
		return SystemTransfer{}, systemClipDemoErr
	}
	if err := systemLocalDir(dest); err != nil {
		return SystemTransfer{}, errors.New("Files can only be pasted into folders on this computer.")
	}
	p, err := c.board().PlanPaste(dest)
	if err != nil {
		return SystemTransfer{}, clipErr(err)
	}
	return systemTransfer(p), nil
}

// PlanTransfer plans copying (or moving) files into dest, for a drop: it refuses a folder into itself.
func (c *SystemClip) PlanTransfer(paths []string, move bool, dest string) (SystemTransfer, error) {
	if c.app.mock != nil {
		return SystemTransfer{}, systemClipDemoErr
	}
	if err := systemLocalDir(dest); err != nil {
		return SystemTransfer{}, errors.New("Files can only be dropped into folders on this computer.")
	}
	op := clipfiles.Copy
	if move {
		op = clipfiles.Cut
	}
	p, err := clipfiles.NewPlan(paths, op, dest)
	if err != nil {
		return SystemTransfer{}, clipErr(err)
	}
	return systemTransfer(p), nil
}

// Clear empties the clipboard (after cut files were moved by a paste, as Explorer does).
func (c *SystemClip) Clear() error {
	if c.app.mock != nil {
		return nil
	}
	return c.board().Clear()
}

// SuggestName is a free name in dir for pasted data: base+ext, or base(2)+ext and so on.
func (c *SystemClip) SuggestName(dir, base, ext string) string {
	return clipfiles.SuggestName(dir, base, ext)
}

// PasteData saves the clipboard's image (preferred) or text as a new file in dir and returns its path. It never
// overwrites a file.
func (c *SystemClip) PasteData(dir, name string) (string, error) {
	if c.app.mock != nil {
		return "", systemClipDemoErr
	}
	if err := systemLocalDir(dir); err != nil {
		return "", err
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", errors.New("A file name can't contain / or \\.")
	}
	p, err := c.board().PasteData(dir, name)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrExist):
			return "", fmt.Errorf("%s is already there. Choose another name.", name)
		case errors.Is(err, clipfiles.ErrEmpty):
			return "", errors.New("The clipboard holds no text and no image.")
		}
		return "", clipErr(err)
	}
	return p, nil
}

// SystemFiles is what the page calls for links made by a drop and for volumes (window.go.main.SystemFiles).
type SystemFiles struct{ app *App }

// CreateLinks makes symbolic links to files in dest (a drop with the link modifier). It returns one sentence per
// file that failed.
func (f *SystemFiles) CreateLinks(paths []string, dest string) ([]string, error) {
	if f.app.mock != nil {
		return nil, systemDemoErr
	}
	if err := systemLocalDir(dest); err != nil {
		return nil, err
	}
	res := links.CreateLinks(context.Background(), links.BatchRequest{
		Kind: links.KindSymlink, Sources: paths, DestDir: dest,
		Name: func(src string) string { return links.UniqueName(dest, links.DefaultLinkName(src, links.KindSymlink)) },
	})
	errs := []string{}
	for _, r := range res {
		if r.Err != nil {
			msg := r.Err.Error()
			if errors.Is(r.Err, links.ErrPrivilege) {
				msg = "you need Developer Mode or administrator rights for symbolic links"
			}
			errs = append(errs, fmt.Sprintf("Can't link %s: %s", filepath.Base(r.Source), msg))
		}
	}
	return errs, nil
}

// SystemVolumeProps describes a volume for its Properties box.
type SystemVolumeProps struct {
	dirsize.VolumeInfo
	Name string `json:"name"`
	Used uint64 `json:"used"`
}

// VolumeProps describes the volume a folder is on: name, file system, device, size and free space.
func (f *SystemFiles) VolumeProps(path string) (SystemVolumeProps, error) {
	if f.app.mock != nil {
		v := (&FS{app: f.app}).VolumeOf(path)
		return SystemVolumeProps{VolumeInfo: dirsize.VolumeInfo{Path: path, Root: v.Path, Label: v.Name, Total: v.Total, Free: v.Free, Available: v.Free, FileSystem: "apfs"},
			Name: v.Name, Used: v.Total - v.Free}, nil
	}
	v, err := dirsize.VolumeOf(path)
	if err != nil {
		return SystemVolumeProps{}, fmt.Errorf("Can't read the volume of %s: %v", path, err)
	}
	return SystemVolumeProps{VolumeInfo: v, Name: v.DisplayName(), Used: v.Used()}, nil
}

// Eject ejects (unmounts) the removable or network volume mounted at path.
func (f *SystemFiles) Eject(path string) error {
	if f.app.mock != nil {
		return errors.New("This is the demo: its volumes are not real, so they can't be ejected.")
	}
	name, args, env := systemEjectCommand(runtime.GOOS, path)
	if name == "" {
		return errors.New("This system can't eject volumes from here.")
	}
	run := func(name string, args []string) (string, error) {
		cmd := exec.Command(name, args...)
		cmd.Env = append(os.Environ(), env...)
		hideWindow(cmd)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	out, err := run(name, args)
	if err != nil && runtime.GOOS == "linux" {
		// gio is GNOME's; umount works for any mount the user may unmount (fstab "user" mounts).
		out, err = run("umount", []string{path})
	}
	if err != nil {
		if out == "" {
			out = err.Error()
		}
		return fmt.Errorf("Can't eject %s: %s. Close the files that are open on it and try again.", filepath.Base(path), strings.TrimSuffix(out, "."))
	}
	return nil
}

// systemEjectCommand is the program that ejects the volume at path on the given system. The path always travels as
// an argument or an environment variable, never inside script text.
func systemEjectCommand(goos, path string) (string, []string, []string) {
	switch goos {
	case "darwin":
		return "diskutil", []string{"eject", path}, nil
	case "linux":
		return "gio", []string{"mount", "-e", path}, nil
	case "windows":
		script := `$d=$env:MC_EJECT; if ($d.Length -gt 3) { $d=$d.Substring(0,3) }; (New-Object -ComObject Shell.Application).Namespace(17).ParseName($d).InvokeVerb('Eject')`
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", script}, []string{"MC_EJECT=" + path}
	}
	return "", nil, nil
}
