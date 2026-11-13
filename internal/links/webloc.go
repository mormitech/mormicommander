// A .webloc file is what macOS Finder and Safari create when a URL is dragged
// to a folder; it is a property list whose top-level dictionary holds the
// address under the key "URL". TC opens .url files in the browser on Enter
// (F-738, F-746), and on macOS the same must work for .webloc. Finder writes
// these files either as XML or in Apple's binary plist format ("bplist00"),
// so both are read here. Only the small subset we need is decoded: the top
// dictionary's string values. That keeps the reader free of dependencies and
// easy to keep bounds-safe. New .webloc files are written as XML, which every
// macOS version accepts. A file:// URL is fine too, which is how the Create
// shortcut command makes a Finder-openable shortcut to a local file.

package links

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"io"
	"strings"
	"unicode/utf16"
)

// ReadWebloc returns the URL stored in the .webloc file at path.
func ReadWebloc(path string) (string, error) {
	data, err := readSmall(path, 1<<20)
	if err != nil {
		return "", err
	}
	return ParseWebloc(data)
}

// ParseWebloc returns the URL from .webloc bytes (XML or binary plist).
func ParseWebloc(data []byte) (string, error) {
	m, err := ParsePlistStrings(data)
	if err != nil {
		return "", err
	}
	u := strings.TrimSpace(m["URL"])
	if u == "" {
		return "", ErrBadShortcut
	}
	return u, nil
}

// ParsePlistStrings decodes the string values of a property list's top-level
// dictionary, in XML or binary format. Non-string values are skipped.
func ParsePlistStrings(data []byte) (map[string]string, error) {
	if bytes.HasPrefix(data, []byte("bplist00")) {
		return parseBinaryPlist(data)
	}
	return parseXMLPlist(data)
}

func parseXMLPlist(data []byte) (map[string]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	out := map[string]string{}
	depth := 0 // dict nesting depth
	key := ""
	inKey, inString := false, false
	var text strings.Builder
	found := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, ErrBadShortcut
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "dict":
				depth++
				found = true
			case "key":
				inKey = depth == 1
				text.Reset()
			case "string":
				inString = depth == 1
				text.Reset()
			}
		case xml.CharData:
			if inKey || inString {
				text.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "dict":
				depth--
			case "key":
				if inKey {
					key = text.String()
				}
				inKey = false
			case "string":
				if inString && key != "" {
					out[key] = text.String()
				}
				inString = false
				key = ""
			default:
				if depth == 1 && t.Name.Local != "key" {
					key = ""
				}
			}
		}
	}
	if !found {
		return nil, ErrBadShortcut
	}
	return out, nil
}

// parseBinaryPlist reads the top-level dictionary of a bplist00 file.
func parseBinaryPlist(data []byte) (map[string]string, error) {
	if len(data) < 8+32 {
		return nil, ErrBadShortcut
	}
	tr := data[len(data)-32:]
	offSize, refSize := int(tr[6]), int(tr[7])
	numObj := binary.BigEndian.Uint64(tr[8:])
	top := binary.BigEndian.Uint64(tr[16:])
	table := binary.BigEndian.Uint64(tr[24:])
	if offSize < 1 || offSize > 8 || refSize < 1 || refSize > 8 || numObj == 0 || top >= numObj ||
		table >= uint64(len(data)) || numObj > uint64(len(data)) || table+numObj*uint64(offSize) > uint64(len(data)) {
		return nil, ErrBadShortcut
	}
	objOffset := func(idx uint64) (int, bool) {
		if idx >= numObj {
			return 0, false
		}
		p := table + idx*uint64(offSize)
		o := beUint(data[p : p+uint64(offSize)])
		if o >= uint64(len(data))-32 {
			return 0, false
		}
		return int(o), true
	}
	// length decodes an object's element count, which is either in the low
	// nibble or, when that is 0xF, in a following integer object.
	length := func(off int) (n int, start int, ok bool) {
		info := int(data[off] & 0x0F)
		if info != 0x0F {
			return info, off + 1, true
		}
		if off+2 > len(data) || data[off+1]>>4 != 0x1 {
			return 0, 0, false
		}
		sz := 1 << (data[off+1] & 0x0F)
		if sz > 8 || off+2+sz > len(data) {
			return 0, 0, false
		}
		v := beUint(data[off+2 : off+2+sz])
		if v > uint64(len(data)) {
			return 0, 0, false
		}
		return int(v), off + 2 + sz, true
	}
	str := func(idx uint64) (string, bool) {
		off, ok := objOffset(idx)
		if !ok {
			return "", false
		}
		n, start, ok := length(off)
		if !ok {
			return "", false
		}
		switch data[off] >> 4 {
		case 0x5: // ASCII
			if start+n > len(data) {
				return "", false
			}
			return string(data[start : start+n]), true
		case 0x6: // UTF-16BE
			if start+2*n > len(data) {
				return "", false
			}
			u := make([]uint16, n)
			for i := range u {
				u[i] = binary.BigEndian.Uint16(data[start+2*i:])
			}
			return string(utf16.Decode(u)), true
		}
		return "", false
	}
	off, ok := objOffset(top)
	if !ok || data[off]>>4 != 0xD {
		return nil, ErrBadShortcut
	}
	n, start, ok := length(off)
	if !ok || start+2*n*refSize > len(data) {
		return nil, ErrBadShortcut
	}
	out := map[string]string{}
	for i := 0; i < n; i++ {
		kp := start + i*refSize
		vp := start + (n+i)*refSize
		k, ok1 := str(beUint(data[kp : kp+refSize]))
		v, ok2 := str(beUint(data[vp : vp+refSize]))
		if ok1 && ok2 {
			out[k] = v
		}
	}
	return out, nil
}

func beUint(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}

// WeblocData returns the XML .webloc content for url.
func WeblocData(url string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>URL</key>
	<string>`)
	xml.EscapeText(&b, []byte(url))
	b.WriteString("</string>\n</dict>\n</plist>\n")
	return b.Bytes()
}
