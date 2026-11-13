// Finding applications the freedesktop way (Linux and the BSDs). Applications install .desktop files under
// applications/ in the XDG data folders ($XDG_DATA_HOME, then $XDG_DATA_DIRS); the file's id is its path below
// applications/ with slashes turned into dashes, and an earlier folder hides a later file with the same id. The
// user's and the system's choices are in mimeapps.list files ([Default Applications], [Added Associations],
// [Removed Associations]) under the XDG config folders and the data folders. A file's MIME type is taken from its
// extension, with a look at its first bytes for text files without one. This file reads all of that given the
// environment as a function, so tests can point it at a temporary folder.
package launch

import (
	"errors"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// xdgEnv holds what the XDG lookups need from the environment.
type xdgEnv struct {
	getenv func(string) string
}

func (e xdgEnv) home() string { return e.getenv("HOME") }

func (e xdgEnv) dataDirs() []string {
	var out []string
	if h := e.getenv("XDG_DATA_HOME"); h != "" {
		out = append(out, h)
	} else if e.home() != "" {
		out = append(out, filepath.Join(e.home(), ".local", "share"))
	}
	dd := e.getenv("XDG_DATA_DIRS")
	if dd == "" {
		dd = "/usr/local/share:/usr/share"
	}
	for _, d := range strings.Split(dd, ":") {
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

func (e xdgEnv) configDirs() []string {
	var out []string
	if h := e.getenv("XDG_CONFIG_HOME"); h != "" {
		out = append(out, h)
	} else if e.home() != "" {
		out = append(out, filepath.Join(e.home(), ".config"))
	}
	cd := e.getenv("XDG_CONFIG_DIRS")
	if cd == "" {
		cd = "/etc/xdg"
	}
	for _, d := range strings.Split(cd, ":") {
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

// desktopFiles maps desktop file ids to their paths; the first data folder wins.
func (e xdgEnv) desktopFiles() map[string]string {
	ids := map[string]string{}
	for _, d := range e.dataDirs() {
		root := filepath.Join(d, "applications")
		_ = filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.HasSuffix(p, ".desktop") {
				return nil
			}
			rel, rerr := filepath.Rel(root, p)
			if rerr != nil {
				return nil
			}
			id := strings.ReplaceAll(filepath.ToSlash(rel), "/", "-")
			if _, seen := ids[id]; !seen {
				ids[id] = p
			}
			return nil
		})
	}
	return ids
}

// mimeappsFiles lists the mimeapps.list files in precedence order (most important first).
func (e xdgEnv) mimeappsFiles() []string {
	var desktops []string
	for _, d := range strings.Split(e.getenv("XDG_CURRENT_DESKTOP"), ":") {
		if d != "" {
			desktops = append(desktops, strings.ToLower(d))
		}
	}
	var out []string
	for _, dir := range e.configDirs() {
		for _, d := range desktops {
			out = append(out, filepath.Join(dir, d+"-mimeapps.list"))
		}
		out = append(out, filepath.Join(dir, "mimeapps.list"))
	}
	for _, dir := range e.dataDirs() {
		for _, d := range desktops {
			out = append(out, filepath.Join(dir, "applications", d+"-mimeapps.list"))
		}
		out = append(out, filepath.Join(dir, "applications", "mimeapps.list"))
		out = append(out, filepath.Join(dir, "applications", "defaults.list"))
	}
	return out
}

// mimeChoices is what the mimeapps.list files say about one MIME type.
type mimeChoices struct {
	defaults []string        // default application ids, best first
	added    []string        // ids added by the user or system
	removed  map[string]bool // ids removed for this type
}

func (e xdgEnv) choicesFor(mimeType string) mimeChoices {
	c := mimeChoices{removed: map[string]bool{}}
	for _, f := range e.mimeappsFiles() {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		ini := ParseINI(data)
		list := func(section string) []string {
			s := ini.Section(section)
			if s == nil {
				return nil
			}
			v, _ := s.Get(mimeType)
			var ids []string
			for _, id := range strings.Split(v, ";") {
				if id = strings.TrimSpace(id); id != "" {
					ids = append(ids, id)
				}
			}
			return ids
		}
		for _, id := range list("Removed Associations") {
			c.removed[id] = true
		}
		for _, id := range list("Default Applications") {
			if !c.removed[id] {
				c.defaults = append(c.defaults, id)
			}
		}
		for _, id := range list("Added Associations") {
			if !c.removed[id] {
				c.added = append(c.added, id)
			}
		}
	}
	return c
}

// xdgAppsForMIME lists the applications for a MIME type.
func xdgAppsForMIME(e xdgEnv, mimeType string) []App {
	files := e.desktopFiles()
	choices := e.choicesFor(mimeType)
	lang := e.getenv("LC_ALL")
	if lang == "" {
		lang = e.getenv("LANG")
	}
	lang, _, _ = strings.Cut(lang, ".")
	seen := map[string]bool{}
	var apps []App
	defaultFound := false
	add := func(id string, isDefault bool, rank int) {
		if seen[id] || choices.removed[id] {
			return
		}
		p, ok := files[id]
		if !ok {
			return
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return
		}
		de := ParseDesktopEntry(data, lang)
		if de.Hidden || de.Exec == "" || (de.Type != "" && de.Type != "Application") {
			return
		}
		seen[id] = true
		if isDefault && defaultFound {
			isDefault = false
		}
		if isDefault {
			defaultFound = true
		}
		name := de.Name
		if name == "" {
			name = strings.TrimSuffix(id, ".desktop")
		}
		apps = append(apps, App{Name: name, Path: p, Exec: de.Exec, Icon: de.Icon, Terminal: de.Terminal, Default: isDefault, rank: rank})
	}
	for _, id := range choices.defaults {
		add(id, true, 0)
	}
	for _, id := range choices.added {
		add(id, false, 0)
	}
	major, _, _ := strings.Cut(mimeType, "/")
	for id, p := range files {
		if seen[id] {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		de := ParseDesktopEntry(data, "")
		if de.Handles(mimeType) {
			rank := 0
			if !containsFold(de.MimeTypes, mimeType) {
				rank = 1 // only through "major/*"
			}
			add(id, false, rank)
		} else if major == "text" && mimeType != "text/plain" && de.Handles("text/plain") {
			// Every text type is also plain text, so text editors are offered too.
			add(id, false, 2)
		}
	}
	return apps
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// MIMEType guesses the MIME type of a file: from its extension, or for files without a known extension, "text/plain"
// when the first bytes are text and "application/octet-stream" otherwise. Folders are "inode/directory".
func MIMEType(path string) string {
	fi, err := os.Stat(path)
	if err == nil && fi.IsDir() {
		return "inode/directory"
	}
	if t := mime.TypeByExtension(filepath.Ext(path)); t != "" {
		t, _, _ = strings.Cut(t, ";")
		return strings.TrimSpace(t)
	}
	if err == nil && fi.Size() == 0 {
		return "application/x-zerosize"
	}
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	buf = buf[:n]
	if n > 0 && !strings.ContainsRune(string(buf), 0) && (utf8.Valid(buf) || validExceptTail(buf)) {
		return "text/plain"
	}
	return "application/octet-stream"
}

// validExceptTail allows a UTF-8 sequence cut off by the 512-byte read.
func validExceptTail(b []byte) bool {
	for i := 1; i <= 3 && i < len(b); i++ {
		if utf8.Valid(b[:len(b)-i]) {
			return true
		}
	}
	return false
}

// findDesktopEntry loads a desktop entry given as a path or as an id.
func findDesktopEntry(l *Launcher, app string) (DesktopEntry, string, error) {
	p := app
	if !filepath.IsAbs(p) {
		var ok bool
		p, ok = xdgEnv{getenv: l.getenv}.desktopFiles()[app]
		if !ok {
			return DesktopEntry{}, "", errors.New("launch: no application " + app)
		}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return DesktopEntry{}, "", err
	}
	e := ParseDesktopEntry(data, "")
	if e.Exec == "" {
		return DesktopEntry{}, "", errors.New("launch: " + app + " has no command")
	}
	return e, p, nil
}
