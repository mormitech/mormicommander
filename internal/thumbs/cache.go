// This file is the on-disk thumbnail cache. Total Commander keeps generated
// thumbnails between sessions so that reopening a folder shows its previews
// instantly (FEATURES F-680), and offers "Delete thumbnails cache" to wipe it
// (F-678). An entry is keyed by the file's absolute path, modification time,
// size and the thumbnail box, so editing or replacing a picture, or changing
// the thumbnail size, naturally misses the old entry. Each entry is one small
// file named after a hash of the key, holding a short JSON header (which
// repeats the key so a hash collision can never return the wrong picture,
// plus the image metadata) followed by the encoded thumbnail. Failed decodes
// are remembered too, so a broken file is not re-read on every visit. The
// cache has a byte budget: when it grows past it, the least recently used
// entries are deleted (hits refresh an entry's timestamp). Writes go through
// a temporary file and a rename so a crash never leaves a torn entry.

package thumbs

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultCacheBytes is the default byte budget of the disk cache.
const DefaultCacheBytes = 256 << 20

// cacheMagic starts every entry file; bump the digit when the layout changes
// and old entries become unreadable (they are then treated as misses).
const cacheMagic = "MCTHUMB1\n"

// entryExt is the extension of entry files; anything else in the cache
// directory (temporary files, foreign files) is ignored by the size count.
const entryExt = ".mct"

// touchInterval throttles the "recently used" timestamp updates on hits, so
// scrolling through a folder does not rewrite metadata of hundreds of files.
const touchInterval = time.Hour

// DefaultCacheDir returns the per-user cache location,
// <UserCacheDir>/mormicommander/thumbs.
func DefaultCacheDir() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "mormicommander", "thumbs"), nil
}

// Key identifies one cached thumbnail.
type Key struct {
	Path    string    // absolute, cleaned source path
	ModTime time.Time // source modification time
	Size    int64     // source size in bytes
	Width   int       // thumbnail box width
	Height  int       // thumbnail box height
	Upscale bool      // whether small images were enlarged
}

// KeyFor stats path and builds its cache key for the given options.
func KeyFor(path string, opt Options) (Key, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Key{}, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return Key{}, err
	}
	return keyFromInfo(abs, fi, opt), nil
}

func keyFromInfo(abs string, fi fs.FileInfo, opt Options) Key {
	opt = opt.normalized()
	return Key{Path: filepath.Clean(abs), ModTime: fi.ModTime(), Size: fi.Size(),
		Width: opt.Width, Height: opt.Height, Upscale: opt.Upscale}
}

// hash returns the entry file base name for k.
func (k Key) hash() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%d\x00%d\x00%d\x00%t", k.Path, k.ModTime.UnixNano(), k.Size, k.Width, k.Height, k.Upscale)
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// entryHeader is the JSON block at the start of each entry file.
type entryHeader struct {
	Path    string `json:"p"`
	ModTime int64  `json:"m"`
	Size    int64  `json:"s"`
	BoxW    int    `json:"bw"`
	BoxH    int    `json:"bh"`
	Upscale bool   `json:"u,omitempty"`
	Meta    Meta   `json:"meta"`
	ThumbW  int    `json:"tw,omitempty"`
	ThumbH  int    `json:"th,omitempty"`
	MIME    string `json:"mime,omitempty"`
	Fail    string `json:"fail,omitempty"` // "unsupported", "decode", "toolarge"
	FailMsg string `json:"msg,omitempty"`
}

func (h *entryHeader) matches(k Key) bool {
	return h.Path == k.Path && h.ModTime == k.ModTime.UnixNano() && h.Size == k.Size &&
		h.BoxW == k.Width && h.BoxH == k.Height && h.Upscale == k.Upscale
}

// Cache is a disk cache of thumbnails. It is safe for concurrent use.
type Cache struct {
	dir      string
	maxBytes int64

	mu    sync.Mutex
	total int64 // approximate bytes of all entry files
	now   func() time.Time
}

// OpenCache opens (creating if needed) a cache in dir with the given byte
// budget; maxBytes <= 0 means DefaultCacheBytes.
func OpenCache(dir string, maxBytes int64) (*Cache, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultCacheBytes
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	c := &Cache{dir: dir, maxBytes: maxBytes, now: time.Now}
	st, err := c.Stats()
	if err != nil {
		return nil, err
	}
	c.total = st.Bytes
	return c, nil
}

// Dir returns the cache directory.
func (c *Cache) Dir() string { return c.dir }

// MaxBytes returns the byte budget.
func (c *Cache) MaxBytes() int64 { return c.maxBytes }

// SetMaxBytes changes the byte budget and evicts immediately if needed.
func (c *Cache) SetMaxBytes(n int64) error {
	if n <= 0 {
		n = DefaultCacheBytes
	}
	c.mu.Lock()
	c.maxBytes = n
	over := c.total > n
	c.mu.Unlock()
	if over {
		_, _, err := c.Evict()
		return err
	}
	return nil
}

func (c *Cache) entryPath(k Key) string {
	h := k.hash()
	return filepath.Join(c.dir, h[:2], h+entryExt)
}

// cachedFailure is returned by Get for remembered failures; it unwraps to
// the original sentinel error so errors.Is keeps working.
type cachedFailure struct {
	kind error
	msg  string
}

func (e *cachedFailure) Error() string { return e.msg + " (cached)" }
func (e *cachedFailure) Unwrap() error { return e.kind }

// IsCachedFailure reports whether err came from a remembered failure.
func IsCachedFailure(err error) bool {
	var cf *cachedFailure
	return errors.As(err, &cf)
}

// Get looks k up. It returns (thumb, true, nil) on a hit, (nil, true, err)
// when a failure was remembered for this exact file version, and
// (nil, false, nil) on a miss.
func (c *Cache) Get(k Key) (*Thumb, bool, error) {
	p := c.entryPath(k)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false, nil
	}
	h, payload, ok := parseEntry(data)
	if !ok || !h.matches(k) {
		return nil, false, nil
	}
	c.touch(p)
	if h.Fail != "" {
		return nil, true, &cachedFailure{kind: failKind(h.Fail), msg: h.FailMsg}
	}
	return &Thumb{Meta: h.Meta, Width: h.ThumbW, Height: h.ThumbH, MIME: h.MIME, Data: payload, FromCache: true}, true, nil
}

// touch refreshes the entry's mtime, which eviction uses as "last used".
func (c *Cache) touch(p string) {
	fi, err := os.Stat(p)
	if err != nil {
		return
	}
	now := c.now()
	if now.Sub(fi.ModTime()) < touchInterval {
		return
	}
	_ = os.Chtimes(p, now, now)
}

func parseEntry(data []byte) (*entryHeader, []byte, bool) {
	if !bytes.HasPrefix(data, []byte(cacheMagic)) {
		return nil, nil, false
	}
	rest := data[len(cacheMagic):]
	if len(rest) < 4 {
		return nil, nil, false
	}
	n := int(binary.BigEndian.Uint32(rest))
	if n < 0 || 4+n > len(rest) {
		return nil, nil, false
	}
	var h entryHeader
	if err := json.Unmarshal(rest[4:4+n], &h); err != nil {
		return nil, nil, false
	}
	return &h, rest[4+n:], true
}

// Put stores a thumbnail for k.
func (c *Cache) Put(k Key, t *Thumb) error {
	h := c.header(k)
	h.Meta, h.ThumbW, h.ThumbH, h.MIME = t.Meta, t.Width, t.Height, t.MIME
	return c.write(k, h, t.Data)
}

// PutFailure remembers that generating a thumbnail for k failed with err.
// Only the package's own sentinel errors are remembered; I/O errors and
// cancellations are transient and ignored.
func (c *Cache) PutFailure(k Key, err error) error {
	kind := failName(err)
	if kind == "" {
		return nil
	}
	h := c.header(k)
	h.Fail, h.FailMsg = kind, err.Error()
	return c.write(k, h, nil)
}

func (c *Cache) header(k Key) *entryHeader {
	return &entryHeader{Path: k.Path, ModTime: k.ModTime.UnixNano(), Size: k.Size,
		BoxW: k.Width, BoxH: k.Height, Upscale: k.Upscale}
}

func failName(err error) string {
	switch {
	case errors.Is(err, ErrUnsupported):
		return "unsupported"
	case errors.Is(err, ErrDecode):
		return "decode"
	case errors.Is(err, ErrTooLarge):
		return "toolarge"
	}
	return ""
}

func failKind(name string) error {
	switch name {
	case "decode":
		return ErrDecode
	case "toolarge":
		return ErrTooLarge
	}
	return ErrUnsupported
}

func (c *Cache) write(k Key, h *entryHeader, payload []byte) error {
	hj, err := json.Marshal(h)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString(cacheMagic)
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(hj)))
	buf.Write(n[:])
	buf.Write(hj)
	buf.Write(payload)

	p := c.entryPath(k)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	var old int64
	if fi, err := os.Stat(p); err == nil {
		old = fi.Size()
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	c.mu.Lock()
	c.total += int64(buf.Len()) - old
	over := c.total > c.maxBytes
	c.mu.Unlock()
	if over {
		_, _, err := c.Evict()
		return err
	}
	return nil
}

// Remove deletes the entry for k, if present.
func (c *Cache) Remove(k Key) error {
	p := c.entryPath(k)
	fi, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	c.mu.Lock()
	c.total -= fi.Size()
	c.mu.Unlock()
	return nil
}

// CacheStats summarises the cache contents.
type CacheStats struct {
	Entries int   `json:"entries"`
	Bytes   int64 `json:"bytes"`
}

type entryFile struct {
	path string
	size int64
	used time.Time
}

// walk lists all entry files. Stray temporary files older than an hour (from
// a crash mid-write) are deleted on the way.
func (c *Cache) walk() ([]entryFile, error) {
	var out []entryFile
	cutoff := c.now().Add(-time.Hour)
	err := filepath.WalkDir(c.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".tmp-") {
			if fi.ModTime().Before(cutoff) {
				os.Remove(p)
			}
			return nil
		}
		if filepath.Ext(p) != entryExt {
			return nil
		}
		out = append(out, entryFile{p, fi.Size(), fi.ModTime()})
		return nil
	})
	return out, err
}

// Stats counts entries and bytes by walking the directory.
func (c *Cache) Stats() (CacheStats, error) {
	files, err := c.walk()
	var st CacheStats
	for _, f := range files {
		st.Entries++
		st.Bytes += f.size
	}
	return st, err
}

// Evict deletes least recently used entries until the cache is at 90 % of
// its budget (the margin avoids evicting on every single write). It returns
// the number of entries removed and the bytes freed.
func (c *Cache) Evict() (int, int64, error) {
	files, err := c.walk()
	if err != nil {
		return 0, 0, err
	}
	var total int64
	for _, f := range files {
		total += f.size
	}
	c.mu.Lock()
	target := c.maxBytes / 10 * 9
	c.mu.Unlock()
	sort.Slice(files, func(i, j int) bool { return files[i].used.Before(files[j].used) })
	var n int
	var freed int64
	for _, f := range files {
		if total <= target {
			break
		}
		if err := os.Remove(f.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		total -= f.size
		freed += f.size
		n++
	}
	c.mu.Lock()
	c.total = total
	c.mu.Unlock()
	return n, freed, nil
}

// EvictOlderThan deletes entries not used within age.
func (c *Cache) EvictOlderThan(age time.Duration) (int, int64, error) {
	files, err := c.walk()
	if err != nil {
		return 0, 0, err
	}
	cutoff := c.now().Add(-age)
	var n int
	var freed int64
	for _, f := range files {
		if f.used.Before(cutoff) {
			if os.Remove(f.path) == nil {
				n++
				freed += f.size
			}
		}
	}
	c.mu.Lock()
	c.total -= freed
	c.mu.Unlock()
	return n, freed, nil
}

// PruneStale deletes entries whose source file is gone or has changed since
// the thumbnail was made. It reads only the small header of each entry.
func (c *Cache) PruneStale() (int, int64, error) {
	files, err := c.walk()
	if err != nil {
		return 0, 0, err
	}
	var n int
	var freed int64
	for _, f := range files {
		h, err := readHeader(f.path)
		stale := err != nil
		if !stale {
			fi, err := os.Stat(h.Path)
			stale = err != nil || fi.ModTime().UnixNano() != h.ModTime || fi.Size() != h.Size
		}
		if stale && os.Remove(f.path) == nil {
			n++
			freed += f.size
		}
	}
	c.mu.Lock()
	c.total -= freed
	c.mu.Unlock()
	return n, freed, nil
}

func readHeader(p string) (*entryHeader, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	pre := make([]byte, len(cacheMagic)+4)
	if _, err := io.ReadFull(f, pre); err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(pre, []byte(cacheMagic)) {
		return nil, errors.New("bad magic")
	}
	n := binary.BigEndian.Uint32(pre[len(cacheMagic):])
	if n > 1<<20 {
		return nil, errors.New("bad header length " + strconv.Itoa(int(n)))
	}
	hj := make([]byte, n)
	if _, err := io.ReadFull(f, hj); err != nil {
		return nil, err
	}
	var h entryHeader
	if err := json.Unmarshal(hj, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

// Clear deletes every entry ("Delete thumbnails cache"). The directory
// itself is kept.
func (c *Cache) Clear() error {
	ents, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}
	var firstErr error
	for _, e := range ents {
		if err := os.RemoveAll(filepath.Join(c.dir, e.Name())); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	st, err := c.Stats()
	c.mu.Lock()
	c.total = st.Bytes
	c.mu.Unlock()
	if firstErr != nil {
		return firstErr
	}
	return err
}
