package main

// Desktop integration, the watching half: the panels reread their folder when files change on disk (F-065, through
// internal/fswatch), the drive bar follows volumes being mounted and ejected (F-709), and errors can go to a log file
// (F-717). Events for the page: "system-fswatch" {path, kind: "changed"|"gone"|"error", names, full, error} and
// "system-volumes" (the volume list changed).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/disk"
	"mormicommander/internal/fswatch"
)

// SystemWatch is what the page calls for automatic refresh (window.go.main.SystemWatch).
type SystemWatch struct {
	app        *App
	mu         sync.Mutex
	w          *fswatch.Watcher
	keys       map[string]string // panel key ("left", "right") → the folder it watches
	onlyActive bool              // refresh only while the window is in front
	active     bool              // the window is in front
	stop       chan struct{}
}

// SystemLog writes the error log file (window.go.main.SystemLog).
type SystemLog struct {
	app *App
	mu  sync.Mutex
}

// systemWatch is the one watcher of the app (control mode reads it).
var systemWatch *SystemWatch

func init() {
	registerFeature("system-watch", func(a *App) []exposed {
		systemWatch = &SystemWatch{app: a, keys: map[string]string{}}
		return []exposed{expose(systemWatch), expose(&SystemLog{app: a})}
	})
	onStartup(func(a *App) {
		if systemWatch != nil && a.mode == "" {
			systemWatch.start()
		}
	})
	onShutdown(func(a *App) {
		if systemWatch != nil {
			systemWatch.close()
		}
	})
	registerControl("fswatch", func(a *App, m map[string]any) (map[string]any, error) {
		if systemWatch == nil {
			return nil, errors.New("automatic refresh is not running")
		}
		return map[string]any{"ok": true, "watch": systemWatch.State()}, nil
	})
}

// start makes the watcher and the goroutines that pass its events and volume changes on to the page.
func (s *SystemWatch) start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w != nil || s.app.mock != nil {
		return
	}
	// Events are held only while the window is inactive and "only when active" is on (see SetActive).
	s.w = fswatch.New(fswatch.Options{PauseWhenInactive: true})
	s.active = true
	s.stop = make(chan struct{})
	go s.pump(s.w)
	go s.pollVolumes(s.stop)
}

func (s *SystemWatch) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w != nil {
		s.w.Close()
		s.w = nil
	}
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
}

// pump sends the watcher's events to the page until the watcher is closed.
func (s *SystemWatch) pump(w *fswatch.Watcher) {
	for ev := range w.Events() {
		data := map[string]any{"path": ev.Path, "names": ev.Names, "full": ev.Full, "error": ""}
		switch ev.Kind {
		case fswatch.Changed:
			data["kind"] = "changed"
		case fswatch.Gone:
			data["kind"] = "gone"
		default:
			data["kind"] = "error"
		}
		if ev.Err != nil {
			data["error"] = ev.Err.Error()
		}
		s.app.emit("system-fswatch", data)
	}
}

// pollVolumes tells the page when a volume is mounted or ejected (the drive bar updates without F2).
func (s *SystemWatch) pollVolumes(stop chan struct{}) {
	sig := func() string {
		var b strings.Builder
		for _, v := range disk.Volumes() {
			b.WriteString(v.Kind + "\x00" + v.Path + "\x00" + v.Name + "\n")
		}
		return b.String()
	}
	last := sig()
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if now := sig(); now != last {
				last = now
				s.app.emit("system-volumes")
			}
		}
	}
}

// systemWatchable says whether a folder can be watched: a local folder (not in an archive, not on a server).
func systemWatchable(p string) bool {
	return p != "" && !strings.Contains(p, "://") && filepath.IsAbs(p)
}

// Watch makes a panel (key) watch a folder; the folder it watched before is let go. An empty path stops watching.
func (s *SystemWatch) Watch(key, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w == nil {
		return nil
	}
	old := s.keys[key]
	if old == path {
		return nil
	}
	if old != "" {
		s.w.Unwatch(old)
		delete(s.keys, key)
	}
	if !systemWatchable(path) {
		return nil
	}
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return nil
	}
	if err := s.w.Watch(path); err != nil {
		return err
	}
	s.keys[key] = path
	return nil
}

// SystemWatchOptions are the Refresh settings: on/off, only while the window is active, folders not watched, and
// whether network folders are polled.
type SystemWatchOptions struct {
	Enabled    bool     `json:"enabled"`
	OnlyActive bool     `json:"onlyActive"`
	Exclude    []string `json:"exclude"`
	Network    bool     `json:"network"`
}

// Configure applies the Refresh settings.
func (s *SystemWatch) Configure(o SystemWatchOptions) {
	s.mu.Lock()
	w := s.w
	s.mu.Unlock()
	if w == nil {
		return
	}
	ex := make([]string, 0, len(o.Exclude))
	for _, e := range o.Exclude {
		if e = strings.TrimSpace(e); e != "" {
			ex = append(ex, e)
		}
	}
	w.SetExclude(ex)
	network := fswatch.MethodPoll
	if !o.Network {
		network = fswatch.MethodOff
	}
	w.SetMethods(fswatch.MethodNative, network)
	w.SetEnabled(o.Enabled)
	s.mu.Lock()
	s.onlyActive = o.OnlyActive
	active := s.active
	s.mu.Unlock()
	w.SetActive(active || !o.OnlyActive)
}

// SetActive tells the watcher whether the window is in front; with "only when active" changes wait until it is.
func (s *SystemWatch) SetActive(active bool) {
	s.mu.Lock()
	s.active = active
	w, only := s.w, s.onlyActive
	s.mu.Unlock()
	if w != nil {
		w.SetActive(active || !only)
	}
}

// SetPathDisabled turns automatic refresh off (or on again) for one folder.
func (s *SystemWatch) SetPathDisabled(path string, disabled bool) error {
	s.mu.Lock()
	w := s.w
	s.mu.Unlock()
	if w == nil {
		return errors.New("Automatic refresh is not running.")
	}
	return w.SetPathDisabled(path, disabled)
}

// Status tells how a folder is watched (method, network, disabled, excluded, reason).
func (s *SystemWatch) Status(path string) fswatch.Status {
	s.mu.Lock()
	w := s.w
	s.mu.Unlock()
	if w == nil {
		return fswatch.Status{}
	}
	return w.Status(path)
}

// State is the watched folders per panel, for control mode.
func (s *SystemWatch) State() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := map[string]string{}
	for k, v := range s.keys {
		keys[k] = v
	}
	out := map[string]any{"running": s.w != nil, "panels": keys}
	if s.w != nil {
		out["watched"] = s.w.Watched()
	}
	return out
}

// Write appends lines to the log file when the log is on (settings "enableLog", path in "logFile", default
// mormicommander.log in the settings folder). Each line gets the date and time in front. With "logMaxKB" (Options >
// Log file > Start a new log above; 0, the default, never), a log that has reached that size is renamed to the same
// name with ".old" added, replacing the one before, and a new log is started.
func (l *SystemLog) Write(lines []string) error {
	if l.app.settings == nil || l.app.mock != nil {
		return nil
	}
	if on, _ := l.app.settings.Get("enableLog").(bool); !on {
		return nil
	}
	path, _ := l.app.settings.Get("logFile").(string)
	path = l.path(path)
	if path == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if max := logMaxBytes(l.app.settings.Get("logMaxKB")); max > 0 {
		if st, err := os.Stat(path); err == nil && st.Size() >= max {
			_ = os.Remove(path + ".old") // (Windows can't rename over an existing file)
			if err := os.Rename(path, path+".old"); err != nil {
				return err
			}
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	stamp := time.Now().Format("2006-01-02 15:04:05")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(stamp + ": " + strings.ReplaceAll(strings.ReplaceAll(line, "\r", " "), "\n", " ") + "\n")
	}
	_, err = f.WriteString(b.String())
	return err
}

// logMaxBytes is the size in bytes at which the log starts again, from the setting "logMaxKB" (a number of kB, as
// the settings file holds it); 0 means never.
func logMaxBytes(v any) int64 {
	var kb float64
	switch x := v.(type) {
	case float64:
		kb = x
	case int:
		kb = float64(x)
	}
	if kb <= 0 {
		return 0
	}
	return int64(kb) * 1024
}

// Path is the log file that Write uses.
func (l *SystemLog) Path() string {
	p := ""
	if l.app.settings != nil {
		p, _ = l.app.settings.Get("logFile").(string)
	}
	return l.path(p)
}

func (l *SystemLog) path(p string) string {
	p = strings.TrimSpace(p)
	dir := configDir()
	if p == "" {
		if dir == "" {
			return ""
		}
		return filepath.Join(dir, "mormicommander.log")
	}
	p = strings.ReplaceAll(p, "%COMMANDER_PATH%", dir)
	p = os.ExpandEnv(p)
	if strings.HasPrefix(p, "~") {
		if h, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(h, strings.TrimPrefix(strings.TrimPrefix(p, "~"), string(filepath.Separator)))
		}
	}
	return p
}
