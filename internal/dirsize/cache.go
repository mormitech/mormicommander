// Once Total Commander has counted a folder, the size column shows that number instead of <DIR> until the panel is
// re-read, and both panels showing the same folder agree. Cache keeps those results so the UI can do the same. Any
// change inside a folder changes the size of that folder and of every folder above it, and a counted parent says
// nothing reliable about a renamed or deleted child, so Invalidate drops the path, all its ancestors and all its
// descendants together. Paths are compared after cleaning, and without regard to case on Windows and macOS, whose
// usual file systems ignore case.
package dirsize

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"
)

// Cache remembers calculated folder sizes by path. The zero value is ready to use and it is safe for concurrent use.
type Cache struct {
	mu sync.Mutex
	m  map[string]Result
}

func cacheKey(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "darwin" {
		p = norm.NFC.String(p) // APFS treats the NFC and NFD forms of a name as one name
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return p
}

// Put stores a result under its Path. Incomplete (cancelled) results are not stored, since showing a partial size as
// the folder size would be wrong.
func (c *Cache) Put(r Result) {
	if !r.Complete {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]Result{}
	}
	c.m[cacheKey(r.Path)] = r
}

// Get returns the stored result for path.
func (c *Cache) Get(path string) (Result, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.m[cacheKey(path)]
	return r, ok
}

// Len is the number of stored results.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}

// Invalidate forgets path, every folder above it and everything below it. Call it for each path a file operation
// touched (source and target of a copy or move, deleted items, a renamed item under both names).
func (c *Cache) Invalidate(path string) {
	k := cacheKey(path)
	sep := string(filepath.Separator)
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.m {
		if key == k || strings.HasPrefix(k, strings.TrimSuffix(key, sep)+sep) || strings.HasPrefix(key, strings.TrimSuffix(k, sep)+sep) {
			delete(c.m, key)
		}
	}
}

// Clear forgets everything, for a full refresh (Ctrl+R on both panels).
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = nil
}
