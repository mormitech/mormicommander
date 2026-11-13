// Tests for the Finder alias (bookmark data) reader, using bookmark blobs
// built here in the documented layout: header, data area, string records,
// a path-component array under key 0x1004 and a table of contents. Both the
// in-memory header and the on-disk variant with "mark" at offset 8 are
// covered, as are corrupted blobs, which must fail cleanly.

package links

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func buildBookmark(comps []string, onDisk bool) []byte {
	le := binary.LittleEndian
	const hdr = 0x30
	var dataArea bytes.Buffer
	dataArea.Write(make([]byte, 4)) // first word: TOC offset, patched below
	rec := func(typ uint32, payload []byte) uint32 {
		off := uint32(dataArea.Len())
		binary.Write(&dataArea, le, uint32(len(payload)))
		binary.Write(&dataArea, le, typ)
		dataArea.Write(payload)
		for dataArea.Len()%4 != 0 {
			dataArea.WriteByte(0)
		}
		return off
	}
	var arr []byte
	for _, c := range comps {
		arr = le.AppendUint32(arr, rec(bmTypeString, []byte(c)))
	}
	pathOff := rec(bmTypeArray, arr)
	volOff := rec(bmTypeString, []byte("/"))
	tocOff := uint32(dataArea.Len())
	toc := make([]byte, 20+2*12)
	le.PutUint32(toc[0:], uint32(len(toc)-8))
	le.PutUint32(toc[4:], bmTOCMagic)
	le.PutUint32(toc[8:], 1)
	le.PutUint32(toc[12:], 0) // no next TOC
	le.PutUint32(toc[16:], 2)
	le.PutUint32(toc[20:], bmKeyPath)
	le.PutUint32(toc[24:], pathOff)
	le.PutUint32(toc[32:], bmKeyVolumePath)
	le.PutUint32(toc[36:], volOff)
	dataArea.Write(toc)
	d := dataArea.Bytes()
	le.PutUint32(d[0:], tocOff)

	head := make([]byte, hdr)
	copy(head, "book")
	if onDisk {
		copy(head[8:], "mark")
		le.PutUint32(head[16:], uint32(hdr+len(d)))
		le.PutUint32(head[20:], hdr)
	} else {
		le.PutUint32(head[4:], uint32(hdr+len(d)))
		le.PutUint32(head[8:], 0x10040000)
		le.PutUint32(head[12:], hdr)
	}
	return append(head, d...)
}

func TestParseAlias(t *testing.T) {
	for _, onDisk := range []bool{false, true} {
		data := buildBookmark([]string{"Users", "me", "Documents", "Report é.pdf"}, onDisk)
		got, err := ParseAlias(data)
		if err != nil {
			t.Fatalf("onDisk=%v: %v", onDisk, err)
		}
		if got != "/Users/me/Documents/Report é.pdf" {
			t.Errorf("onDisk=%v: got %q", onDisk, got)
		}
		if !IsAliasData(data) {
			t.Error("IsAliasData = false")
		}
	}
	if _, err := ParseAlias([]byte("bookmark? no.....")); !errors.Is(err, ErrBadShortcut) {
		t.Errorf("garbage: %v", err)
	}
	if _, err := ParseAlias(buildBookmark(nil, false)); !errors.Is(err, ErrBadShortcut) {
		t.Errorf("no components: %v", err)
	}
}

func TestParseAliasCorrupt(t *testing.T) {
	data := buildBookmark([]string{"a", "b"}, true)
	for n := 0; n < len(data); n++ {
		_, _ = ParseAlias(data[:n])
	}
	for i := 4; i < len(data); i++ {
		c := append([]byte(nil), data...)
		c[i] ^= 0xFF
		_, _ = ParseAlias(c)
	}
}
