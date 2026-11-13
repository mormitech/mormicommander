// Tests for the disk cache: keying by path, mtime, size and box, remembered
// failures, corruption handling, LRU eviction, pruning and clearing.

package thumbs

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleThumb(n int) *Thumb {
	return &Thumb{Meta: Meta{Format: "png", Width: 10, Height: 5, Orientation: 1},
		Width: 4, Height: 2, MIME: "image/png", Data: bytes.Repeat([]byte{byte(n)}, 1000)}
}

func sampleKey(path string, n int) Key {
	return Key{Path: path, ModTime: time.Unix(1700000000, int64(n)), Size: int64(100 + n), Width: 96, Height: 96}
}

func TestCachePutGet(t *testing.T) {
	c, err := OpenCache(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	k := sampleKey("/x/a.png", 1)
	if _, ok, _ := c.Get(k); ok {
		t.Fatal("hit on empty cache")
	}
	if err := c.Put(k, sampleThumb(1)); err != nil {
		t.Fatal(err)
	}
	th, ok, err := c.Get(k)
	if !ok || err != nil {
		t.Fatalf("Get = %v, %v", ok, err)
	}
	want := sampleThumb(1)
	if !th.FromCache || th.Meta != want.Meta || th.Width != 4 || th.Height != 2 || th.MIME != want.MIME || !bytes.Equal(th.Data, want.Data) {
		t.Errorf("roundtrip mismatch: %+v", th)
	}
	variants := map[string]Key{}
	v := k
	v.ModTime = v.ModTime.Add(time.Second)
	variants["mtime"] = v
	v = k
	v.Size++
	variants["size"] = v
	v = k
	v.Width = 128
	variants["width"] = v
	v = k
	v.Height = 64
	variants["height"] = v
	v = k
	v.Upscale = true
	variants["upscale"] = v
	v = k
	v.Path = "/x/b.png"
	variants["path"] = v
	for name, v := range variants {
		if _, ok, _ := c.Get(v); ok {
			t.Errorf("%s change still hits", name)
		}
	}
}

func TestCacheFailures(t *testing.T) {
	c, err := OpenCache(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range []error{ErrDecode, ErrTooLarge, ErrUnsupported} {
		k := sampleKey("/f", i)
		if err := c.PutFailure(k, e); err != nil {
			t.Fatal(err)
		}
		th, ok, err := c.Get(k)
		if !ok || th != nil || !errors.Is(err, e) || !IsCachedFailure(err) {
			t.Errorf("%v: got %v %v %v", e, th, ok, err)
		}
	}
	// Transient errors are not remembered.
	k := sampleKey("/f", 9)
	if err := c.PutFailure(k, os.ErrPermission); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Get(k); ok {
		t.Error("transient error was cached")
	}
	if IsCachedFailure(ErrDecode) {
		t.Error("plain error reported as cached")
	}
}

func TestCacheCorruptEntries(t *testing.T) {
	c, err := OpenCache(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	k := sampleKey("/c", 1)
	if err := c.Put(k, sampleThumb(1)); err != nil {
		t.Fatal(err)
	}
	p := c.entryPath(k)
	good, _ := os.ReadFile(p)
	for name, data := range map[string][]byte{
		"empty":      nil,
		"bad magic":  append([]byte("XXTHUMB1\n"), good[9:]...),
		"short":      good[:11],
		"bad length": append(append([]byte(cacheMagic), 0xff, 0xff, 0xff, 0x00), good[13:]...),
		"bad json":   append(append([]byte(cacheMagic), 0, 0, 0, 3), "{{{"...),
	} {
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok, _ := c.Get(k); ok {
			t.Errorf("%s: corrupt entry hit", name)
		}
	}
	// A different key whose hash file contains another key's header (a
	// simulated collision) must miss.
	other := sampleKey("/other", 2)
	if err := os.MkdirAll(filepath.Dir(c.entryPath(other)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.entryPath(other), good, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Get(other); ok {
		t.Error("colliding header hit")
	}
}

func TestCacheEvictionLRU(t *testing.T) {
	dir := t.TempDir()
	c, err := OpenCache(dir, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-24 * time.Hour)
	keys := make([]Key, 8)
	for i := range keys {
		keys[i] = sampleKey("/e", i)
		if err := c.Put(keys[i], sampleThumb(i)); err != nil {
			t.Fatal(err)
		}
		// Make the entry's "last used" time increase with i.
		at := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(c.entryPath(keys[i]), at, at); err != nil {
			t.Fatal(err)
		}
	}
	// Using key 0 (the oldest) must refresh it so it survives.
	if _, ok, _ := c.Get(keys[0]); !ok {
		t.Fatal("key 0 missing before eviction")
	}
	for i := 8; i < 14; i++ {
		if err := c.Put(sampleKey("/e", i), sampleThumb(i)); err != nil {
			t.Fatal(err)
		}
	}
	st, err := c.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.Bytes > 10_000 {
		t.Errorf("cache over budget: %d", st.Bytes)
	}
	if _, ok, _ := c.Get(keys[0]); !ok {
		t.Error("recently used entry evicted")
	}
	if _, ok, _ := c.Get(keys[1]); ok {
		t.Error("least recently used entry survived")
	}
	if _, ok, _ := c.Get(sampleKey("/e", 13)); !ok {
		t.Error("newest entry evicted")
	}
}

func TestCacheSetMaxBytesEvicts(t *testing.T) {
	c, err := OpenCache(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := c.Put(sampleKey("/m", i), sampleThumb(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.SetMaxBytes(3000); err != nil {
		t.Fatal(err)
	}
	st, _ := c.Stats()
	if st.Bytes > 3000 || st.Entries == 0 {
		t.Errorf("after shrink: %+v", st)
	}
	if c.MaxBytes() != 3000 {
		t.Errorf("MaxBytes %d", c.MaxBytes())
	}
}

func TestCachePersistenceAndClear(t *testing.T) {
	dir := t.TempDir()
	c, err := OpenCache(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := c.Put(sampleKey("/p", i), sampleThumb(i)); err != nil {
			t.Fatal(err)
		}
	}
	c2, err := OpenCache(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c2.Get(sampleKey("/p", 2)); !ok {
		t.Error("entry lost across reopen")
	}
	st, _ := c2.Stats()
	if st.Entries != 3 || c2.total != st.Bytes {
		t.Errorf("stats %+v total %d", st, c2.total)
	}
	if err := c2.Remove(sampleKey("/p", 2)); err != nil {
		t.Fatal(err)
	}
	if err := c2.Remove(sampleKey("/p", 2)); err != nil {
		t.Errorf("second Remove: %v", err)
	}
	if err := c2.Clear(); err != nil {
		t.Fatal(err)
	}
	st, _ = c2.Stats()
	if st.Entries != 0 || st.Bytes != 0 || c2.total != 0 {
		t.Errorf("after Clear: %+v", st)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Error("Clear removed the cache dir itself")
	}
}

// Regression: Clear used to RemoveAll every entry of the cache folder, so a
// cache pointed at an existing folder wiped the user's files.
func TestCacheClearKeepsForeignFiles(t *testing.T) {
	dir := t.TempDir()
	foreign := map[string]string{
		"important.doc":     "keep me",
		"ab/notes.txt":      "inside a shard-looking folder",
		"photos/a.mct":      "not a shard folder",
		"cd/" + "short.mct": "not a hash name",
	}
	for name, body := range foreign {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := OpenCache(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := c.Put(sampleKey("/p", i), sampleThumb(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	for name, body := range foreign {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil || string(got) != body {
			t.Errorf("%s: err=%v got %q", name, err, got)
		}
	}
	if st, _ := c.Stats(); st.Entries != 0 {
		t.Errorf("entries left after Clear: %+v", st)
	}
}

func TestCachePruneStale(t *testing.T) {
	dir := t.TempDir()
	src := t.TempDir()
	c, err := OpenCache(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	keep := writeFile(t, src, "keep.png", []byte("1"))
	gone := writeFile(t, src, "gone.png", []byte("2"))
	changed := writeFile(t, src, "changed.png", []byte("3"))
	for _, p := range []string{keep, gone, changed} {
		k, err := KeyFor(p, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Put(k, sampleThumb(1)); err != nil {
			t.Fatal(err)
		}
	}
	os.Remove(gone)
	if err := os.WriteFile(changed, []byte("changed!"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, freed, err := c.PruneStale()
	if err != nil || n != 2 || freed <= 0 {
		t.Errorf("PruneStale = %d, %d, %v", n, freed, err)
	}
	k, _ := KeyFor(keep, Options{})
	if _, ok, _ := c.Get(k); !ok {
		t.Error("valid entry pruned")
	}
}

func TestCacheEvictOlderThanAndTmpCleanup(t *testing.T) {
	dir := t.TempDir()
	c, err := OpenCache(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	old, fresh := sampleKey("/o", 1), sampleKey("/o", 2)
	c.Put(old, sampleThumb(1))
	c.Put(fresh, sampleThumb(2))
	past := time.Now().Add(-48 * time.Hour)
	os.Chtimes(c.entryPath(old), past, past)
	tmp := writeFile(t, filepath.Dir(c.entryPath(old)), ".tmp-123", []byte("partial"))
	os.Chtimes(tmp, past, past)
	n, _, err := c.EvictOlderThan(24 * time.Hour)
	if err != nil || n != 1 {
		t.Errorf("EvictOlderThan = %d, %v", n, err)
	}
	if _, ok, _ := c.Get(old); ok {
		t.Error("old entry survived")
	}
	if _, ok, _ := c.Get(fresh); !ok {
		t.Error("fresh entry evicted")
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("stale temp file not cleaned")
	}
}

func TestKeyForAndDefaultDir(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "k.png", []byte("abc"))
	k, err := KeyFor(p, Options{Width: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if k.Size != 3 || k.Width != MaxSize || k.Height != DefaultSize || !filepath.IsAbs(k.Path) {
		t.Errorf("key %+v", k)
	}
	if _, err := KeyFor(filepath.Join(dir, "missing"), Options{}); err == nil {
		t.Error("no error for missing file")
	}
	if d, err := DefaultCacheDir(); err == nil && filepath.Base(d) != "thumbs" {
		t.Errorf("DefaultCacheDir %q", d)
	}
}
