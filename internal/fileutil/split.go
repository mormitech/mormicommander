// Splitting a file is TC's cm_FileSplit: a big file is cut into numbered
// parts of a chosen size (name.001, name.002, ...) so it fits on smaller
// media or through size-limited transfers, and a name.crc info file records
// the original name, size and CRC32 so Combine can check the result. The CRC
// is computed while the parts are written, so the file is read only once.
// Like TC, the parts replace the file's extension ("movie.mkv" becomes
// "movie.001"); the original name lives in the .crc file. For writing
// straight to removable disks the caller can ask for "Automatic" part sizes
// and be called back before each part to let the user swap the disk. If the
// split fails or is cancelled, every part already written is removed.

package fileutil

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SplitOptions configures SplitFile.
type SplitOptions struct {
	// Source is the file to split.
	Source string
	// TargetDir is where the parts go (default: the source's folder).
	TargetDir string
	// PartSize is the size of each part in bytes, or SizeAuto.
	PartSize int64
	// FreeSpace reports the free bytes in a folder. It is required for
	// SizeAuto and called before each part, after BeforePart.
	FreeSpace func(dir string) (int64, error)
	// BeforePart, when set, is called before part number index (1-based) is
	// created at path, e.g. to ask the user to insert the next disk. An error
	// aborts the split.
	BeforePart func(index int, path string) error
	// NoCRCFile skips writing the .crc info file.
	NoCRCFile bool
	// Format picks the line ending of the .crc file.
	Format LineFormat
	// Overwrite allows replacing existing parts and .crc file.
	Overwrite bool
	Progress  ProgressFunc
}

// SplitResult reports what SplitFile wrote.
type SplitResult struct {
	Parts   []string
	CRCFile string // empty when NoCRCFile was set
	Info    CRCInfo
}

// autoReserve is kept free on the target in Automatic mode for file system
// overhead and the .crc file.
const autoReserve = 64 << 10

// PartName returns the path of part index (1-based) for splitting source into
// dir: the source's base name with its extension replaced by a three-digit
// number (four or more digits beyond 999 parts).
func PartName(dir, source string, index int) string {
	return filepath.Join(dir, fmt.Sprintf("%s.%03d", stem(source), index))
}

// CRCFileName returns the path of the .crc info file for splitting source
// into dir.
func CRCFileName(dir, source string) string {
	return filepath.Join(dir, stem(source)+".crc")
}

func stem(path string) string {
	b := filepath.Base(path)
	if e := filepath.Ext(b); e != "" && e != b {
		return strings.TrimSuffix(b, e)
	}
	return b
}

// PartCount returns how many parts a file of the given size makes. An empty
// file still makes one (empty) part so Combine has something to work on.
func PartCount(size, partSize int64) int {
	if partSize <= 0 {
		return 0
	}
	if size == 0 {
		return 1
	}
	// Divide first: size + partSize - 1 overflows for a huge part size, and a negative count made SplitFile skip its
	// checks (among them the one that keeps part 1 from overwriting a source named like a part).
	n := size / partSize
	if size%partSize != 0 {
		n++
	}
	return int(n)
}

// SplitFile splits opts.Source into parts.
func SplitFile(ctx context.Context, opts SplitOptions) (*SplitResult, error) {
	if opts.PartSize == 0 || opts.PartSize < SizeAuto {
		return nil, errors.New("the part size must be larger than zero")
	}
	if opts.PartSize == SizeAuto && opts.FreeSpace == nil {
		return nil, errors.New("automatic part size needs a free space query")
	}
	size, err := fileSize(opts.Source)
	if err != nil {
		return nil, err
	}
	dir := opts.TargetDir
	if dir == "" {
		dir = filepath.Dir(opts.Source)
	}
	if fi, err := os.Stat(dir); err != nil {
		return nil, err
	} else if !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a folder", dir)
	}

	// With a fixed size we know every name in advance, so refuse up front
	// instead of failing halfway when a part already exists, and make sure
	// no part would overwrite the source itself.
	srcSet := newFileSet(opts.Source)
	crcPath := CRCFileName(dir, opts.Source)
	if opts.PartSize > 0 {
		n := PartCount(size, opts.PartSize)
		for i := 1; i <= n; i++ {
			p := PartName(dir, opts.Source, i)
			if srcSet.contains(p) {
				return nil, fmt.Errorf("part %s would overwrite the file being split", p)
			}
			if !opts.Overwrite {
				if _, err := os.Lstat(p); err == nil {
					return nil, &os.PathError{Op: "create", Path: p, Err: ErrExists}
				}
			}
		}
	}
	if !opts.NoCRCFile && srcSet.contains(crcPath) {
		return nil, fmt.Errorf("the .crc file would overwrite the file being split")
	}
	if !opts.NoCRCFile && !opts.Overwrite {
		if _, err := os.Lstat(crcPath); err == nil {
			return nil, &os.PathError{Op: "create", Path: crcPath, Err: ErrExists}
		}
	}

	in, err := os.Open(opts.Source)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	t := newTracker(ctx, opts.Progress, size)
	crc := crc32.NewIEEE()
	src := io.TeeReader(in, crc)
	res := &SplitResult{}
	fail := func(err error) (*SplitResult, error) {
		removeAll(res.Parts)
		if res.CRCFile != "" {
			os.Remove(res.CRCFile)
		}
		return nil, err
	}

	var done int64
	for index := 1; done < size || index == 1; index++ {
		if err := t.check(); err != nil {
			return fail(err)
		}
		p := PartName(dir, opts.Source, index)
		if opts.BeforePart != nil {
			if err := opts.BeforePart(index, p); err != nil {
				return fail(err)
			}
		}
		want := opts.PartSize
		if want == SizeAuto {
			free, err := opts.FreeSpace(dir)
			if err != nil {
				return fail(err)
			}
			want = free - autoReserve
			if want <= 0 {
				return fail(fmt.Errorf("there is not enough free space in %s for the next part", dir))
			}
			if srcSet.contains(p) {
				return fail(fmt.Errorf("part %s would overwrite the file being split", p))
			}
		}
		if rest := size - done; want > rest {
			want = rest
		}
		out, err := createOutput(p, opts.Overwrite)
		if err != nil {
			return fail(err)
		}
		res.Parts = append(res.Parts, p)
		n, err := t.copyN(out, src, want, opts.Source, done, size)
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
		if err == nil && n != want {
			err = fmt.Errorf("%s got shorter while it was being split", opts.Source)
		}
		if err != nil {
			return fail(err)
		}
		done += n
	}
	res.Info = CRCInfo{FileName: filepath.Base(opts.Source), Size: size, CRC32: crc.Sum32()}
	if !opts.NoCRCFile {
		if err := writeTextFile(crcPath, res.Info.Format(opts.Format.unix()), opts.Overwrite); err != nil {
			return fail(err)
		}
		res.CRCFile = crcPath
	}
	return res, nil
}
