// This file finds and joins the parts of a multi-volume zip. Total Commander
// (like Info-ZIP's "zip -s" and WinZip) writes split archives as name.z01,
// name.z02, ... with the last part called name.zip, and records in the
// central directory on which part each file starts. Some tools instead cut a
// finished zip into name.zip.001, name.zip.002, ... which are simply
// concatenated. Both kinds are presented to the reader as one continuous
// io.ReaderAt; the reader then decides from the end record whether offsets are
// per-part (true split archive) or global (plain concatenation). The user may
// open any part, and a missing part is reported by name.
package ziparc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"mormicommander/internal/archive"
)

// volumeSet is a read-only concatenation of volume files.
type volumeSet struct {
	names  []string
	files  []io.ReaderAt
	closer []io.Closer
	starts []int64 // offset of each volume in the logical stream
	sizes  []int64
	total  int64
}

func (v *volumeSet) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("ziparc: negative offset")
	}
	n := 0
	for len(p) > 0 {
		if off >= v.total {
			return n, io.EOF
		}
		i := v.find(off)
		local := off - v.starts[i]
		want := len(p)
		if rem := v.sizes[i] - local; int64(want) > rem {
			want = int(rem)
		}
		m, err := v.files[i].ReadAt(p[:want], local)
		n += m
		off += int64(m)
		p = p[m:]
		if err != nil && err != io.EOF {
			return n, err
		}
		if m < want {
			return n, io.ErrUnexpectedEOF
		}
	}
	return n, nil
}

func (v *volumeSet) find(off int64) int {
	lo, hi := 0, len(v.starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if v.starts[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

func (v *volumeSet) Close() error {
	var first error
	for _, c := range v.closer {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func singleVolume(r io.ReaderAt, size int64, name string) *volumeSet {
	return &volumeSet{names: []string{name}, files: []io.ReaderAt{r}, starts: []int64{0}, sizes: []int64{size}, total: size}
}

func openVolumes(names []string) (*volumeSet, error) {
	v := &volumeSet{names: names}
	for _, n := range names {
		f, err := os.Open(n)
		if err != nil {
			v.Close()
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%w: %s", archive.ErrMissingVolume, filepath.Base(n))
			}
			return nil, err
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			v.Close()
			return nil, err
		}
		v.files = append(v.files, f)
		v.closer = append(v.closer, f)
		v.starts = append(v.starts, v.total)
		v.sizes = append(v.sizes, st.Size())
		v.total += st.Size()
	}
	return v, nil
}

var (
	reSplitPart = regexp.MustCompile(`(?i)^(.*)\.z(\d{2,3})$`)
	reNumPart   = regexp.MustCompile(`^(.*)\.(\d{3})$`)
)

// existingCase returns the first candidate that exists.
func existingCase(cands ...string) (string, bool) {
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c, true
		}
	}
	return "", false
}

// volumeNameFor returns the name of the split part with number n (1-based)
// for a set whose last part is mainPath, keeping the case of the extension.
func volumeNameFor(mainPath string, n int) string {
	ext := filepath.Ext(mainPath)
	base := strings.TrimSuffix(mainPath, ext)
	z := "z"
	if ext == strings.ToUpper(ext) && ext != "" {
		z = "Z"
	}
	return fmt.Sprintf("%s.%s%02d", base, z, n)
}

// discoverVolumes returns the files of the set that p belongs to, in order,
// and whether they are numbered parts (.001) rather than split parts (.z01).
func discoverVolumes(p string) (names []string, numbered bool, err error) {
	// name.zNN: the main file is name.zip.
	if m := reSplitPart.FindStringSubmatch(p); m != nil {
		main, ok := existingCase(m[1]+".zip", m[1]+".ZIP", m[1]+".Zip")
		if !ok {
			return nil, false, fmt.Errorf("%w: %s.zip (the last part of the set)", archive.ErrMissingVolume, filepath.Base(m[1]))
		}
		p = main
	}
	// name.001, name.002, ...: concatenate all consecutive parts.
	if m := reNumPart.FindStringSubmatch(p); m != nil {
		start, _ := strconv.Atoi(m[2])
		if start != 0 {
			first := m[1] + ".001"
			if _, err := os.Stat(first); err != nil {
				return nil, false, fmt.Errorf("%w: %s", archive.ErrMissingVolume, filepath.Base(first))
			}
			for i := 1; ; i++ {
				n := fmt.Sprintf("%s.%03d", m[1], i)
				if _, err := os.Stat(n); err != nil {
					break
				}
				names = append(names, n)
			}
			return names, true, nil
		}
	}
	// name.zip with name.z01 ... next to it.
	base := strings.TrimSuffix(p, filepath.Ext(p))
	for i := 1; ; i++ {
		cand, ok := existingCase(fmt.Sprintf("%s.z%02d", base, i), fmt.Sprintf("%s.Z%02d", base, i))
		if !ok {
			break
		}
		names = append(names, cand)
	}
	names = append(names, p)
	return names, false, nil
}
