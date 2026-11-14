package main

// Editing a file inside an archive (F4 on an archive entry; FEATURES.md F-592, TC's "edit, then repack"). The entry is
// unpacked to a temporary copy, the copy opens in the editor, and the copy is then watched: whenever it changes (the
// editor saved it), Go sends the page an "archive-edit-changed" event and the page asks whether to put the new version
// back into the archive. The answer comes back through ListerEdit.AnswerArchiveEdit, which packs the copy over the
// entry (or leaves the archive as it is) and keeps watching, so every later save asks again. Watching ends when the
// app ends; the temporary copies are removed then too. Archives that can't be changed (7z, RAR, an archive inside
// another one) still open their entry in the editor, with a note that the changes stay in the temporary copy.
//
// Before this existed F4 on an archive entry only said that editing inside archives was not ready yet, so nothing
// that worked before behaves differently; files on servers still get that answer.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"mormicommander/internal/archive"
	"mormicommander/internal/vfs"
)

// arcEditPollEvery is how often the temporary copies are checked for changes.
const arcEditPollEvery = time.Second

// ListerEditResult tells the page what F4 did: Watching is true when a temporary copy of an archive entry is watched
// for changes; Note is a sentence to show (for example why the changes can't go back into the archive).
type ListerEditResult struct {
	Watching bool   `json:"watching"`
	Note     string `json:"note"`
}

// arcEditChange is the "archive-edit-changed" event: the temporary copy of Entry (a full panel path inside Archive)
// was saved; the page answers with AnswerArchiveEdit(ID, update).
type arcEditChange struct {
	ID      int    `json:"id"`
	Entry   string `json:"entry"`
	Archive string `json:"archive"`
	Name    string `json:"name"`
}

// arcEditWatch is one watched temporary copy and what it looked like when it was last unpacked, packed or declined.
type arcEditWatch struct {
	id      int
	entry   string // the entry's full path (the archive's path, then the path inside it)
	archive string // the archive file (its root), for the question and for refreshing panels
	local   string // the temporary copy the editor has
	mtime   time.Time
	size    int64
	sum     [32]byte
	pending bool // the page is asking about this change; no new question until it is answered
}

// arcEditWatcher keeps the watched copies. scan finds the changed ones; answer packs or declines one change. upload
// and emit are fields so tests can run it without a window or a running app.
type arcEditWatcher struct {
	mu      sync.Mutex
	seq     int
	watches map[int]*arcEditWatch
	upload  func(local, entry string) error
	emit    func(arcEditChange)
	stop    chan struct{}
	running bool
}

func newArcEditWatcher(upload func(local, entry string) error, emit func(arcEditChange)) *arcEditWatcher {
	return &arcEditWatcher{watches: map[int]*arcEditWatch{}, upload: upload, emit: emit, stop: make(chan struct{})}
}

// fileState reads a file's modification time, size and content checksum.
func arcEditFileState(p string) (time.Time, int64, [32]byte, error) {
	var sum [32]byte
	fi, err := os.Stat(p)
	if err != nil {
		return time.Time{}, 0, sum, err
	}
	f, err := os.Open(p)
	if err != nil {
		return time.Time{}, 0, sum, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return time.Time{}, 0, sum, err
	}
	copy(sum[:], h.Sum(nil))
	return fi.ModTime(), fi.Size(), sum, nil
}

// add starts watching the temporary copy local of entry (inside archive) and gives the watch's id.
func (w *arcEditWatcher) add(entry, archive, local string) (int, error) {
	mt, sz, sum, err := arcEditFileState(local)
	if err != nil {
		return 0, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	// F4 again on the same entry while its copy is still watched: the old watch gives way to the new copy.
	for id, x := range w.watches {
		if x.entry == entry {
			delete(w.watches, id)
		}
	}
	w.seq++
	w.watches[w.seq] = &arcEditWatch{id: w.seq, entry: entry, archive: archive, local: local, mtime: mt, size: sz, sum: sum}
	return w.seq, nil
}

// remove stops watching id.
func (w *arcEditWatcher) remove(id int) {
	w.mu.Lock()
	delete(w.watches, id)
	w.mu.Unlock()
}

// scan checks every watched copy and asks about each one that changed since it was last unpacked, packed or declined
// (a copy whose time changed but whose content did not is not a change). It returns the questions it sent.
func (w *arcEditWatcher) scan() []arcEditChange {
	w.mu.Lock()
	var todo []*arcEditWatch
	for _, x := range w.watches {
		if !x.pending {
			todo = append(todo, x)
		}
	}
	w.mu.Unlock()
	sort.Slice(todo, func(i, j int) bool { return todo[i].id < todo[j].id })
	var out []arcEditChange
	for _, x := range todo {
		fi, err := os.Stat(x.local)
		if err != nil {
			continue // (an editor that saves by replacing the file can leave it missing for a moment)
		}
		w.mu.Lock()
		same := fi.ModTime().Equal(x.mtime) && fi.Size() == x.size
		w.mu.Unlock()
		if same {
			continue
		}
		mt, sz, sum, err := arcEditFileState(x.local)
		if err != nil {
			continue
		}
		w.mu.Lock()
		if w.watches[x.id] != x || x.pending {
			w.mu.Unlock()
			continue
		}
		if bytes.Equal(sum[:], x.sum[:]) {
			x.mtime, x.size = mt, sz // touched, not changed
			w.mu.Unlock()
			continue
		}
		x.pending = true
		c := arcEditChange{ID: x.id, Entry: x.entry, Archive: x.archive, Name: filepath.Base(x.local)}
		w.mu.Unlock()
		out = append(out, c)
		if w.emit != nil {
			w.emit(c)
		}
	}
	return out
}

// answer settles the question about watch id: with update, the copy is packed over the entry. Either way the copy's
// current state becomes the new starting point, so the next save asks again. A failed update keeps the old starting
// point, so the change is offered again on the next check.
func (w *arcEditWatcher) answer(id int, update bool) error {
	w.mu.Lock()
	x := w.watches[id]
	w.mu.Unlock()
	if x == nil {
		return errors.New("This file is no longer watched for changes.")
	}
	mt, sz, sum, statErr := arcEditFileState(x.local)
	if update {
		if statErr != nil {
			w.setPending(x, false)
			return fmt.Errorf("Can't read the edited copy of %s: %v", filepath.Base(x.local), statErr)
		}
		if err := w.upload(x.local, x.entry); err != nil {
			w.setPending(x, false)
			return fmt.Errorf("Can't update %s in %s: %s", filepath.Base(x.local), vfsBase(x.archive), vfsErrText(err))
		}
	}
	w.mu.Lock()
	if statErr == nil {
		x.mtime, x.size, x.sum = mt, sz, sum
	}
	x.pending = false
	w.mu.Unlock()
	return nil
}

func (w *arcEditWatcher) setPending(x *arcEditWatch, v bool) {
	w.mu.Lock()
	x.pending = v
	w.mu.Unlock()
}

// start runs scan every arcEditPollEvery until close (once; later calls do nothing).
func (w *arcEditWatcher) start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()
	go func() {
		t := time.NewTicker(arcEditPollEvery)
		defer t.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-t.C:
				w.scan()
			}
		}
	}()
}

// close stops the checks.
func (w *arcEditWatcher) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	select {
	case <-w.stop:
	default:
		close(w.stop)
	}
}

var arcEditByApp sync.Map // *App → *arcEditWatcher

func init() {
	onShutdown(func(a *App) {
		if v, ok := arcEditByApp.LoadAndDelete(a); ok {
			v.(*arcEditWatcher).close()
		}
	})
}

// arcEditWatcherOf is the app's watcher, made on first use with the panels' router.
func arcEditWatcherOf(a *App, r *vfs.Router) *arcEditWatcher {
	if v, ok := arcEditByApp.Load(a); ok {
		return v.(*arcEditWatcher)
	}
	w := newArcEditWatcher(func(local, entry string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_, err := r.Upload(ctx, local, r.Clean(entry), vfs.TransferOptions{PreserveTime: true})
		return err
	}, func(c arcEditChange) { a.emit("archive-edit-changed", c) })
	v, _ := arcEditByApp.LoadOrStore(a, w)
	return v.(*arcEditWatcher)
}

// arcEditPrepare unpacks the archive entry path to a temporary copy for the editor. readOnly is the sentence that says
// why the archive can't take the changes back ("" when it can); arc is the archive file the entry belongs to.
func arcEditPrepare(r *vfs.Router, path string) (local, arc, readOnly string, err error) {
	path = r.Clean(path)
	e, err := r.Stat(path)
	if err != nil {
		return "", "", "", errors.New(listError(path, err))
	}
	if e.Flags&vfs.FlagDir != 0 {
		return "", "", "", fmt.Errorf("%s is a folder, not a file.", e.Name)
	}
	arc = path
	if ai, ierr := r.Archives.Info(path); ierr == nil && ai.Root != "" {
		arc = ai.Root
	}
	readOnly = vfsArchiveReadOnly(r, path, false)
	dir, err := vfsTempDir("edit")
	if err != nil {
		return "", "", "", err
	}
	local = filepath.Join(dir, archive.SanitizeName(e.Name))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if _, err := r.Download(ctx, path, local, vfs.TransferOptions{PreserveTime: true}); err != nil {
		return "", "", "", fmt.Errorf("Can't unpack %s: %s", e.Name, vfsErrText(err))
	}
	return local, arc, readOnly, nil
}

// AnswerArchiveEdit answers the "archive-edit-changed" question id: update=true packs the edited copy back into the
// archive, false leaves the archive as it is. Later saves ask again either way.
func (e *ListerEdit) AnswerArchiveEdit(id int, update bool) error {
	v, ok := arcEditByApp.Load(e.app)
	if !ok {
		return errors.New("This file is no longer watched for changes.")
	}
	return v.(*arcEditWatcher).answer(id, update)
}
