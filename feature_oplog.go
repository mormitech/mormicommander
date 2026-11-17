package main

// What goes into the log file besides errors (board issue #94: Options > Log file > Log copy and move, Log delete,
// Log rename, Log new folders, Log pack and unpack, Log FTP and network operations, Log errors only). The log itself is
// SystemLog's (feature_system_watch.go): nothing is written unless Options > Log file > Write a log file is on. Errors
// are logged by the page as they are shown (frontend/src/features/system-lib.js logError), so here only what was done.

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/fileops"
	lst "mormicommander/internal/listing"
)

var opLogCache struct {
	mu   sync.Mutex
	at   time.Time
	cats map[string]bool
	errs bool
}

// opLogWanted says whether a category ("copy", "delete", "rename", "mkdir", "pack", "ftp") is logged. The options are
// read at most once a second, as a big copy logs every file.
func opLogWanted(cat string) bool {
	c := &opLogCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cats == nil || time.Since(c.at) > time.Second {
		c.at = time.Now()
		c.cats = map[string]bool{"copy": true, "delete": true, "rename": true, "mkdir": true, "pack": true, "ftp": true}
		c.errs = false
		if s, ok := currentSettings(); ok {
			c.cats = map[string]bool{"copy": s.Log.Copy, "delete": s.Log.Delete, "rename": s.Log.Rename, "mkdir": s.Log.MkDir, "pack": s.Log.Pack, "ftp": s.Log.FTP}
			c.errs = s.Log.ErrorsOnly
		}
	}
	return !c.errs && c.cats[cat]
}

// opLog writes lines of a category to the log file when that category is logged.
func opLog(a *App, cat string, lines ...string) {
	if a == nil || a.settings == nil || a.mock != nil {
		return
	}
	if on, _ := a.settings.Get("enableLog").(bool); !on || !opLogWanted(cat) {
		return
	}
	_ = (&SystemLog{app: a}).Write(lines)
}

// jobLog logs one thing a file job did.
func jobLog(a *App, e fileops.Entry) {
	cat := ""
	switch e.Action {
	case "copy", "move":
		cat = "copy"
	case "delete", "trash", "wipe", "rmdir":
		cat = "delete"
	case "rename":
		cat = "rename"
	case "mkdir":
		cat = "mkdir"
	default:
		return
	}
	line := e.Action + ": " + e.Source
	if e.Target != "" {
		line += " -> " + e.Target
	}
	opLog(a, cat, line)
}

// vfsLog logs a job on archives or servers when it ends.
func vfsLog(a *App, r VfsRequest, state string) {
	cat := "copy"
	switch r.Kind {
	case "pack", "unpack", "test":
		cat = "pack"
	case "delete", "trash", "wipe":
		cat = "delete"
	}
	for _, p := range append([]string{r.Target}, r.Sources...) {
		if l := strings.ToLower(p); strings.HasPrefix(l, "ftp://") || strings.HasPrefix(l, "sftp://") || strings.HasPrefix(l, "ftps://") || strings.HasPrefix(l, "dav://") || strings.HasPrefix(l, "davs://") || strings.HasPrefix(l, "webdav://") {
			if cat == "copy" || cat == "delete" {
				cat = "ftp"
			}
			break
		}
	}
	line := fmt.Sprintf("%s: %s", r.Kind, strings.Join(r.Sources, ", "))
	if r.Target != "" {
		line += " -> " + r.Target
	}
	opLog(a, cat, line+" ("+state+")")
}

// copyJobComments takes the file comments (descript.ion) of what a copy or move got across along to the target folder
// (Options > Copy/Delete > Copy file comments); a move also takes them out of the source folder's file. Sources that
// did not get across whole keep their comments where they were.
func copyJobComments(r fileops.Request, final fileops.Progress) {
	if s, ok := currentSettings(); !ok || !s.CopyDelete.CopyComments {
		return
	}
	left := map[string]bool{}
	for _, l := range final.Left {
		left[l] = true
	}
	byDir := map[string][]string{}
	for _, src := range r.Sources {
		if !left[src] {
			byDir[filepath.Dir(src)] = append(byDir[filepath.Dir(src)], src)
		}
	}
	var dst *lst.Comments
	changed := false
	for dir, srcs := range byDir {
		c, err := lst.LoadComments(dir)
		if err != nil || c.Len() == 0 {
			continue
		}
		srcChanged := false
		for _, src := range srcs {
			name := filepath.Base(src)
			text := c.Get(name)
			if text == "" {
				continue
			}
			if dst == nil {
				if dst, err = lst.LoadComments(r.Target); err != nil {
					return
				}
			}
			dst.Set(fileops.ApplyMask(r.Rename, name), text)
			changed = true
			if r.Kind == fileops.Move {
				c.Set(name, "")
				srcChanged = true
			}
		}
		if srcChanged && !samePathCI(dir, r.Target) {
			_ = c.Save(dir)
		}
	}
	if changed {
		_ = dst.Save(r.Target)
	}
}

func samePathCI(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }
