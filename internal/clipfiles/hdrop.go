// This file builds and parses the Windows clipboard formats for files. Explorer puts copied or cut files on the
// clipboard as CF_HDROP: a DROPFILES header (20 bytes) followed by the paths as NUL-terminated UTF-16 strings and one
// extra NUL at the end. Whether the files were cut is told by a second format, "Preferred DropEffect", a 4-byte
// little-endian DWORD holding DROPEFFECT_MOVE (2) for a cut and DROPEFFECT_COPY (1) for a copy (Explorer writes 5,
// COPY|LINK, for a copy, so only the MOVE bit decides). The encoding is plain byte work with no Windows API calls,
// so it lives in a platform-neutral file and is tested on every OS; clip_windows.go copies these bytes in and out of
// global memory handles.

package clipfiles

import (
	"encoding/binary"
	"errors"
	"unicode/utf16"
)

// Windows clipboard constants shared by the encoder and the Windows backend.
const (
	dropFilesHeaderSize = 20
	dropEffectCopy      = 1
	dropEffectMove      = 2
	dropEffectLink      = 4
	// FormatPreferredDropEffect is the registered clipboard format name Explorer uses to mark a cut.
	FormatPreferredDropEffect = "Preferred DropEffect"
)

// EncodeHDROP builds a CF_HDROP block (DROPFILES header plus a double-NUL-terminated UTF-16 path list).
func EncodeHDROP(paths []string) []byte {
	var units []uint16
	for _, p := range paths {
		units = append(units, utf16.Encode([]rune(p))...)
		units = append(units, 0)
	}
	units = append(units, 0)
	if len(paths) == 0 {
		units = append(units, 0) // an empty list is still double-NUL terminated
	}
	b := make([]byte, dropFilesHeaderSize+2*len(units))
	binary.LittleEndian.PutUint32(b[0:], dropFilesHeaderSize) // pFiles: offset of the path list
	// pt.x, pt.y and fNC stay zero: the drop point means nothing for a clipboard paste.
	binary.LittleEndian.PutUint32(b[16:], 1) // fWide: the paths are UTF-16
	for i, u := range units {
		binary.LittleEndian.PutUint16(b[dropFilesHeaderSize+2*i:], u)
	}
	return b
}

// DecodeHDROP reads a CF_HDROP block. It handles both the wide (UTF-16) and the old ANSI form; ANSI bytes are read as
// Latin-1, which is right for ASCII names and the best that can be done without the writer's code page.
func DecodeHDROP(b []byte) ([]string, error) {
	if len(b) < dropFilesHeaderSize {
		return nil, errors.New("CF_HDROP block is too short")
	}
	off := int(binary.LittleEndian.Uint32(b[0:]))
	wide := binary.LittleEndian.Uint32(b[16:]) != 0
	if off < dropFilesHeaderSize || off > len(b) {
		return nil, errors.New("CF_HDROP block has a bad file list offset")
	}
	var out []string
	data := b[off:]
	if wide {
		var cur []uint16
		for i := 0; i+1 < len(data); i += 2 {
			u := binary.LittleEndian.Uint16(data[i:])
			if u != 0 {
				cur = append(cur, u)
				continue
			}
			if len(cur) == 0 { // the second NUL in a row ends the list
				return out, nil
			}
			out = append(out, string(utf16.Decode(cur)))
			cur = cur[:0]
		}
		if len(cur) > 0 {
			out = append(out, string(utf16.Decode(cur)))
		}
		return out, nil
	}
	var cur []rune
	for _, c := range data {
		if c != 0 {
			cur = append(cur, rune(c))
			continue
		}
		if len(cur) == 0 {
			return out, nil
		}
		out = append(out, string(cur))
		cur = cur[:0]
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out, nil
}

// EncodeDropEffect builds the "Preferred DropEffect" DWORD for op.
func EncodeDropEffect(op Op) []byte {
	v := uint32(dropEffectCopy)
	if op == Cut {
		v = dropEffectMove
	}
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// DecodeDropEffect reads a "Preferred DropEffect" DWORD. Only the MOVE bit means cut; anything else, including a
// missing or short value, is a copy.
func DecodeDropEffect(b []byte) Op {
	if len(b) >= 4 && binary.LittleEndian.Uint32(b)&dropEffectMove != 0 {
		return Cut
	}
	return Copy
}
