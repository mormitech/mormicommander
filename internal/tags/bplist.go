package tags

// A tiny binary property list codec for the one shape Finder tags use: an array of strings. macOS keeps a file's tags
// in the extended attribute com.apple.metadata:_kMDItemUserTags as such a list ("bplist00", the objects, an offset
// table and a 32-byte trailer). Only arrays of strings are written; reading skips anything in the array that is not a
// string and refuses lists that are cut off or point outside themselves, so a broken attribute reads as no tags.

import (
	"encoding/binary"
	"errors"
	"unicode/utf16"
	"unicode/utf8"
)

var errPlist = errors.New("not a binary property list of strings")

// encodeStrings writes a binary property list holding an array of the strings.
func encodeStrings(list []string) []byte {
	n := len(list) + 1 // the array and its strings
	refSize := 1
	if n > 255 {
		refSize = 2
	}
	out := []byte("bplist00")
	offsets := make([]int, 0, n)
	// object 0: the array
	offsets = append(offsets, len(out))
	out = appendMarker(out, 0xA, len(list))
	for i := range list {
		out = appendUint(out, uint64(i+1), refSize)
	}
	for _, s := range list {
		offsets = append(offsets, len(out))
		ascii := true
		for i := 0; i < len(s); i++ {
			if s[i] >= 0x80 {
				ascii = false
				break
			}
		}
		if ascii {
			out = appendMarker(out, 0x5, len(s))
			out = append(out, s...)
			continue
		}
		units := utf16.Encode([]rune(s))
		out = appendMarker(out, 0x6, len(units))
		for _, u := range units {
			out = binary.BigEndian.AppendUint16(out, u)
		}
	}
	tableAt := len(out)
	offSize := sizeFor(uint64(tableAt))
	for _, o := range offsets {
		out = appendUint(out, uint64(o), offSize)
	}
	trailer := make([]byte, 32)
	trailer[6] = byte(offSize)
	trailer[7] = byte(refSize)
	binary.BigEndian.PutUint64(trailer[8:], uint64(n))
	binary.BigEndian.PutUint64(trailer[16:], 0)
	binary.BigEndian.PutUint64(trailer[24:], uint64(tableAt))
	return append(out, trailer...)
}

func sizeFor(v uint64) int {
	switch {
	case v <= 0xFF:
		return 1
	case v <= 0xFFFF:
		return 2
	case v <= 0xFFFFFFFF:
		return 4
	}
	return 8
}

func appendUint(b []byte, v uint64, size int) []byte {
	for i := size - 1; i >= 0; i-- {
		b = append(b, byte(v>>(8*uint(i))))
	}
	return b
}

// appendMarker writes an object marker with its count; counts of 15 and more follow as an int object.
func appendMarker(b []byte, kind byte, count int) []byte {
	if count < 15 {
		return append(b, kind<<4|byte(count))
	}
	b = append(b, kind<<4|0xF)
	size := sizeFor(uint64(count))
	exp := map[int]byte{1: 0, 2: 1, 4: 2, 8: 3}[size]
	b = append(b, 0x10|exp)
	return appendUint(b, uint64(count), size)
}

// decodeStrings reads the strings of a binary property list whose top object is an array.
func decodeStrings(data []byte) ([]string, error) {
	if len(data) < 8+32 || string(data[:7]) != "bplist0" {
		return nil, errPlist
	}
	tr := data[len(data)-32:]
	offSize, refSize := int(tr[6]), int(tr[7])
	numObjects := binary.BigEndian.Uint64(tr[8:])
	top := binary.BigEndian.Uint64(tr[16:])
	tableAt := binary.BigEndian.Uint64(tr[24:])
	end := uint64(len(data) - 32)
	if offSize < 1 || offSize > 8 || refSize < 1 || refSize > 8 || numObjects == 0 || numObjects > end ||
		top >= numObjects || tableAt >= end || tableAt+numObjects*uint64(offSize) > end {
		return nil, errPlist
	}
	readUint := func(at uint64, size int) (uint64, bool) {
		if at+uint64(size) > end {
			return 0, false
		}
		var v uint64
		for i := 0; i < size; i++ {
			v = v<<8 | uint64(data[at+uint64(i)])
		}
		return v, true
	}
	offsetOf := func(obj uint64) (uint64, bool) {
		if obj >= numObjects {
			return 0, false
		}
		o, ok := readUint(tableAt+obj*uint64(offSize), offSize)
		return o, ok && o >= 8 && o < tableAt
	}
	// count reads the count of the object at off and returns it with where its contents start
	count := func(off uint64) (uint64, uint64, bool) {
		c := uint64(data[off] & 0xF)
		at := off + 1
		if c != 0xF {
			return c, at, true
		}
		if at >= end || data[at]&0xF0 != 0x10 {
			return 0, 0, false
		}
		size := 1 << (data[at] & 0xF)
		if size > 8 {
			return 0, 0, false
		}
		v, ok := readUint(at+1, size)
		return v, at + 1 + uint64(size), ok
	}
	off, ok := offsetOf(top)
	if !ok || data[off]>>4 != 0xA {
		return nil, errPlist
	}
	n, at, ok := count(off)
	if !ok || n > numObjects || at+n*uint64(refSize) > end {
		return nil, errPlist
	}
	out := make([]string, 0, n)
	for i := uint64(0); i < n; i++ {
		ref, _ := readUint(at+i*uint64(refSize), refSize)
		so, ok := offsetOf(ref)
		if !ok {
			return nil, errPlist
		}
		kind := data[so] >> 4
		if kind != 0x5 && kind != 0x6 && kind != 0x7 {
			continue // not a string
		}
		c, from, ok := count(so)
		if !ok {
			return nil, errPlist
		}
		switch kind {
		case 0x5, 0x7: // ASCII, UTF-8
			if from+c > end {
				return nil, errPlist
			}
			s := data[from : from+c]
			if !utf8.Valid(s) {
				return nil, errPlist
			}
			out = append(out, string(s))
		case 0x6: // UTF-16 big-endian
			if from+2*c > end {
				return nil, errPlist
			}
			units := make([]uint16, c)
			for k := range units {
				units[k] = binary.BigEndian.Uint16(data[from+2*uint64(k):])
			}
			out = append(out, string(utf16.Decode(units)))
		}
	}
	return out, nil
}
