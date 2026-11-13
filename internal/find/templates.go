// This file stores saved searches, the Load/Save tab of Find Files (F-537), and the
// last search so a new Find Files dialog starts where the previous one ended (F-542).
// Total Commander keeps these in wincmd.ini; we keep them in one small JSON file in
// the app's config folder, written atomically (temporary file, then rename) so a crash
// in the middle of saving can never leave a half-written file behind. Templates are
// also how saved searches become selection filters ("select group", custom column
// filters): load the template, Compile it, and call Query.Match per file.
//
// Start folders are often not part of a template (TC calls them "search in current
// folder"), so SaveTemplate keeps SearchIn only when the caller asks for it.

package find

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Template is a named saved search.
type Template struct {
	Name    string  `json:"name"`
	Options Options `json:"options"`
}

// storeFile is the on-disk format.
type storeFile struct {
	Version   int        `json:"version"`
	Last      *Options   `json:"last,omitempty"`
	Templates []Template `json:"templates"`
}

// Store is a file of saved searches. Methods are safe for concurrent use.
type Store struct {
	path string
	mu   sync.Mutex
	data storeFile
}

// OpenStore loads a store, creating an empty one if the file does not exist yet.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, data: storeFile{Version: 1}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("find: saved searches file %s is damaged: %w", path, err)
	}
	if s.data.Version == 0 {
		s.data.Version = 1
	}
	return s, nil
}

// Names lists the template names, sorted without regard to case.
func (s *Store) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.data.Templates))
	for _, t := range s.data.Templates {
		out = append(out, t.Name)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

func (s *Store) index(name string) int {
	for i, t := range s.data.Templates {
		if strings.EqualFold(t.Name, name) {
			return i
		}
	}
	return -1
}

// Get returns a template by name (names compare without regard to case).
func (s *Store) Get(name string) (Options, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.index(name); i >= 0 {
		return s.data.Templates[i].Options, true
	}
	return Options{}, false
}

// Save stores (or replaces) a template and writes the file. Unless keepSearchIn is
// set, the start folders are dropped so the template works from any folder. Runtime
// fields (FS, Archives, Now, Within) are never saved.
func (s *Store) Save(name string, o Options, keepSearchIn bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("find: a saved search needs a name")
	}
	o = storable(o)
	if !keepSearchIn {
		o.SearchIn = nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := Template{Name: name, Options: o}
	if i := s.index(name); i >= 0 {
		s.data.Templates[i] = t
	} else {
		s.data.Templates = append(s.data.Templates, t)
	}
	return s.write()
}

// Delete removes a template; deleting a missing name is not an error.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.index(name)
	if i < 0 {
		return nil
	}
	s.data.Templates = append(s.data.Templates[:i], s.data.Templates[i+1:]...)
	return s.write()
}

// Rename gives a template a new name.
func (s *Store) Rename(oldName, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return errors.New("find: a saved search needs a name")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.index(oldName)
	if i < 0 {
		return fmt.Errorf("find: no saved search named %q", oldName)
	}
	if j := s.index(newName); j >= 0 && j != i {
		return fmt.Errorf("find: a saved search named %q already exists", newName)
	}
	s.data.Templates[i].Name = newName
	return s.write()
}

// SetLast remembers the most recent search (with its start folders).
func (s *Store) SetLast(o Options) error {
	o = storable(o)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Last = &o
	return s.write()
}

// Last returns the most recent search, if any.
func (s *Store) Last() (Options, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Last == nil {
		return Options{}, false
	}
	return *s.data.Last, true
}

// storable strips the fields that only make sense while a search runs.
func storable(o Options) Options {
	o.FS, o.Archives, o.Now, o.Within = nil, nil, nil, nil
	return o
}

// write saves the file atomically. Callers hold s.mu.
func (s *Store) write() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".searches-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(b, '\n'))
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmp.Name())
		return werr
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
