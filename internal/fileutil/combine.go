// Combining is TC's cm_FileCombine, the reverse of Split. Run on the first
// part (name.001) it finds name.002, name.003 and so on by itself, joins them
// in order and, when the name.crc info file is there, restores the original
// file name and checks both the size and the CRC32 of the result. Without a
// .crc file it simply joins all consecutive parts it finds. It also works on
// an explicit list of files, because TC joins any selected files in the order
// they were selected. Parts made by other tools with the same numbering, such
// as 7-Zip's "archive.zip.001", work too. When a part is missing in the
// middle, a callback lets the UI ask for the disk holding it; a size or CRC
// mismatch is reported as an error but the joined file is kept, so the user
// can still rescue what is in it.

package fileutil

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrCombineMismatch is returned (wrapped) when the joined file does not
// match the size or CRC32 recorded in the .crc file.
var ErrCombineMismatch = errors.New("the combined file does not match the split info")

// CombineOptions configures CombineFiles.
type CombineOptions struct {
	// FirstPart is the first numbered part (or any part, or the .crc file);
	// the remaining parts are found by counting up. Ignored when Parts is set.
	FirstPart string
	// Parts is an explicit, ordered list of files to join.
	Parts []string
	// TargetDir is where the result goes (default: the first part's folder).
	TargetDir string
	// TargetName overrides the result's file name. By default it comes from
	// the .crc file, or is the first part's name without the number.
	TargetName string
	// CRCFile is the split info file. By default name.crc next to the first
	// part is used when it exists. Set NoCRCCheck to ignore it.
	CRCFile    string
	NoCRCCheck bool
	// NextPart is called when an expected part is missing (index is
	// 1-based). It can return another path for it, e.g. after the user
	// inserted the next disk. Returning an error aborts. Without NextPart a
	// missing part is an error when the .crc file says more data is due,
	// and the end of the parts otherwise.
	NextPart  func(index int, expected string) (string, error)
	Overwrite bool
	Progress  ProgressFunc
}

// CombineResult reports what CombineFiles did.
type CombineResult struct {
	Target  string
	Parts   []string
	Size    int64
	CRC32   uint32
	Info    *CRCInfo // split info used for checking, nil if none
	Checked bool     // size and CRC were checked against Info and matched
}

// partPattern splits "dir/name.007" into "dir/name", 7 and the digit width.
func partPattern(path string) (prefix string, num, width int, ok bool) {
	ext := filepath.Ext(path)
	if len(ext) < 2 {
		return "", 0, 0, false
	}
	digits := ext[1:]
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 || strings.ContainsAny(digits, "+-") {
		return "", 0, 0, false
	}
	return strings.TrimSuffix(path, ext), n, len(digits), true
}

func numbered(prefix string, n, width int) string {
	return fmt.Sprintf("%s.%0*d", prefix, width, n)
}

// FindParts lists the consecutive numbered parts starting at the part with
// the lowest number for the given part (or .crc file) path. For "x.003" it
// starts at "x.001" if that exists, else at "x.000" or "x.003" itself.
func FindParts(path string) ([]string, error) {
	prefix, num, width, ok := partPattern(path)
	if !ok {
		if strings.EqualFold(filepath.Ext(path), ".crc") {
			prefix, num, width = strings.TrimSuffix(path, filepath.Ext(path)), 1, 3
		} else {
			return nil, fmt.Errorf("%s is not a numbered part such as name.001", filepath.Base(path))
		}
	}
	start := num
	for _, s := range []int{0, 1} {
		if s < num && exists(numbered(prefix, s, width)) {
			start = s
			break
		}
	}
	var parts []string
	for n := start; ; n++ {
		p := numbered(prefix, n, width)
		if !exists(p) {
			// Beyond 999 parts the numbers simply get wider.
			if w := len(strconv.Itoa(n)); w > width && exists(numbered(prefix, n, w)) {
				p = numbered(prefix, n, w)
			} else {
				break
			}
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return nil, &os.PathError{Op: "open", Path: numbered(prefix, start, width), Err: os.ErrNotExist}
	}
	return parts, nil
}

func exists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// CombineFiles joins parts into one file.
func CombineFiles(ctx context.Context, opts CombineOptions) (*CombineResult, error) {
	var parts []string
	var prefix string
	var startNum, width int
	auto := len(opts.Parts) == 0
	if auto {
		if opts.FirstPart == "" {
			return nil, errors.New("no part given")
		}
		var err error
		parts, err = FindParts(opts.FirstPart)
		if err != nil {
			return nil, err
		}
		prefix, startNum, width, _ = partPattern(parts[0])
	} else {
		parts = append(parts, opts.Parts...)
	}
	first := parts[0]

	var info *CRCInfo
	if !opts.NoCRCCheck {
		crcPath := opts.CRCFile
		if crcPath == "" {
			p, _, _, ok := partPattern(first)
			if !ok {
				p = strings.TrimSuffix(first, filepath.Ext(first))
			}
			if exists(p + ".crc") {
				crcPath = p + ".crc"
			}
		}
		if crcPath != "" {
			ci, err := ReadCRCInfo(crcPath)
			if err != nil {
				return nil, err
			}
			info = &ci
		}
	}

	dir := opts.TargetDir
	if dir == "" {
		dir = filepath.Dir(first)
	}
	name := opts.TargetName
	if name == "" && info != nil {
		name = info.FileName
	}
	if name == "" {
		if p, _, _, ok := partPattern(first); ok {
			name = filepath.Base(p)
		} else {
			name = filepath.Base(first) + ".combined"
		}
	}
	// The name may come from a .crc file someone sent us: never let it point
	// outside the target folder.
	name, err := safeFileName(name)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(dir, name)
	// Compare by file identity, not spelling: "DATA.001" on a case-insensitive
	// disk, or a symlinked target folder, still names part 1.
	if newFileSet(parts...).contains(target) {
		return nil, fmt.Errorf("the combined file %s would overwrite one of its parts", target)
	}
	if opts.CRCFile != "" && newFileSet(opts.CRCFile).contains(target) {
		return nil, fmt.Errorf("the combined file %s would overwrite the .crc file", target)
	}

	var total int64
	for _, p := range parts {
		s, err := fileSize(p)
		if err != nil {
			return nil, err
		}
		total += s
	}
	if info != nil && info.Size > total {
		total = info.Size
	}

	out, err := createOutput(target, opts.Overwrite)
	if err != nil {
		return nil, err
	}
	res := &CombineResult{Target: target, Info: info}
	crc := crc32.NewIEEE()
	w := io.MultiWriter(out, crc)
	t := newTracker(ctx, opts.Progress, total)
	fail := func(err error) (*CombineResult, error) {
		out.Close()
		os.Remove(target)
		return nil, err
	}

	appendPart := func(p string) error {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		ps, _ := fileSize(p)
		n, err := t.copyN(w, f, -1, p, 0, ps)
		res.Size += n
		res.Parts = append(res.Parts, p)
		return err
	}
	for _, p := range parts {
		if err := appendPart(p); err != nil {
			return fail(err)
		}
	}
	// With split info we know how much data is due; keep asking for the
	// next part while some is missing (parts spread over several disks).
	if auto {
		for next := startNum + len(parts); info != nil && res.Size < info.Size; next++ {
			expected := numbered(prefix, next, width)
			p := expected
			if !exists(p) {
				if opts.NextPart == nil {
					return fail(fmt.Errorf("part %s is missing", filepath.Base(expected)))
				}
				p, err = opts.NextPart(next-startNum+1, expected)
				if err != nil {
					return fail(err)
				}
			}
			if err := appendPart(p); err != nil {
				return fail(err)
			}
		}
	}
	if err := out.Close(); err != nil {
		os.Remove(target)
		return nil, err
	}
	res.CRC32 = crc.Sum32()
	if info != nil {
		if res.Size != info.Size {
			return res, fmt.Errorf("%w: size is %d bytes, expected %d", ErrCombineMismatch, res.Size, info.Size)
		}
		if res.CRC32 != info.CRC32 {
			return res, fmt.Errorf("%w: CRC32 is %08X, expected %08X", ErrCombineMismatch, res.CRC32, info.CRC32)
		}
		res.Checked = true
	}
	return res, nil
}

// safeFileName reduces a name taken from a file's content to a plain file
// name: no folders, no "..", no characters Windows forbids.
func safeFileName(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	name = name[strings.LastIndex(name, "/")+1:]
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("the stored file name %q cannot be used", name)
	}
	return name, nil
}
