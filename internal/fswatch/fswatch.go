// Package fswatch tells the file panels when the folder they show has changed on disk, so the panel can reread it
// without the user pressing Ctrl+R. Total Commander does this with the Windows change notification API and lets the
// user switch it off globally or for chosen drives and folders; network drives are often unreliable with native
// notifications, so they are watched by polling instead. This package is the UI-free engine behind that: the app asks
// it to Watch the folder of each panel, reads Events from one channel, and rereads the matching panel. Raw operating
// system notifications arrive in bursts (a copy of 1000 files produces thousands), so they are debounced per folder:
// one Event is sent after a short quiet period, but never later than MaxDelay after the first change, so a file that
// another program keeps writing still shows up within about a second. Watching is not recursive on purpose: a panel
// only shows one folder level, and recursive watches are expensive on every platform.
package fswatch

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Method says how a folder is watched.
type Method int

const (
	// MethodDefault means "use the default for this kind of drive": native for local drives, polling for network
	// drives. It is only meaningful in Options; Status never reports it.
	MethodDefault Method = iota
	// MethodNative uses the operating system notifications (inotify, kqueue, ReadDirectoryChangesW). If they cannot
	// be set up for a folder the watcher falls back to polling and says why in Status.Reason.
	MethodNative
	// MethodPoll rereads the folder every PollInterval and compares names, sizes, times and modes.
	MethodPoll
	// MethodOff does not watch at all; the user refreshes by hand.
	MethodOff
)

// String returns a short lower-case name for the method, used in logs and status lines.
func (m Method) String() string {
	switch m {
	case MethodNative:
		return "native"
	case MethodPoll:
		return "poll"
	case MethodOff:
		return "off"
	}
	return "default"
}

// Kind says what an Event means for the panel.
type Kind int

const (
	// Changed means entries in the folder were added, removed, renamed or modified; the panel should reread it.
	Changed Kind = iota
	// Gone means the watched folder itself was deleted, renamed away or unmounted. Total Commander then switches the
	// panel to the nearest existing parent; the watch for this path stays registered but inactive until Unwatch.
	Gone
	// Error reports a problem that did not stop the watch, for example a network folder that could not be read
	// during one poll. The panel usually just shows it in the status line.
	Error
)

// String returns a short lower-case name for the kind.
func (k Kind) String() string {
	switch k {
	case Changed:
		return "changed"
	case Gone:
		return "gone"
	}
	return "error"
}

// Event is one debounced notification for one watched folder.
type Event struct {
	// Path is the folder exactly as it was passed to Watch (cleaned and made absolute).
	Path string
	Kind Kind
	// Names lists the entry names (not full paths) that changed, sorted, when the backend knew them. It is a hint
	// for keeping the cursor or flashing changed rows; the panel should still reread the whole folder.
	Names []string
	// Full is true when the backend could not say which entries changed (queue overflow, re-enabled watch, unknown
	// directory change). Names is then empty.
	Full bool
	// Err is set for Kind == Error and may explain a Gone.
	Err error
}

// Options configures a Watcher. The zero value is usable and matches Total Commander's defaults: auto refresh on,
// native notifications for local drives, polling every 2 seconds for network drives.
type Options struct {
	// Debounce is the quiet period after the last raw notification before an Event is sent. Default 200 ms.
	Debounce time.Duration
	// MaxDelay caps how long a continuous stream of changes can hold an Event back. Default 1 s (FEATURES F-065
	// asks for new files to appear within about a second).
	MaxDelay time.Duration
	// PollInterval is how often polled folders are reread. Default 2 s.
	PollInterval time.Duration
	// Local is the method for folders on local drives. MethodDefault means MethodNative.
	Local Method
	// Network is the method for folders on network drives (SMB, NFS, AFP, WebDAV, FUSE, UNC paths). MethodDefault
	// means MethodPoll, because native notifications on shares miss changes made by other machines.
	Network Method
	// Exclude lists folders that are never watched, like Total Commander's "exclude these drives/folders" field.
	// A plain path excludes that folder and everything below it; an entry with * ? or [ is a filepath.Match
	// pattern tried against the folder and each of its parents. Matching ignores case on Windows.
	Exclude []string
	// Disabled turns automatic refresh off for everything (Total Commander's "Refresh file lists automatically"
	// checkbox unticked). It can be changed later with SetEnabled.
	Disabled bool
	// PauseWhenInactive holds Events while the app window is inactive (see SetActive) and delivers one merged Event
	// per folder when it becomes active again, so background windows do not reread folders nobody is looking at.
	PauseWhenInactive bool
	// FileWatchLimit is used by the kqueue backend (macOS, BSD), which only sees changes to the folder itself
	// unless each file is opened too. Up to this many regular files per folder get their own watch so growing or
	// rewritten files are noticed; files beyond the limit are only noticed when they are created, removed or
	// renamed. Default 256; a negative value disables per-file watches. Other platforms ignore it.
	FileWatchLimit int
}

func (o Options) withDefaults() Options {
	if o.Debounce <= 0 {
		o.Debounce = 200 * time.Millisecond
	}
	if o.MaxDelay <= 0 {
		o.MaxDelay = time.Second
	}
	if o.MaxDelay < o.Debounce {
		o.MaxDelay = o.Debounce
	}
	if o.PollInterval <= 0 {
		o.PollInterval = 2 * time.Second
	}
	if o.Local == MethodDefault {
		o.Local = MethodNative
	}
	if o.Network == MethodDefault {
		o.Network = MethodPoll
	}
	if o.FileWatchLimit == 0 {
		o.FileWatchLimit = 256
	}
	return o
}

// Status describes how one path is currently watched.
type Status struct {
	// Watched is true when Watch was called for the path and not yet balanced by Unwatch.
	Watched bool
	// Active is true when a backend is running for it right now.
	Active bool
	// Method is the method in use: MethodNative, MethodPoll or MethodOff (not active).
	Method Method
	// Network is true when the path was detected as being on a network drive.
	Network bool
	// Disabled is true when SetPathDisabled turned the path off.
	Disabled bool
	// Excluded is true when the path matches Options.Exclude.
	Excluded bool
	// Reason explains in plain words why the method differs from the preferred one, for example "native watch
	// failed: too many open files; polling instead", or why the path is not active.
	Reason string
}

// ErrClosed is returned by methods called after Close.
var ErrClosed = errors.New("fswatch: watcher is closed")

// backend is one running watch on one folder. stop must be safe to call more than once and from any goroutine
// except the backend's own, and must not return before the backend has stopped calling its report function.
type backend interface {
	stop()
}

// raw is one undebounced notification from a backend. Exactly one of the fields is meaningful; a zero raw
// (empty Name, no flags) means "something changed, names unknown".
type raw struct {
	Name     string
	Overflow bool
	// Terminal means the backend has stopped by itself, because the folder is gone or reading notifications failed.
	// The watcher then checks whether the folder still exists and either restarts the watch or sends Gone.
	Terminal bool
	// Soft is a non-fatal error (polling a share failed once); it becomes an Error event.
	Soft error
	Err  error
}

// startFunc starts a backend for an absolute folder path. It is a variable so tests can force failures.
type startFunc func(path string, opts Options, report func(raw)) (backend, error)

var (
	startNative startFunc = startNativeBackend
	startPoll   startFunc = startPollBackend
	isNetwork             = isNetworkPath
)

// maxNames caps how many names one Event carries; beyond that the Event becomes Full, because a panel would
// reread everything anyway and a huge list only costs memory.
const maxNames = 512

// entry is the state of one watched path.
type entry struct {
	path      string
	refs      int
	network   bool
	method    Method // method of the running backend, MethodOff when none
	reason    string
	be        backend
	gen       uint64 // bumped whenever be changes, so late reports from an old backend are ignored
	gone      bool
	verifying bool

	// Debounce state.
	pending map[string]struct{}
	full    bool
	first   time.Time
	due     time.Time
	armed   bool
	held    bool // a flushed Event is waiting for SetActive(true)
}

// Watcher watches any number of folders and sends debounced Events on one channel. All methods are safe for
// concurrent use.
type Watcher struct {
	mu       sync.Mutex
	opts     Options
	entries  map[string]*entry // key: normalized path
	disabled map[string]string // key: normalized path, value: path as given; survives Unwatch so the switch is remembered
	active   bool
	closed   bool

	queue  []Event // undelivered Events, at most one per path (later ones merge into earlier ones)
	kick   chan struct{}
	out    chan Event
	done   chan struct{}
	pumpWG sync.WaitGroup
}

// New creates a Watcher. It starts no OS resources until Watch is called.
func New(opts Options) *Watcher {
	w := &Watcher{
		opts:     opts.withDefaults(),
		entries:  map[string]*entry{},
		disabled: map[string]string{},
		active:   true,
		kick:     make(chan struct{}, 1),
		out:      make(chan Event, 16),
		done:     make(chan struct{}),
	}
	w.pumpWG.Add(1)
	go w.pump()
	return w
}

// Events returns the channel on which debounced Events arrive. It is closed by Close. The app should read it
// continuously; when it falls behind, Events for the same folder are merged instead of piling up.
func (w *Watcher) Events() <-chan Event { return w.out }

// Watch starts watching a folder. Calls are reference counted per path, so two panels showing the same folder can
// both call Watch and Unwatch independently. The error is only about the path itself (not a folder, not
// readable); a failing native watch silently falls back to polling and is reported in Status.
func (w *Watcher) Watch(path string) error {
	abs, err := cleanPath(path)
	if err != nil {
		return err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return &os.PathError{Op: "watch", Path: abs, Err: errors.New("not a directory")}
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return ErrClosed
	}
	k := key(abs)
	if e, ok := w.entries[k]; ok {
		e.refs++
		w.mu.Unlock()
		return nil
	}
	e := &entry{path: abs, refs: 1, method: MethodOff}
	w.entries[k] = e
	w.mu.Unlock()

	e.network = isNetwork(abs)
	w.restart(e, false)
	return nil
}

// Unwatch releases one Watch call for the path. When the last reference goes, the backend is stopped and any
// pending Event for the path is dropped. Unwatching a path that is not watched is not an error.
func (w *Watcher) Unwatch(path string) error {
	abs, err := cleanPath(path)
	if err != nil {
		return err
	}
	w.mu.Lock()
	k := key(abs)
	e, ok := w.entries[k]
	if !ok {
		w.mu.Unlock()
		return nil
	}
	e.refs--
	if e.refs > 0 {
		w.mu.Unlock()
		return nil
	}
	delete(w.entries, k)
	be := w.detach(e)
	w.dropQueued(e.path)
	w.mu.Unlock()
	if be != nil {
		be.stop()
	}
	return nil
}

// Watched returns the watched paths, sorted.
func (w *Watcher) Watched() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.entries))
	for _, e := range w.entries {
		out = append(out, e.path)
	}
	sort.Strings(out)
	return out
}

// Status reports how a path is watched. It works for unwatched paths too, saying whether they would be disabled or
// excluded.
func (w *Watcher) Status(path string) Status {
	abs, err := cleanPath(path)
	if err != nil {
		return Status{Method: MethodOff, Reason: err.Error()}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	k := key(abs)
	st := Status{Method: MethodOff, Disabled: w.disabled[k] != "", Excluded: excluded(abs, w.opts.Exclude)}
	if e, ok := w.entries[k]; ok {
		st.Watched = true
		st.Active = e.be != nil
		st.Method = e.method
		st.Network = e.network
		st.Reason = e.reason
	}
	return st
}

// SetEnabled switches automatic refresh on or off for all paths. Switching it on restarts every watch and sends a
// Full Changed Event for each, because changes made while it was off were missed.
func (w *Watcher) SetEnabled(on bool) {
	w.mu.Lock()
	if w.closed || w.opts.Disabled == !on {
		w.mu.Unlock()
		return
	}
	w.opts.Disabled = !on
	w.mu.Unlock()
	w.restartAll(on)
}

// SetPathDisabled switches automatic refresh off (true) or back on (false) for one folder, like Total Commander's
// per-drive exclusion but usable from a panel's context menu. The switch is remembered even while the folder is
// not watched. Re-enabling sends a Full Changed Event for the folder if it is watched.
func (w *Watcher) SetPathDisabled(path string, disabled bool) error {
	abs, err := cleanPath(path)
	if err != nil {
		return err
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return ErrClosed
	}
	k := key(abs)
	if (w.disabled[k] != "") == disabled {
		w.mu.Unlock()
		return nil
	}
	if disabled {
		w.disabled[k] = abs
	} else {
		delete(w.disabled, k)
	}
	e := w.entries[k]
	w.mu.Unlock()
	if e != nil {
		w.restart(e, !disabled)
	}
	return nil
}

// DisabledPaths returns the folders switched off with SetPathDisabled, sorted, so the app can save them.
func (w *Watcher) DisabledPaths() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.disabled))
	for _, p := range w.disabled {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// SetExclude replaces Options.Exclude and applies it to the watched paths. Paths that stop being excluded get a
// Full Changed Event.
func (w *Watcher) SetExclude(patterns []string) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.opts.Exclude = append([]string(nil), patterns...)
	w.mu.Unlock()
	w.restartAll(true)
}

// SetMethods changes the local and network methods (MethodDefault keeps the default) and restarts all watches.
func (w *Watcher) SetMethods(local, network Method) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	o := w.opts
	o.Local, o.Network = local, network
	o = o.withDefaults()
	w.opts.Local, w.opts.Network = o.Local, o.Network
	w.mu.Unlock()
	w.restartAll(false)
}

// SetActive tells the watcher whether the app window is active. With Options.PauseWhenInactive, Events are held
// while inactive and delivered (one per folder) as soon as the window is active again. Without it, this only
// records the state.
func (w *Watcher) SetActive(active bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.active == active {
		return
	}
	w.active = active
	if !active {
		return
	}
	for _, e := range w.entries {
		if e.held {
			e.held = false
			w.emitLocked(e)
		}
	}
}

// Close stops all watches and closes the Events channel. It is safe to call more than once.
func (w *Watcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	var stops []backend
	for k, e := range w.entries {
		if be := w.detach(e); be != nil {
			stops = append(stops, be)
		}
		delete(w.entries, k)
	}
	w.queue = nil
	w.mu.Unlock()
	for _, be := range stops {
		be.stop()
	}
	close(w.done)
	w.pumpWG.Wait()
	return nil
}

// restartAll restarts every watched entry, optionally announcing a Full change for each.
func (w *Watcher) restartAll(announce bool) {
	w.mu.Lock()
	list := make([]*entry, 0, len(w.entries))
	for _, e := range w.entries {
		list = append(list, e)
	}
	w.mu.Unlock()
	for _, e := range list {
		w.restart(e, announce)
	}
}

// restart stops the entry's backend (if any) and starts the one its settings call for. With announce, a Full
// Changed Event is sent when the entry ends up active and was not active before (or was switched back on).
func (w *Watcher) restart(e *entry, announce bool) {
	w.mu.Lock()
	if w.closed || w.entries[key(e.path)] != e {
		w.mu.Unlock()
		return
	}
	old := w.detach(e)
	want, reason := w.wantMethod(e)
	opts := w.opts
	e.gen++
	gen := e.gen
	e.gone = false
	w.mu.Unlock()
	if old != nil {
		old.stop()
	}

	var be backend
	method := want
	if want != MethodOff {
		report := func(r raw) { w.report(e, gen, r) }
		var err error
		if want == MethodNative {
			be, err = startNative(e.path, opts, report)
			if err != nil {
				reason = "native watch failed: " + err.Error() + "; polling instead"
				method = MethodPoll
			}
		}
		if method == MethodPoll {
			be, err = startPoll(e.path, opts, report)
			if err != nil {
				reason = "cannot watch: " + err.Error()
				method = MethodOff
				be = nil
			}
		}
	}

	w.mu.Lock()
	if w.closed || w.entries[key(e.path)] != e || e.gen != gen {
		// Unwatched, closed or restarted again while we were starting; throw this backend away.
		w.mu.Unlock()
		if be != nil {
			be.stop()
		}
		return
	}
	e.be = be
	e.method = method
	e.reason = reason
	if be != nil && announce {
		e.full = true
		w.scheduleLocked(e, true)
	}
	w.mu.Unlock()
}

// wantMethod decides the method for an entry from the global switch, exclusions, the per-path switch and the
// drive kind. The reason is empty when nothing unusual applies.
func (w *Watcher) wantMethod(e *entry) (Method, string) {
	switch {
	case w.opts.Disabled:
		return MethodOff, "automatic refresh is off"
	case w.disabled[key(e.path)] != "":
		return MethodOff, "automatic refresh is off for this folder"
	case excluded(e.path, w.opts.Exclude):
		return MethodOff, "folder is excluded from automatic refresh"
	}
	if e.network {
		return w.opts.Network, ""
	}
	return w.opts.Local, ""
}

// detach takes the backend out of the entry (caller holds mu and must stop the returned backend after
// unlocking) and resets the debounce state.
func (w *Watcher) detach(e *entry) backend {
	be := e.be
	e.be = nil
	e.method = MethodOff
	e.gen++
	e.pending = nil
	e.full = false
	e.armed = false
	e.first = time.Time{}
	e.held = false
	return be
}

// report receives raw notifications from a backend goroutine.
func (w *Watcher) report(e *entry, gen uint64, r raw) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || e.gen != gen || w.entries[key(e.path)] != e {
		return
	}
	switch {
	case r.Terminal:
		if e.verifying {
			return
		}
		e.verifying = true
		be := e.be
		e.be = nil
		e.gen++
		go w.verify(e, be, r.Err)
	case r.Soft != nil:
		w.enqueueLocked(Event{Path: e.path, Kind: Error, Err: r.Soft})
	case r.Overflow || r.Name == "":
		e.full = true
		w.scheduleLocked(e, false)
	default:
		if !e.full {
			if e.pending == nil {
				e.pending = map[string]struct{}{}
			}
			e.pending[r.Name] = struct{}{}
			if len(e.pending) > maxNames {
				e.full = true
				e.pending = nil
			}
		}
		w.scheduleLocked(e, false)
	}
}

// verify runs after a backend stopped by itself. If the folder still exists (it was replaced, or the notification
// read failed for another reason) the watch is restarted and a Full change is sent; otherwise Gone is sent.
func (w *Watcher) verify(e *entry, old backend, cause error) {
	if old != nil {
		old.stop()
	}
	fi, err := os.Stat(e.path)
	w.mu.Lock()
	e.verifying = false
	if w.closed || w.entries[key(e.path)] != e {
		w.mu.Unlock()
		return
	}
	if err == nil && fi.IsDir() {
		w.mu.Unlock()
		w.restart(e, true)
		return
	}
	if err == nil {
		err = errors.New("not a directory any more")
	}
	if cause == nil {
		cause = err
	}
	e.method = MethodOff
	e.gone = true
	e.reason = "folder is gone"
	e.pending, e.full, e.armed, e.held = nil, false, false, false
	w.dropQueued(e.path)
	w.enqueueLocked(Event{Path: e.path, Kind: Gone, Err: cause})
	w.mu.Unlock()
}

// scheduleLocked arms or extends the debounce timer for an entry. now=true flushes at once (used when announcing a
// restart, where there is nothing to wait for).
func (w *Watcher) scheduleLocked(e *entry, now bool) {
	t := time.Now()
	if !e.armed && e.first.IsZero() {
		e.first = t
	}
	due := t.Add(w.opts.Debounce)
	if limit := e.first.Add(w.opts.MaxDelay); due.After(limit) {
		due = limit
	}
	if now {
		due = t
	}
	e.due = due
	if e.armed {
		return
	}
	e.armed = true
	gen := e.gen
	time.AfterFunc(time.Until(due), func() { w.fire(e, gen) })
}

// fire runs when a debounce timer expires.
func (w *Watcher) fire(e *entry, gen uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || e.gen != gen || !e.armed {
		return
	}
	if d := time.Until(e.due); d > 0 {
		time.AfterFunc(d, func() { w.fire(e, gen) })
		return
	}
	e.armed = false
	e.first = time.Time{}
	if w.opts.PauseWhenInactive && !w.active {
		e.held = true
		return
	}
	w.emitLocked(e)
}

// emitLocked turns the entry's pending state into an Event.
func (w *Watcher) emitLocked(e *entry) {
	if !e.full && len(e.pending) == 0 {
		return
	}
	ev := Event{Path: e.path, Kind: Changed, Full: e.full}
	if !e.full {
		ev.Names = make([]string, 0, len(e.pending))
		for n := range e.pending {
			ev.Names = append(ev.Names, n)
		}
		sort.Strings(ev.Names)
	}
	e.pending, e.full = nil, false
	w.enqueueLocked(ev)
}

// enqueueLocked adds an Event to the delivery queue, merging it into an undelivered Event for the same path.
func (w *Watcher) enqueueLocked(ev Event) {
	for i := range w.queue {
		q := &w.queue[i]
		if q.Path != ev.Path || q.Kind != ev.Kind {
			continue
		}
		switch ev.Kind {
		case Changed:
			if q.Full || ev.Full {
				q.Full, q.Names = true, nil
			} else {
				q.Names = mergeNames(q.Names, ev.Names)
				if len(q.Names) > maxNames {
					q.Full, q.Names = true, nil
				}
			}
		default:
			q.Err = ev.Err
		}
		return
	}
	w.queue = append(w.queue, ev)
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// dropQueued removes undelivered Changed/Error Events for a path (caller holds mu).
func (w *Watcher) dropQueued(path string) {
	q := w.queue[:0]
	for _, ev := range w.queue {
		if ev.Path != path || ev.Kind == Gone {
			q = append(q, ev)
		}
	}
	w.queue = q
}

// pump moves queued Events to the output channel without ever blocking while holding mu.
func (w *Watcher) pump() {
	defer w.pumpWG.Done()
	defer close(w.out)
	for {
		w.mu.Lock()
		var ev Event
		have := len(w.queue) > 0
		if have {
			ev = w.queue[0]
			w.queue = w.queue[1:]
		}
		w.mu.Unlock()
		if !have {
			select {
			case <-w.kick:
				continue
			case <-w.done:
				return
			}
		}
		select {
		case w.out <- ev:
		case <-w.done:
			return
		}
	}
}

func mergeNames(a, b []string) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for _, n := range a {
		set[n] = struct{}{}
	}
	for _, n := range b {
		set[n] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// cleanPath makes a path absolute and clean so the same folder always maps to the same entry.
func cleanPath(p string) (string, error) {
	if p == "" {
		return "", errors.New("fswatch: empty path")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// key is the map key of a cleaned absolute path. Windows paths are case-insensitive, so they are folded; other
// systems keep case because case-sensitive volumes exist on macOS and are the norm on Linux.
func key(abs string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(abs)
	}
	return abs
}
