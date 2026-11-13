// This file understands the names of multi-volume archives. A big archive is
// often split into parts so it fits on a disk or an upload limit, and every
// packer names the parts differently:
//
//   - RAR 3 and later:   name.part1.rar, name.part2.rar ... (also part01/part001)
//   - RAR 2 and older:   name.rar, name.r00, name.r01 ... name.r99, name.s00 ...
//   - 7-Zip and others:  name.7z.001, name.7z.002 ... (any "name.NNN" split)
//   - Zip (spanned):     name.z01, name.z02 ... name.zip (the .zip is the last)
//
// Total Commander lets the user press Enter on any of the parts and still
// opens the whole set from the first volume, and its "unpack each archive to
// a separate subfolder" option names the folder after the archive without the
// volume suffix. The helpers here give the UI exactly that: the first volume
// of a set, the ordered list of all volumes that exist on disk, and the plain
// archive name ("stem").

package sevenrar

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type volKind int

const (
	volNone     volKind = iota
	volRarNew           // name.partN.rar
	volRarOld           // name.rar / name.r00
	volSplit            // name.001 (7z and generic split files)
	volZipSplit         // name.z01 ... name.zip
)

// volName is a parsed volume file name.
type volName struct {
	kind  volKind
	stem  string // everything before the volume suffix, e.g. "backup" or "backup.7z"
	index int    // zero-based volume index
	width int    // digits in the volume number, kept when making other names
	ext   string // the original suffix text, to keep its letter case
}

var (
	reRarNew   = regexp.MustCompile(`(?i)^(.*)\.(part)(\d+)\.(rar|exe)$`)
	reRarOld   = regexp.MustCompile(`(?i)^(.*)\.([r-z])(\d\d)$`)
	reRarFirst = regexp.MustCompile(`(?i)^(.*)\.(rar)$`)
	reSplit    = regexp.MustCompile(`^(.*)\.(\d{3})$`)
	reZipSplit = regexp.MustCompile(`(?i)^(.*)\.(z)(\d\d+)$`)
)

// parseVolumeName recognises the volume naming schemes listed above. A plain
// "name.rar" is reported as volRarOld index 0: it is either a single archive
// or the first volume of an old-style set.
func parseVolumeName(base string) volName {
	if m := reRarNew.FindStringSubmatch(base); m != nil {
		n, _ := strconv.Atoi(m[3])
		if n >= 1 {
			return volName{kind: volRarNew, stem: m[1], index: n - 1, width: len(m[3]), ext: m[2] + "|" + m[4]}
		}
	}
	if m := reZipSplit.FindStringSubmatch(base); m != nil {
		n, _ := strconv.Atoi(m[3])
		if n >= 1 {
			return volName{kind: volZipSplit, stem: m[1], index: n - 1, width: len(m[3]), ext: m[2]}
		}
	}
	if m := reRarOld.FindStringSubmatch(base); m != nil {
		letter := strings.ToLower(m[2])[0]
		n, _ := strconv.Atoi(m[3])
		return volName{kind: volRarOld, stem: m[1], index: int(letter-'r')*100 + n + 1, width: 2, ext: m[2]}
	}
	if m := reRarFirst.FindStringSubmatch(base); m != nil {
		return volName{kind: volRarOld, stem: m[1], index: 0, width: 2, ext: m[2]}
	}
	if m := reSplit.FindStringSubmatch(base); m != nil {
		n, err := strconv.Atoi(m[2])
		if err == nil && n >= 1 {
			return volName{kind: volSplit, stem: m[1], index: n - 1, width: len(m[2])}
		}
	}
	return volName{kind: volNone}
}

// upperLike returns s in upper case if model is upper case, else s.
func upperLike(model, s string) string {
	if model != "" && model == strings.ToUpper(model) && model != strings.ToLower(model) {
		return strings.ToUpper(s)
	}
	return s
}

// nextVolumeName returns the path of volume number idx (zero based) of the
// set that path belongs to, using the naming scheme parsed into vn.
func nextVolumeName(path string, vn volName, idx int) string {
	dir := filepath.Dir(path)
	var base string
	switch vn.kind {
	case volRarNew:
		parts := strings.SplitN(vn.ext, "|", 2)
		base = fmt.Sprintf("%s.%s%0*d.%s", vn.stem, parts[0], vn.width, idx+1, parts[1])
	case volRarOld:
		if idx == 0 {
			base = vn.stem + "." + upperLike(vn.ext, "rar")
		} else {
			n := idx - 1
			letter := string(rune('r' + n/100))
			base = fmt.Sprintf("%s.%s%02d", vn.stem, upperLike(vn.ext, letter), n%100)
		}
	case volSplit:
		base = fmt.Sprintf("%s.%0*d", vn.stem, vn.width, idx+1)
	case volZipSplit:
		base = fmt.Sprintf("%s.%s%0*d", vn.stem, vn.ext, vn.width, idx+1)
	default:
		return path
	}
	return filepath.Join(dir, base)
}

// FirstVolume returns the path the archive must be opened from when path is
// any volume of a multi-volume set: "x.part3.rar" gives "x.part1.rar",
// "x.r05" gives "x.rar", "x.7z.004" gives "x.7z.001". For a spanned zip it
// returns the final "x.zip", which holds the central directory. For a file
// that is not a volume it returns path unchanged. If the first volume does
// not exist with the expected letter case, other cases are tried.
func FirstVolume(path string) string {
	vn := parseVolumeName(filepath.Base(path))
	if vn.kind == volNone {
		return path
	}
	if vn.kind == volZipSplit {
		return existingCase(filepath.Join(filepath.Dir(path), vn.stem+"."+upperLike(vn.ext, "zip")))
	}
	if vn.index == 0 {
		return path
	}
	return existingCase(nextVolumeName(path, vn, 0))
}

// existingCase returns p, or a variant of p whose extension differs only in
// letter case, whichever exists; p itself if none does.
func existingCase(p string) string {
	if _, err := os.Lstat(p); err == nil {
		return p
	}
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	for _, e := range []string{strings.ToLower(ext), strings.ToUpper(ext)} {
		if _, err := os.Lstat(stem + e); err == nil {
			return stem + e
		}
	}
	return p
}

// maxVolumes bounds the volume enumeration, so a directory with millions of
// numbered files cannot make FindVolumes run for ever.
const maxVolumes = 100000

// FindVolumes returns the ordered list of volume files of the set that path
// belongs to, starting with the first volume and stopping at the first
// missing number. For a file that is not part of a set the result is just
// []string{path}. A spanned zip is returned in reading order: x.z01, x.z02,
// ..., x.zip.
func FindVolumes(path string) ([]string, error) {
	vn := parseVolumeName(filepath.Base(path))
	if vn.kind == volNone {
		if _, err := os.Stat(path); err != nil {
			return nil, err
		}
		return []string{path}, nil
	}
	first := FirstVolume(path)
	if vn.kind == volZipSplit {
		var out []string
		for i := 0; i < maxVolumes; i++ {
			p := nextVolumeName(path, vn, i)
			if _, err := os.Stat(p); err != nil {
				break
			}
			out = append(out, p)
		}
		if _, err := os.Stat(first); err != nil {
			return out, &MissingVolumeError{Path: first, Err: err}
		}
		return append(out, first), nil
	}
	if _, err := os.Stat(first); err != nil {
		return nil, &MissingVolumeError{Path: first, Err: err}
	}
	fvn := parseVolumeName(filepath.Base(first))
	if vn.kind == volRarOld && vn.index == 0 {
		fvn = vn
	}
	out := []string{first}
	for i := 1; i < maxVolumes; i++ {
		p := existingCase(nextVolumeName(first, fvn, i))
		if _, err := os.Stat(p); err != nil {
			break
		}
		out = append(out, p)
	}
	return out, nil
}

// archiveExts are the extensions ArchiveStem strips, longest first so that
// ".tar.gz" wins over ".gz".
var archiveExts = []string{
	".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst", ".tar.lz4", ".tar.z",
	".7z", ".rar", ".zip", ".tar", ".tgz", ".tbz", ".tbz2", ".txz", ".tzst",
	".gz", ".bz2", ".xz", ".zst", ".lz4", ".z", ".cab", ".iso", ".arj", ".lzh", ".lha",
	".jar", ".apk", ".exe", ".sfx",
}

// ArchiveStem returns the archive's file name without its archive and volume
// suffixes, which is the folder name Total Commander uses for "unpack each
// archive to a separate subfolder": "photos.part01.rar" gives "photos",
// "backup.7z.001" gives "backup", "src.tar.gz" gives "src". If nothing would
// remain, the base name is returned unchanged.
func ArchiveStem(path string) string {
	base := filepath.Base(path)
	name := base
	if vn := parseVolumeName(name); vn.kind != volNone {
		name = vn.stem
		if vn.kind == volRarOld || vn.kind == volZipSplit || vn.kind == volRarNew {
			return nonEmpty(name, base)
		}
	}
	lower := strings.ToLower(name)
	for _, e := range archiveExts {
		if strings.HasSuffix(lower, e) && len(name) > len(e) {
			return name[:len(name)-len(e)]
		}
	}
	return nonEmpty(name, base)
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
