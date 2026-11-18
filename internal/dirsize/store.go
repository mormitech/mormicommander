// Store keeps folder sizes between sessions in one file, so a size picture (the folder size treemap, board #24) can
// show the sizes of an earlier count at once while a new count runs. A remembered size can be out of date by any
// amount, so it is never presented as the folder's size: the caller shows it as remembered, with its date, and counts
// again. The file is gzip-compressed gob (standard library only), written to a temporary file and renamed, so a crash
// while saving leaves the previous file. A file that cannot be read is treated as empty.
package dirsize

import (
	"compress/gzip"
	"encoding/gob"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultStoreMax is how many folders a Store keeps when Max is 0; the oldest counts are dropped first.
const DefaultStoreMax = 20000

// Remembered is one stored folder size.
type Remembered struct {
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	Allocated int64  `json:"allocated"`
	When      int64  `json:"when"` // when it was counted, Unix seconds
}

// Store is a set of remembered folder sizes backed by a file. Make it with OpenStore; it is safe for concurrent use.
type Store struct {
	File string // the file it is loaded from and saved to
	Max  int    // most folders kept (0 = DefaultStoreMax)

	mu    sync.Mutex
	m     map[string]Remembered
	dirty bool
}

// storeFile is the layout of the file; Version lets a later layout ignore an older file.
type storeFile struct {
	Version int
	Items   []Remembered
}

const storeVersion = 1

// OpenStore loads the store kept in file. A missing or unreadable file gives an empty store.
func OpenStore(file string) *Store {
	s := &Store{File: file, m: map[string]Remembered{}}
	f, err := os.Open(file)
	if err != nil {
		return s
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return s
	}
	var sf storeFile
	if gob.NewDecoder(zr).Decode(&sf) != nil || sf.Version != storeVersion {
		return s
	}
	for _, r := range sf.Items {
		if r.Path != "" && r.Bytes >= 0 {
			s.m[cacheKey(r.Path)] = r
		}
	}
	return s
}

// Get returns the remembered size of path.
func (s *Store) Get(path string) (Remembered, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.m[cacheKey(path)]
	return r, ok
}

// Len is the number of remembered folders.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

// Put remembers a size (When 0 means now). Negative sizes and empty paths are ignored.
func (s *Store) Put(r Remembered) {
	if r.Path == "" || r.Bytes < 0 {
		return
	}
	if r.When == 0 {
		r.When = time.Now().Unix()
	}
	r.Path = filepath.Clean(r.Path)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[cacheKey(r.Path)] = r
	s.dirty = true
	s.trim()
}

// PutResult remembers a complete calculation result; cancelled ones are ignored like in Cache.
func (s *Store) PutResult(r Result) {
	if r.Complete && r.IsDir {
		s.Put(Remembered{Path: r.Path, Bytes: r.Bytes, Allocated: r.Allocated})
	}
}

// trim drops the oldest entries above Max. Called with mu held.
func (s *Store) trim() {
	max := s.Max
	if max <= 0 {
		max = DefaultStoreMax
	}
	if len(s.m) <= max {
		return
	}
	all := make([]Remembered, 0, len(s.m))
	for _, r := range s.m {
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].When < all[j].When })
	for _, r := range all[:len(all)-max] {
		delete(s.m, cacheKey(r.Path))
	}
}

// Invalidate forgets path, every folder above it and everything below it (the same rule as Cache.Invalidate).
func (s *Store) Invalidate(path string) {
	k := cacheKey(path)
	sep := string(filepath.Separator)
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.m {
		if key == k || strings.HasPrefix(k, strings.TrimSuffix(key, sep)+sep) || strings.HasPrefix(key, strings.TrimSuffix(k, sep)+sep) {
			delete(s.m, key)
			s.dirty = true
		}
	}
}

// Clear forgets everything and removes the file.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = map[string]Remembered{}
	s.dirty = false
	if s.File == "" {
		return nil
	}
	if err := os.Remove(s.File); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Save writes the store to its file when something changed since it was loaded or last saved.
func (s *Store) Save() error {
	s.mu.Lock()
	if !s.dirty || s.File == "" {
		s.mu.Unlock()
		return nil
	}
	sf := storeFile{Version: storeVersion, Items: make([]Remembered, 0, len(s.m))}
	for _, r := range s.m {
		sf.Items = append(sf.Items, r)
	}
	s.dirty = false
	s.mu.Unlock()
	sort.Slice(sf.Items, func(i, j int) bool { return sf.Items[i].Path < sf.Items[j].Path })
	err := writeStore(s.File, sf)
	if err != nil {
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
	}
	return err
}

func writeStore(file string, sf storeFile) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".foldersizes-*")
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(tmp)
	err = gob.NewEncoder(zw).Encode(sf)
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), file)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
