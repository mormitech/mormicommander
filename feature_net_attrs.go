package main

// Properties and attributes of files on servers (F-649, F-633): Alt+Enter on a server panel shows a file's details
// (RemoteInfo), Files > Change attributes sets the Unix permissions (SITE CHMOD on FTP, chmod on SFTP), optionally
// through subfolders, and the modification time where the server can set it (RemoteChmod), and Create symbolic link
// makes a link on an SFTP server (RemoteSymlink). The page side is frontend/src/features/net-attrs.js.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"mormicommander/internal/vfs"
)

// NetRemoteInfo is what the properties of a server file show.
type NetRemoteInfo struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Folder     string `json:"folder"`
	Server     string `json:"server"` // user@host[:port]
	Kind       string `json:"kind"`   // ftp, sftp, webdav
	Size       int64  `json:"size"`
	MTime      int64  `json:"mtime"`
	Perms      string `json:"perms"` // "-rw-r--r--"; "" when the server sends none
	Octal      string `json:"octal"` // "0644"
	IsDir      bool   `json:"isDir"`
	IsLink     bool   `json:"isLink"`
	Broken     bool   `json:"broken"`
	LinkTarget string `json:"linkTarget"`
	UID        int64  `json:"uid"` // -1 when unknown
	GID        int64  `json:"gid"`
	CanChmod   bool   `json:"canChmod"`
	CanChtimes bool   `json:"canChtimes"`
	CanSymlink bool   `json:"canSymlink"`
}

// netUnixBits turns a file mode into the Unix permission number (with set-user-ID, set-group-ID and sticky).
func netUnixBits(m fs.FileMode) uint32 {
	b := uint32(m.Perm())
	if m&fs.ModeSetuid != 0 {
		b |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		b |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		b |= 0o1000
	}
	return b
}

// netModeOf is the file mode of a Unix permission number.
func netModeOf(bits uint32) fs.FileMode {
	m := fs.FileMode(bits & 0o777)
	if bits&0o4000 != 0 {
		m |= fs.ModeSetuid
	}
	if bits&0o2000 != 0 {
		m |= fs.ModeSetgid
	}
	if bits&0o1000 != 0 {
		m |= fs.ModeSticky
	}
	return m
}

// netParseOctal reads "644" or "0755" (at most 4 octal digits).
func netParseOctal(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 4 {
		return 0, fmt.Errorf("%q is not a permission number such as 644", s)
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("%q is not a permission number such as 644", s)
	}
	return uint32(v), nil
}

// RemoteInfo describes one file or folder on a server.
func (n *Net) RemoteInfo(p string) (NetRemoteInfo, error) {
	if !netIsRemote(p) {
		return NetRemoteInfo{}, errors.New("This is not on a server.")
	}
	r := n.router()
	e, err := r.Stat(p)
	if err != nil {
		return NetRemoteInfo{}, fmt.Errorf("Can't read %s: %s", netBase(p), netErrText(err))
	}
	caps := r.Caps(p)
	kind := netSchemes[netSchemeOf(p)]
	key := netKeyFor(p)
	if i := strings.Index(key, "://"); i >= 0 {
		key = key[i+3:]
	}
	info := NetRemoteInfo{
		Path: p, Name: netBase(p), Folder: r.Parent(p), Server: key, Kind: kind,
		Size: e.Size, MTime: e.MTime, IsDir: e.Flags&vfs.FlagDir != 0, IsLink: e.Flags&vfs.FlagLink != 0,
		Broken: e.Flags&vfs.FlagBroken != 0, UID: -1, GID: -1,
		CanChmod: caps.Chmod, CanChtimes: caps.Chtimes, CanSymlink: kind == "sftp" && r.SFTP != nil,
	}
	if e.Mode != 0 {
		m := fs.FileMode(e.Mode)
		info.Perms = m.String()
		info.Octal = fmt.Sprintf("%04o", netUnixBits(m))
	}
	if kind == "sftp" && r.SFTP != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if t, uid, gid, err := r.SFTP.Details(ctx, p); err == nil {
			info.LinkTarget, info.UID, info.GID = t, int64(uid), int64(gid)
		}
	}
	return info, nil
}

// NetChmodRequest is a change of attributes on a server.
type NetChmodRequest struct {
	Paths     []string `json:"paths"`
	Octal     string   `json:"octal"`     // the permissions to set ("" leaves them)
	Recursive bool     `json:"recursive"` // also everything inside selected folders
	ApplyTo   string   `json:"applyTo"`   // "" files and folders, "files", "folders"
	MTime     int64    `json:"mtime"`     // Unix ms to set as the modification time, 0 leaves it
}

// NetFailure is one item a server operation could not do.
type NetFailure struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// NetChmodResult says how a change of attributes went.
type NetChmodResult struct {
	Done   int          `json:"done"`
	Failed []NetFailure `json:"failed"`
}

// RemoteChmod sets permissions (and the modification time) of server files, through subfolders when asked.
func (n *Net) RemoteChmod(req NetChmodRequest) (NetChmodResult, error) {
	res := NetChmodResult{Failed: []NetFailure{}}
	if len(req.Paths) == 0 {
		return res, errors.New("Nothing is selected.")
	}
	var mode fs.FileMode
	setMode := strings.TrimSpace(req.Octal) != ""
	if setMode {
		bits, err := netParseOctal(req.Octal)
		if err != nil {
			return res, err
		}
		mode = netModeOf(bits)
	}
	if !setMode && req.MTime == 0 {
		return res, errors.New("Nothing to change.")
	}
	r := n.router()
	for _, p := range req.Paths {
		if !netIsRemote(p) {
			return res, fmt.Errorf("%s is not on a server.", netBase(p))
		}
		if setMode && !r.Caps(p).Chmod {
			return res, errors.New("This server can't change permissions.")
		}
		if req.MTime != 0 && !r.Caps(p).Chtimes {
			return res, errors.New("This server can't change the date of files.")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	apply := func(p string, dir bool) {
		if (req.ApplyTo == "files" && dir) || (req.ApplyTo == "folders" && !dir) {
			return
		}
		var err error
		if setMode {
			err = r.Chmod(ctx, p, mode)
		}
		if err == nil && req.MTime != 0 {
			err = r.Chtimes(ctx, p, time.UnixMilli(req.MTime))
		}
		if err != nil {
			res.Failed = append(res.Failed, NetFailure{Path: p, Error: netErrText(err)})
			return
		}
		res.Done++
	}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 64 {
			res.Failed = append(res.Failed, NetFailure{Path: dir, Error: "the folders are nested too deeply"})
			return
		}
		l, err := r.List(ctx, dir)
		if err != nil {
			res.Failed = append(res.Failed, NetFailure{Path: dir, Error: netErrText(err)})
			return
		}
		for _, e := range l.Entries {
			if !netPlainName(e.Name) {
				continue // a listing is not trusted: never touch a name that leaves the folder
			}
			c := r.Join(dir, e.Name)
			isDir := e.Flags&vfs.FlagDir != 0
			if isDir && e.Flags&vfs.FlagLink != 0 {
				apply(c, true) // a link to a folder is changed but not followed (loops)
				continue
			}
			if isDir {
				walk(c, depth+1)
			}
			apply(c, isDir)
		}
	}
	for _, p := range req.Paths {
		e, err := r.Stat(p)
		if err != nil {
			res.Failed = append(res.Failed, NetFailure{Path: p, Error: netErrText(err)})
			continue
		}
		isDir := e.Flags&vfs.FlagDir != 0
		if isDir && req.Recursive && e.Flags&vfs.FlagLink == 0 {
			walk(p, 0)
		}
		apply(p, isDir)
	}
	opLog(n.app, "ftp", fmt.Sprintf("attributes (network): %s mode=%q mtime=%d recursive=%v: %d done, %d failed", strings.Join(req.Paths, ", "), req.Octal, req.MTime, req.Recursive, res.Done, len(res.Failed)))
	return res, nil
}

// RemoteSymlink makes a symbolic link named name in the SFTP folder dir that points to target, and returns its path.
// An existing name is never replaced.
func (n *Net) RemoteSymlink(dir, name, target string) (string, error) {
	if netSchemes[netSchemeOf(dir)] != "sftp" {
		return "", errors.New("Symbolic links can be made on SFTP servers only.")
	}
	name, target = strings.TrimSpace(name), strings.TrimSpace(target)
	if !netPlainName(name) {
		return "", fmt.Errorf("%q is not a plain name.", name)
	}
	if target == "" {
		return "", errors.New("Type what the link points to.")
	}
	r := n.router()
	if r.SFTP == nil {
		return "", errors.New("SFTP is not available.")
	}
	p := r.Join(dir, name)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := r.SFTP.Symlink(ctx, target, p); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("%s is already there.", name)
		}
		return "", fmt.Errorf("Can't make the link %s: %s", name, netErrText(err))
	}
	opLog(n.app, "ftp", fmt.Sprintf("symlink (network): %s -> %s", p, target))
	return p, nil
}
