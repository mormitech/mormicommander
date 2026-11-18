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
	case "pack", "unpack", "test", "convert":
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
		same := samePathCI(dir, r.Target) // a copy or move inside the folder (a new name): one descript.ion (F-745)
		for _, src := range srcs {
			name := filepath.Base(src)
			text := c.Get(name)
			if text == "" {
				continue
			}
			if same {
				newName := fileops.ApplyMask(r.Rename, name)
				if r.Kind == fileops.Move {
					c.Set(name, "")
				}
				c.Set(newName, text)
				srcChanged = true
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
		if srcChanged {
			_ = c.Save(dir)
		}
	}
	if changed {
		_ = dst.Save(r.Target)
	}
}

// moveComments carries the file comments (descript.ion) along renames: Shift+F6 in place and the multi-rename tool
// (F-745, Options > Copy/Delete > Copy file comments). moved maps old paths to new ones.
func moveComments(moved map[string]string) {
	if s, ok := currentSettings(); !ok || !s.CopyDelete.CopyComments {
		return
	}
	moveCommentsNow(moved)
}

// moveCommentsNow moves the comments of moved (old path to new path). All texts are read before any is written, so
// names that swap ("a" to "b" and "b" to "a") keep their own comments.
func moveCommentsNow(moved map[string]string) {
	files := map[string]*lst.Comments{}
	load := func(dir string) *lst.Comments {
		if c, ok := files[dir]; ok {
			return c
		}
		c, err := lst.LoadComments(dir)
		if err != nil {
			c = nil
		}
		files[dir] = c
		return c
	}
	type carry struct{ dir, name, text string }
	var news []carry
	for from, to := range moved {
		if c := load(filepath.Dir(from)); c != nil {
			if t := c.Get(filepath.Base(from)); t != "" {
				news = append(news, carry{filepath.Dir(to), filepath.Base(to), t})
			}
		}
	}
	if len(news) == 0 {
		return
	}
	changed := map[string]bool{}
	for from := range moved {
		dir, name := filepath.Dir(from), filepath.Base(from)
		if c := load(dir); c != nil && c.Get(name) != "" {
			c.Set(name, "")
			changed[dir] = true
		}
	}
	for _, n := range news {
		if c := load(n.dir); c != nil {
			c.Set(n.name, n.text)
			changed[n.dir] = true
		}
	}
	for dir := range changed {
		_ = files[dir].Save(dir)
	}
}

func samePathCI(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }
