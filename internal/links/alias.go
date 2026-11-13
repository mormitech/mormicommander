// A macOS Finder alias is the Mac counterpart of a Windows .lnk: a small
// file that remembers where a file or folder lives. Since Mac OS X 10.6 the
// alias file's data fork holds "bookmark data", the same structure that
// NSURL bookmarkData produces. Its layout is undocumented but stable and well
// described by open-source readers: a header starting with "book", then a
// data area whose first word points at a table of contents; each TOC entry
// maps a numeric key to a typed record, and key 0x1004 is an array of path
// components (UTF-8 strings) from the volume root down to the target. We read
// just that, which is enough for Enter / Ctrl+PgDn to follow an alias the way
// TC follows a .lnk. Volume UUIDs, file IDs and security scopes are ignored;
// if the target moved, Finder's own resolution would find it and we report
// the stored path as broken. The header has two known variants (the plain
// in-memory one and the on-disk one with "mark" at offset 8), so the header
// size is found by checking candidates against the TOC magic.

package links

import (
	"bytes"
	"encoding/binary"
	"path"
)

const (
	bmTOCMagic       = 0xFFFFFFFE
	bmKeyPath        = 0x1004
	bmKeyVolumePath  = 0x2002
	bmTypeString     = 0x0101
	bmTypeArray      = 0x0601
	bmMaxTOCs        = 64
	bmMaxPathEntries = 256
)

// IsAliasData reports whether data looks like a Finder alias (bookmark).
func IsAliasData(data []byte) bool {
	return len(data) >= 16 && bytes.Equal(data[:4], []byte("book"))
}

// ReadAlias returns the POSIX path stored in the Finder alias at p.
func ReadAlias(p string) (string, error) {
	data, err := readSmall(p, 4<<20)
	if err != nil {
		return "", err
	}
	return ParseAlias(data)
}

// ParseAlias returns the POSIX path stored in bookmark data.
func ParseAlias(data []byte) (string, error) {
	if !IsAliasData(data) {
		return "", ErrBadShortcut
	}
	le := binary.LittleEndian
	u32 := func(off int) (uint32, bool) {
		if off < 0 || off+4 > len(data) {
			return 0, false
		}
		return le.Uint32(data[off:]), true
	}
	var hdr int
	cands := []int{}
	if v, ok := u32(12); ok {
		cands = append(cands, int(v))
	}
	if v, ok := u32(20); ok {
		cands = append(cands, int(v))
	}
	cands = append(cands, 0x30, 0x34, 0x38, 0x40)
	for _, c := range cands {
		if c < 16 || c+4 > len(data) {
			continue
		}
		toc, _ := u32(c)
		if m, ok := u32(c + int(toc) + 4); ok && toc != 0 && m == bmTOCMagic {
			hdr = c
			break
		}
	}
	if hdr == 0 {
		return "", ErrBadShortcut
	}
	// record returns the type and payload of the record at a data offset.
	record := func(off uint32) (uint32, []byte, bool) {
		o := hdr + int(off)
		n, ok1 := u32(o)
		t, ok2 := u32(o + 4)
		if !ok1 || !ok2 || o+8+int(n) > len(data) || int(n) < 0 {
			return 0, nil, false
		}
		return t, data[o+8 : o+8+int(n)], true
	}
	str := func(off uint32) (string, bool) {
		t, b, ok := record(off)
		if !ok || t != bmTypeString {
			return "", false
		}
		return string(b), true
	}
	var comps []string
	volPath := ""
	tocOff, _ := u32(hdr)
	for i := 0; tocOff != 0 && i < bmMaxTOCs; i++ {
		base := hdr + int(tocOff)
		magic, ok := u32(base + 4)
		next, _ := u32(base + 12)
		count, ok2 := u32(base + 16)
		if !ok || !ok2 || magic != bmTOCMagic || count > 4096 {
			break
		}
		for j := 0; j < int(count); j++ {
			e := base + 20 + j*12
			key, ok1 := u32(e)
			val, ok2 := u32(e + 4)
			if !ok1 || !ok2 {
				break
			}
			switch key &^ 0x80000000 {
			case bmKeyPath:
				t, b, ok := record(val)
				if !ok || t != bmTypeArray || len(comps) > 0 {
					continue
				}
				for k := 0; k+4 <= len(b) && k/4 < bmMaxPathEntries; k += 4 {
					if s, ok := str(le.Uint32(b[k:])); ok {
						comps = append(comps, s)
					}
				}
			case bmKeyVolumePath:
				if s, ok := str(val); ok {
					volPath = s
				}
			}
		}
		tocOff = next
	}
	if len(comps) == 0 {
		return "", ErrBadShortcut
	}
	p := "/" + path.Join(comps...)
	// Components are relative to the root of the boot volume; for other
	// volumes they already start with "Volumes/<name>". Only fall back on
	// the volume path when the components clearly are not absolute.
	if volPath != "" && volPath != "/" && !bytes.HasPrefix([]byte(p), []byte(volPath)) && comps[0] != "Volumes" {
		p = path.Join(volPath, p)
	}
	return p, nil
}
