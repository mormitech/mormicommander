package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings are the user's choices (keymap overrides, panel paths and sort, layout toggles...), kept as one JSON object
// in settings.json in the settings folder. Each feature uses its own top-level key, so features never step on each
// other: "keymap", "session", "hotlist", ... The page reads all of it once at start and writes a key when it changes;
// every write goes to disk at once (tmp + rename), so nothing is lost when the app is killed.
//
// (F-443 asks for a wincmd.ini-style file; the configuration feature can switch the storage format behind this same
// Get/Set interface without touching other features.)

type Settings struct {
	mu      sync.Mutex
	path    string
	data    map[string]json.RawMessage
	changed func(key string, value any, from string) // told after every change (the "setting" event to the pages)
}

func init() {
	registerFeature("settings", func(a *App) []any {
		a.settings = loadSettings(settingsPath())
		// Every window of the app keeps a copy of the settings (frontend/src/core/settings.js); a change made in one
		// (a Lister window, the file manager) reaches the others as the "setting" event, so no copy goes stale.
		// (the window that made the change is not told: its copy is changed already, and an echo arriving after a newer
		// change would put the older value back for a moment)
		a.settings.changed = func(key string, value any, from string) { a.emitExcept(from, "setting", key, value) }
		return []any{a.settings}
	})
}

func settingsPath() string {
	if dir := configDir(); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	return ""
}

func loadSettings(path string) *Settings {
	s := &Settings{path: path, data: map[string]json.RawMessage{}}
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			json.Unmarshal(b, &s.data) // a broken file starts empty rather than stopping the app
		}
	}
	return s
}

// All returns every setting as one object.
func (s *Settings) All() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{}
	for k, v := range s.data {
		var x any
		if json.Unmarshal(v, &x) == nil {
			out[k] = x
		}
	}
	return out
}

// Get returns one setting, or nil when it is not set.
func (s *Settings) Get(key string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	var x any
	if v, ok := s.data[key]; ok && json.Unmarshal(v, &x) == nil {
		return x
	}
	return nil
}

// Set stores one setting and writes the file (nil removes the key). The other windows of the app are told.
func (s *Settings) Set(ctx context.Context, key string, value any) error {
	from := ""
	if w := windowOf(ctx); w != nil {
		from = w.Name()
	}
	return s.set(key, value, from)
}

// put is Set for the Go side (every window is told).
func (s *Settings) put(key string, value any) error { return s.set(key, value, "") }

func (s *Settings) set(key string, value any, from string) error {
	s.mu.Lock()
	if value == nil {
		delete(s.data, key)
	} else {
		b, err := json.Marshal(value)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		s.data[key] = b
	}
	err := s.save()
	changed := s.changed
	s.mu.Unlock()
	if changed != nil {
		changed(key, value, from)
	}
	return err
}

// Path is where the settings file is (for "edit settings files directly" and the docs).
func (s *Settings) Path() string { return s.path }

func (s *Settings) save() error {
	if s.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return writeAtomic(s.path, b)
}
