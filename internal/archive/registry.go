// This file keeps the list of archive formats the program understands and
// picks the right one for a file. Total Commander decides by extension when
// the user presses Enter, and by content when the user presses Ctrl+PgDn on a
// file whose extension is not an archive one (a .jar, .apk, .docx or an
// installer with a zip appended). Detect therefore checks the magic bytes of
// every registered format first, then a deeper Probe (for example looking for
// the zip end record at the end of a self-extracting .exe), and only then the
// extension. Formats live in sub-packages and call Register from init.
package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Caps tells the UI which Pack dialog options a format supports.
type Caps struct {
	Write       bool // can create and update archives
	Encrypt     bool // supports PackOptions.Password
	Volumes     bool // supports PackOptions.VolumeSize
	Comments    bool // supports archive and entry comments
	Levels      bool // honors PackOptions.Level
	Methods     []Method
	Encryptions []Encryption
}

// Format describes one registered archive format.
type Format struct {
	// Name is the short identifier, for example "zip".
	Name string
	// Description is shown in the packer list, for example "ZIP archive".
	Description string
	// Extensions lists lower-case extensions without the leading dot. Multi
	// part extensions such as "tar.gz" are allowed and win over shorter ones.
	Extensions []string
	// Magic reports whether the first bytes of a file (up to 512) look like
	// this format.
	Magic func(header []byte) bool
	// Probe, optional, performs a deeper check on the whole file when Magic
	// failed, for formats whose signature is not at offset 0.
	Probe func(r io.ReaderAt, size int64) bool
	// Open opens an archive file (for multi-volume sets, any volume).
	Open func(path string, opts OpenOptions) (Archive, error)
	// OpenReaderAt, optional, opens an archive held in memory or inside
	// another archive; name is used for messages and Archive.Path.
	OpenReaderAt func(r io.ReaderAt, size int64, name string, opts OpenOptions) (Archive, error)
	// Packer is nil for read-only formats.
	Packer Packer
	Caps   Caps
}

var (
	regMu   sync.RWMutex
	formats []*Format
)

// Register adds a format. Registering the same name twice replaces the old
// entry, which keeps tests that register fakes simple.
func Register(f Format) {
	regMu.Lock()
	defer regMu.Unlock()
	for i, old := range formats {
		if old.Name == f.Name {
			formats[i] = &f
			return
		}
	}
	formats = append(formats, &f)
}

// Formats returns all registered formats sorted by name.
func Formats() []Format {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Format, len(formats))
	for i, f := range formats {
		out[i] = *f
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// Lookup returns the format with the given name.
func Lookup(name string) (Format, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	for _, f := range formats {
		if strings.EqualFold(f.Name, name) {
			return *f, true
		}
	}
	return Format{}, false
}

// ByExtension returns the format whose extension matches the file name best
// (longest match), without looking at the content.
func ByExtension(name string) (Format, bool) {
	lower := strings.ToLower(filepath.Base(name))
	regMu.RLock()
	defer regMu.RUnlock()
	var best *Format
	bestLen := 0
	for _, f := range formats {
		for _, ext := range f.Extensions {
			if strings.HasSuffix(lower, "."+ext) && len(ext) > bestLen {
				best, bestLen = f, len(ext)
			}
		}
	}
	if best == nil {
		return Format{}, false
	}
	return *best, true
}

// DetectReaderAt identifies the format of content by magic bytes, probes and
// finally the name's extension.
func DetectReaderAt(r io.ReaderAt, size int64, name string) (Format, bool) {
	head := make([]byte, 512)
	n, _ := r.ReadAt(head, 0)
	head = head[:n]
	regMu.RLock()
	list := append([]*Format(nil), formats...)
	regMu.RUnlock()
	for _, f := range list {
		if f.Magic != nil && f.Magic(head) {
			return *f, true
		}
	}
	for _, f := range list {
		if f.Probe != nil && f.Probe(r, size) {
			return *f, true
		}
	}
	return ByExtension(name)
}

// Detect identifies the format of a file on disk. Files that cannot be read
// (for example the .z01 part of a split zip) are matched by extension.
func Detect(path string) (Format, bool) {
	f, err := os.Open(path)
	if err != nil {
		return ByExtension(path)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return Format{}, false
	}
	return DetectReaderAt(f, st.Size(), path)
}

// Open detects the format of path and opens it.
func Open(path string, opts OpenOptions) (Archive, error) {
	f, ok := Detect(path)
	if !ok || f.Open == nil {
		return nil, fmt.Errorf("%w: %s", ErrFormat, filepath.Base(path))
	}
	return f.Open(path, opts)
}

// IsArchiveName reports whether Enter on this file name should open it as an
// archive (extension check only, as Total Commander does for Enter).
func IsArchiveName(name string) bool {
	_, ok := ByExtension(name)
	return ok
}
