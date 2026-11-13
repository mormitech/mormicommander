// Inspect is the one call the UI makes to learn everything about a possible
// link: what kind it is, where it points, whether the target exists, and how
// many names a hard-linked file has. The panel uses it to draw broken links
// in their own colour, the Properties dialog shows its fields, and FollowLink
// turns it into the action TC performs for Ctrl+PgDn (cm_GoToDir on a link:
// open the target folder, or the target's folder with the cursor on it) and
// for "smart" Enter on shortcut files (F-738: follow to the folder, open the
// URL in the browser, or run the .desktop application). Broken links and
// loops are reported in the Info fields rather than as errors, because a
// broken link is still a perfectly listable file; only FollowLink, which must
// actually go somewhere, turns them into errors.

package links

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Inspect describes path without following it first.
func Inspect(path string) (*Info, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	info := &Info{Path: abs}
	if isFSLink(abs, fi) {
		info.Kind = KindSymlink
		if fi.Mode()&fs.ModeSymlink == 0 {
			info.Kind = KindJunction
		}
		if t, err := ReadLink(abs); err == nil {
			info.Target = t
		}
		res, rerr := Resolve(abs)
		if res != nil {
			info.Chain = res.Chain
			info.Resolved = res.Final
			info.TargetExists = res.Exists
			info.TargetIsDir = res.IsDir
		}
		switch {
		case errors.Is(rerr, ErrLoop):
			info.Loop = true
			info.Resolved = ""
		case errors.Is(rerr, ErrBroken):
			info.Broken = true
		}
		if info.TargetExists && !info.TargetIsDir {
			if tfi, err := os.Stat(info.Resolved); err == nil {
				info.HardLinks = linkCount(info.Resolved, tfi)
			}
		}
		return info, nil
	}
	if !fi.Mode().IsRegular() {
		return info, nil
	}
	info.HardLinks = linkCount(abs, fi)
	if info.HardLinks > 1 {
		info.Kind = KindHardLink
	}
	kind := ShortcutKindByName(abs)
	if kind == KindNone && fi.Size() >= 16 && fi.Size() <= 4<<20 && sniffAlias(abs) {
		kind = KindAlias
	}
	if kind != KindNone {
		if readShortcut(abs, kind, info) == nil {
			info.Kind = kind
			fillTarget(info)
		}
	}
	return info, nil
}

// readShortcut fills info from the shortcut file at p.
func readShortcut(p string, kind Kind, info *Info) error {
	switch kind {
	case KindLnk:
		l, err := ReadLnk(p)
		if err != nil {
			return err
		}
		info.Target = l.Target
		if info.Target == "" && l.RelativePath != "" {
			rel := l.RelativePath
			if runtime.GOOS != "windows" {
				rel = strings.ReplaceAll(rel, `\`, "/")
			}
			info.Target = filepath.Join(filepath.Dir(p), rel)
		}
		if runtime.GOOS == "windows" {
			info.Target = ExpandWindowsEnv(info.Target, nil)
		}
		info.Name, info.Arguments, info.WorkingDir = l.Name, l.Arguments, l.WorkingDir
		info.Icon = l.IconLocation
		info.TargetIsDir = l.IsDir()
	case KindURL:
		data, err := readSmall(p, 1<<20)
		if err != nil {
			return err
		}
		s, err := ParseURLShortcut(data)
		if err != nil {
			return err
		}
		info.Target, info.WorkingDir, info.Icon = s.URL, s.WorkingDir, s.Icon
	case KindWebloc:
		u, err := ReadWebloc(p)
		if err != nil {
			return err
		}
		info.Target = u
	case KindDesktop:
		data, err := readSmall(p, 1<<20)
		if err != nil {
			return err
		}
		s, err := ParseDesktopEntry(data)
		if err != nil {
			return err
		}
		info.Name, info.WorkingDir, info.Icon = s.Name, s.WorkingDir, s.Icon
		switch s.Type {
		case "Link":
			info.Target = s.URL
		case "Application":
			info.Exec = s.Exec
		}
	case KindAlias:
		t, err := ReadAlias(p)
		if err != nil {
			return err
		}
		info.Target = t
	default:
		return ErrNotLink
	}
	return nil
}

// fillTarget turns a shortcut's stored Target into URL or Resolved and
// checks whether a local target exists.
func fillTarget(info *Info) {
	t := info.Target
	if t == "" {
		return
	}
	if IsURL(t) {
		p, ok := PathFromFileURL(t)
		if !ok {
			info.URL = t
			return
		}
		t = p
	}
	if !filepath.IsAbs(t) {
		return // a foreign path, e.g. C:\x read on macOS
	}
	info.Resolved = filepath.Clean(t)
	fi, err := os.Stat(info.Resolved)
	if err != nil {
		info.Broken = true
		info.TargetExists = false
		return
	}
	info.TargetExists, info.TargetIsDir = true, fi.IsDir()
	if r, err := filepath.EvalSymlinks(info.Resolved); err == nil {
		info.Resolved = r
	}
}

// ActionKind says what following a link should do.
type ActionKind int

const (
	// ActionOpenDir: show folder Dir in the panel.
	ActionOpenDir ActionKind = iota + 1
	// ActionFocusFile: show folder Dir and put the cursor on Focus.
	ActionFocusFile
	// ActionOpenURL: open URL in the default browser / handler.
	ActionOpenURL
	// ActionRun: start the program Args[0] with Args[1:] in Dir.
	ActionRun
)

// Action is the result of FollowLink.
type Action struct {
	Kind  ActionKind
	Dir   string
	Focus string
	URL   string
	Args  []string
	Info  *Info
}

// FollowLink returns what Ctrl+PgDn / smart Enter should do with the link or
// shortcut at path. It fails with ErrNotLink for ordinary files, ErrBroken
// when the target is missing (or is a path from another OS) and ErrLoop for
// cycles.
func FollowLink(path string) (*Action, error) {
	info, err := Inspect(path)
	if err != nil {
		return nil, err
	}
	a := &Action{Info: info}
	switch {
	case info.Kind == KindNone || info.Kind == KindHardLink:
		return nil, &fs.PathError{Op: "follow", Path: info.Path, Err: ErrNotLink}
	case info.Loop:
		return nil, &LoopError{Path: info.Path, Chain: info.Chain}
	case info.URL != "":
		a.Kind, a.URL = ActionOpenURL, info.URL
		return a, nil
	case info.Exec != "":
		a.Kind, a.Args, a.Dir = ActionRun, ExecArgs(info.Exec), info.WorkingDir
		if len(a.Args) == 0 {
			return nil, &fs.PathError{Op: "follow", Path: info.Path, Err: ErrBadShortcut}
		}
		return a, nil
	case !info.TargetExists:
		return nil, &fs.PathError{Op: "follow", Path: info.Path, Err: ErrBroken}
	case info.TargetIsDir:
		a.Kind, a.Dir = ActionOpenDir, info.Resolved
	default:
		a.Kind, a.Dir, a.Focus = ActionFocusFile, filepath.Dir(info.Resolved), filepath.Base(info.Resolved)
	}
	return a, nil
}
