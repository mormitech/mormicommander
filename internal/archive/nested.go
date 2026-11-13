// This file lets the user press Enter on an archive that sits inside another
// archive (a zip inside a zip, a .jar inside a .war) and keep browsing, as
// Total Commander does. When the inner archive is stored uncompressed and
// the outer format can hand out a direct view of its bytes (the Sectioner
// interface), the inner archive is opened in place without copying anything.
// Otherwise it is unpacked to a temporary file first; that file is removed
// again when the nested archive is closed.
package archive

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Sectioner is implemented by archives that can expose the raw bytes of an
// entry without decompression (stored, unencrypted entries). It returns
// ok == false when that is not possible for e.
type Sectioner interface {
	Section(e Entry) (r io.ReaderAt, size int64, ok bool)
}

// nested wraps an inner archive so Close also releases the temporary copy.
type nested struct {
	Archive
	display string
	temp    string
}

func (n *nested) Path() string { return n.display }

func (n *nested) Close() error {
	err := n.Archive.Close()
	if n.temp != "" {
		os.Remove(n.temp)
	}
	return err
}

// OpenNested opens the archive stored in entry e of outer. tempDir is where
// a temporary copy goes when one is needed ("" means os.TempDir()). The
// returned archive's Path is "<outer path>/<entry path>" for display; the UI
// keeps track of the chain for the path bar.
func OpenNested(ctx context.Context, outer Archive, e Entry, opts OpenOptions, tempDir string) (Archive, error) {
	if e.IsDir {
		return nil, fmt.Errorf("archive: %s is a folder", e.Path)
	}
	display := outer.Path() + "/" + e.Path
	if s, ok := outer.(Sectioner); ok {
		if r, size, ok := s.Section(e); ok {
			if f, ok := DetectReaderAt(r, size, e.Name); ok && f.OpenReaderAt != nil {
				inner, err := f.OpenReaderAt(r, size, display, opts)
				if err == nil {
					return &nested{Archive: inner, display: display}, nil
				}
			}
		}
	}
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	tmp, err := os.CreateTemp(tempDir, "mc-nested-*-"+SanitizeName(e.Name))
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	rc, err := outer.Open(e)
	if err == nil {
		_, err = CopyContext(ctx, tmp, rc, nil)
		rc.Close()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(name)
		return nil, err
	}
	f, ok := Detect(name)
	if !ok || f.Open == nil {
		os.Remove(name)
		return nil, fmt.Errorf("%w: %s", ErrFormat, filepath.Base(e.Path))
	}
	inner, err := f.Open(name, opts)
	if err != nil {
		os.Remove(name)
		return nil, err
	}
	return &nested{Archive: inner, display: display, temp: name}, nil
}
