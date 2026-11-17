package main

// Notes and colour labels (board issue #147): the user can give any file or folder a short note and a colour label.
// They are kept in mormiCommander's own small database, notes.json in the settings folder, never in the files or their
// extended attributes, so they work on every disk, read-only ones and servers too. Each entry is keyed by the item's
// full path; items on servers by their URL (without a password).
//
// In the page they are called "notes", not "comments", so they are not confused with Total Commander's descript.ion
// comments (cm_EditComment, Ctrl+Z, internal/listing/comments.go), which stay as they are and are kept apart.
//
// The page (frontend/src/features/notes.js) asks only for the rows on screen (Lookup, in batches), sets notes and
// colours for the selection, and cleans up entries of items that are gone. Renames and moves done inside
// mormiCommander take the entries along: the rename in place (Jobs.Rename), the multi-rename tool, the move jobs (seen
// through the job log, like the undo journal sees them) and the renames an undo does. After such a change the page
// gets a "notes-changed" event and asks again.

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/config"
	"mormicommander/internal/fileops"
	"mormicommander/internal/undo"
)

// Notes is what the page calls for notes and colour labels (window.go.main.Notes).
type Notes struct {
	app *App

	mu     sync.Mutex
	loaded bool
	items  map[string]*noteRec // by noteKey
	timer  *time.Timer         // a save waiting after a move seen in a job
}

// NoteInfo is the note and the colour label of one item ("" when it has none).
type NoteInfo struct {
	Note  string `json:"note,omitempty"`
	Color string `json:"color,omitempty"` // "#rrggbb"
}

// noteRec is one entry of notes.json.
type noteRec struct {
	Path    string `json:"path"`
	Note    string `json:"note,omitempty"`
	Color   string `json:"color,omitempty"`
	Changed int64  `json:"changed,omitempty"` // Unix milliseconds
}

type notesFile struct {
	Version int        `json:"version"`
	Items   []*noteRec `json:"items"`
}

// NotesClean is what a clean-up found: the entries whose items are gone, and how many were not checked.
type NotesClean struct {
	Gone      []string `json:"gone"`
	Servers   int      `json:"servers"`   // entries on servers, which are not checked
	Unplugged int      `json:"unplugged"` // entries on disks that are not connected now, kept
	Total     int      `json:"total"`
}

// notesInstance is the one Notes object, for the job log, the rename in place and undo (nil before it is made).
var notesInstance *Notes

const notesMaxNote = 4000 // characters of one note

var noteColorRx = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func init() {
	registerFeature("notes", func(a *App) []exposed {
		n := &Notes{app: a}
		notesInstance = n
		return []exposed{expose(n)}
	})
	onShutdown(func(a *App) {
		if n := notesInstance; n != nil {
			n.mu.Lock()
			if n.timer != nil && n.timer.Stop() {
				n.saveLocked()
			}
			n.mu.Unlock()
		}
	})
}

func notesPath() string {
	d := configDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "notes.json")
}

// isURLPath says whether p is a server path (scheme://…).
func isURLPath(p string) bool {
	i := strings.Index(p, "://")
	if i <= 0 {
		return false
	}
	for _, c := range p[:i] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '.' || c == '-') {
			return false
		}
	}
	return true
}

// notePath is the spelling of a path that is stored: a local path cleaned, a URL without its password and without a
// trailing slash.
func notePath(p string) string {
	if p == "" {
		return ""
	}
	if isURLPath(p) {
		if u, err := url.Parse(p); err == nil && u.User != nil {
			if _, has := u.User.Password(); has {
				u.User = url.User(u.User.Username())
				p = u.String()
			}
		}
		if t := strings.TrimRight(p, "/"); !strings.HasSuffix(t, ":/") {
			p = t
		}
		return p
	}
	return filepath.Clean(p)
}

// noteKey says when two spellings are the same item: the stored spelling, without regard to case for local paths on
// Windows and macOS, whose disks ignore case by default.
func noteKey(p string) string {
	p = notePath(p)
	if !isURLPath(p) && (runtime.GOOS == "windows" || runtime.GOOS == "darwin") {
		p = strings.ToLower(p)
	}
	return p
}

// noteSep is the separator inside the path p.
func noteSep(p string) string {
	if isURLPath(p) {
		return "/"
	}
	return string(filepath.Separator)
}

// loadLocked reads notes.json the first time it is needed.
func (n *Notes) loadLocked() {
	if n.loaded {
		return
	}
	n.loaded = true
	n.items = map[string]*noteRec{}
	p := notesPath()
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var f notesFile
	if json.Unmarshal(b, &f) != nil {
		// a damaged file is kept aside, so the next save does not throw away what could still be got back
		_ = os.Rename(p, p+".damaged")
		return
	}
	for _, r := range f.Items {
		if r == nil || r.Path == "" || (r.Note == "" && r.Color == "") {
			continue
		}
		r.Path = notePath(r.Path)
		n.items[noteKey(r.Path)] = r
	}
}

// saveLocked writes notes.json at once (through a temporary file, so a crash never leaves half a file).
func (n *Notes) saveLocked() error {
	if n.timer != nil {
		n.timer.Stop()
		n.timer = nil
	}
	p := notesPath()
	if p == "" {
		return nil
	}
	f := notesFile{Version: 1, Items: make([]*noteRec, 0, len(n.items))}
	for _, r := range n.items {
		f.Items = append(f.Items, r)
	}
	sort.Slice(f.Items, func(i, j int) bool { return f.Items[i].Path < f.Items[j].Path })
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(p, b, 0o600)
}

// saveSoon writes notes.json a moment later, so a job that moves many files writes it once.
func (n *Notes) saveSoonLocked() {
	if n.timer != nil {
		return
	}
	n.timer = time.AfterFunc(700*time.Millisecond, func() {
		n.mu.Lock()
		n.timer = nil
		_ = n.saveLocked()
		n.mu.Unlock()
		n.app.emit("notes-changed")
	})
}

func notesSaveErr(err error) error {
	if err == nil {
		return nil
	}
	return errors.New("The notes could not be saved: " + fileopsPlain(err) + ".")
}

// Lookup returns the notes and colour labels of the paths given that have any, keyed by the path as it was given.
func (n *Notes) Lookup(paths []string) map[string]NoteInfo {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.loadLocked()
	out := map[string]NoteInfo{}
	if len(n.items) == 0 {
		return out
	}
	for _, p := range paths {
		if r := n.items[noteKey(p)]; r != nil {
			out[p] = NoteInfo{Note: r.Note, Color: r.Color}
		}
	}
	return out
}

// Get returns the note and the colour label of one path.
func (n *Notes) Get(path string) NoteInfo {
	return n.Lookup([]string{path})[path]
}

// change applies f to the entry of each path (made when missing) and drops entries left empty.
func (n *Notes) change(paths []string, f func(r *noteRec)) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.loadLocked()
	now := time.Now().UnixMilli()
	for _, p := range paths {
		if p == "" {
			continue
		}
		k := noteKey(p)
		r := n.items[k]
		if r == nil {
			r = &noteRec{Path: notePath(p)}
		}
		f(r)
		r.Changed = now
		if r.Note == "" && r.Color == "" {
			delete(n.items, k)
		} else {
			n.items[k] = r
		}
	}
	return notesSaveErr(n.saveLocked())
}

// SetNote gives every path the note ("" removes the note).
func (n *Notes) SetNote(paths []string, note string) error {
	note = strings.TrimSpace(strings.ReplaceAll(note, "\x00", ""))
	if len([]rune(note)) > notesMaxNote {
		return errors.New("The note is too long (at most 4000 characters).")
	}
	return n.change(paths, func(r *noteRec) { r.Note = note })
}

// SetColor gives every path the colour label "#rrggbb" ("" removes the colour).
func (n *Notes) SetColor(paths []string, color string) error {
	color = strings.TrimSpace(color)
	if color != "" && !noteColorRx.MatchString(color) {
		return errors.New("A colour is written as #rrggbb.")
	}
	color = strings.ToLower(color)
	return n.change(paths, func(r *noteRec) { r.Color = color })
}

// Set gives one path both its note and its colour label (the Properties dialog).
func (n *Notes) Set(path, note, color string) error {
	if err := n.SetNote([]string{path}, note); err != nil {
		return err
	}
	return n.SetColor([]string{path}, color)
}

// notesMountParents are the folders where other disks appear: an entry whose nearest folder that is still there is one
// of these is on a disk that is not connected now, and is kept.
func notesUnplugged(p string) bool {
	d := filepath.Dir(p)
	for {
		if _, err := os.Lstat(d); err == nil {
			break
		}
		up := filepath.Dir(d)
		if up == d {
			return true // nothing of the path is there (a Windows drive that is not connected)
		}
		d = up
	}
	d = filepath.ToSlash(d)
	switch {
	case d == "/" || strings.HasSuffix(d, ":/") || d == "/Volumes" || d == "/media" || d == "/mnt" || d == "/run/media":
		return true
	case strings.Count(d, "/") == 2 && (strings.HasPrefix(d, "/media/") || strings.HasPrefix(d, "/run/media/")):
		return true
	case strings.Count(d, "/") == 3 && strings.HasPrefix(d, "/run/media/"):
		return true
	}
	return false
}

// CleanPreview lists the entries whose files and folders are gone. Entries on servers are not checked, nor entries on
// disks that are not connected now.
func (n *Notes) CleanPreview() NotesClean {
	n.mu.Lock()
	list := make([]string, 0, len(n.items))
	n.loadLocked()
	for _, r := range n.items {
		list = append(list, r.Path)
	}
	n.mu.Unlock()
	out := NotesClean{Gone: []string{}, Total: len(list)}
	for _, p := range list {
		if isURLPath(p) {
			out.Servers++
			continue
		}
		if _, err := os.Lstat(p); err == nil || !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if notesUnplugged(p) {
			out.Unplugged++
			continue
		}
		out.Gone = append(out.Gone, p)
	}
	sort.Strings(out.Gone)
	return out
}

// Forget removes the entries of the paths given.
func (n *Notes) Forget(paths []string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.loadLocked()
	for _, p := range paths {
		delete(n.items, noteKey(p))
	}
	return notesSaveErr(n.saveLocked())
}

// moveLocked moves the entry of from, and the entries of everything inside it, to to. It says whether any moved.
func (n *Notes) moveLocked(from, to string, inside bool) bool {
	n.loadLocked()
	if len(n.items) == 0 || from == "" || to == "" {
		return false
	}
	from, to = notePath(from), notePath(to)
	fk, tk := noteKey(from), noteKey(to)
	if fk == tk && from == to {
		return false
	}
	moved := false
	put := func(k string, r *noteRec, path string) {
		delete(n.items, k)
		r.Path = path
		n.items[noteKey(path)] = r
		moved = true
	}
	if r := n.items[fk]; r != nil {
		put(fk, r, to)
	}
	if inside {
		pre := fk + noteSep(from)
		for k, r := range n.items {
			if !strings.HasPrefix(k, pre) {
				continue
			}
			rest := k[len(fk):]
			if len(r.Path) >= len(from) && strings.EqualFold(r.Path[:len(from)], from) {
				rest = r.Path[len(from):]
			}
			if noteSep(to) != noteSep(from) {
				rest = strings.ReplaceAll(rest, noteSep(from), noteSep(to))
			}
			put(k, r, to+rest)
		}
	}
	return moved
}

// moved is called after an item was renamed or moved inside mormiCommander: its entry (and those of what is inside a
// folder) go along. now says whether to save at once (a single rename) or a moment later (a job).
func (n *Notes) moved(from, to string, now bool) {
	if n == nil {
		return
	}
	n.mu.Lock()
	ok := n.moveLocked(from, to, true)
	if ok {
		if now {
			_ = n.saveLocked()
		} else {
			n.saveSoonLocked()
		}
	}
	n.mu.Unlock()
	if ok && now {
		n.app.emit("notes-changed")
	}
}

// movedMany is moved for a batch of renames (the multi-rename tool): deepest paths first, so a file renamed inside a
// folder that was renamed too gets both changes.
func (n *Notes) movedMany(pairs map[string]string) {
	if n == nil || len(pairs) == 0 {
		return
	}
	from := make([]string, 0, len(pairs))
	for f := range pairs {
		from = append(from, f)
	}
	sort.Slice(from, func(i, j int) bool { return len(from[i]) > len(from[j]) })
	n.mu.Lock()
	changed := false
	for _, f := range from {
		if n.moveLocked(f, pairs[f], true) {
			changed = true
		}
	}
	if changed {
		_ = n.saveLocked()
	}
	n.mu.Unlock()
	if changed {
		n.app.emit("notes-changed")
	}
}

// observe follows the job log (feature_jobs.go): what a move job moved, and what any job renamed (an existing
// target put aside), takes its entries along. A move between disks logs each file and folder it made at the new place,
// so each takes its own entry.
func (n *Notes) observe(e fileops.Entry) {
	if n == nil || e.Err != "" || e.Source == "" || e.Target == "" {
		return
	}
	switch {
	case e.Action == "rename", e.Action == "move" && e.Kind == fileops.Move:
		n.moved(e.Source, e.Target, false)
	case e.Action == "mkdir" && e.Kind == fileops.Move:
		n.mu.Lock()
		if n.moveLocked(e.Source, e.Target, false) {
			n.saveSoonLocked()
		}
		n.mu.Unlock()
	}
}

// afterUndo takes the entries back along with the renames an undo did at once (moves come back as jobs, which the job
// log reports).
func (n *Notes) afterUndo(steps []undo.Step) {
	if n == nil {
		return
	}
	pairs := map[string]string{}
	for _, s := range steps {
		if s.Op != undo.OpRename {
			continue
		}
		if _, err := os.Lstat(s.From); err != nil {
			continue
		}
		if _, err := os.Lstat(s.To); err == nil && !strings.EqualFold(s.From, s.To) {
			continue // not put back
		}
		pairs[s.To] = s.From
	}
	n.movedMany(pairs)
}
