// Property lists, the settings format of macOS. The "Open with" list on macOS has to read each application's
// Info.plist to learn which file types it opens, the user's default apps live in the LaunchServices plist, and
// .webloc shortcuts are plists too. These files come in two shapes, XML and the compact binary "bplist00" format,
// and both are read here with the standard library only. The result is plain Go values: map[string]any, []any,
// string, bool, int64, float64 and []byte. Malformed files give an error, never a panic or an endless loop.
package launch

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

var errPlist = errors.New("launch: malformed property list")

// ParsePlist reads an XML or binary property list.
func ParsePlist(data []byte) (any, error) {
	if bytes.HasPrefix(data, []byte("bplist00")) {
		return parseBinaryPlist(data)
	}
	return parseXMLPlist(data)
}

func parseXMLPlist(data []byte) (any, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, errPlist
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local != "plist" {
			return xmlValue(d, se, 0)
		}
	}
}

func xmlValue(d *xml.Decoder, se xml.StartElement, depth int) (any, error) {
	if depth > 64 {
		return nil, errPlist
	}
	switch se.Name.Local {
	case "dict":
		m := map[string]any{}
		key := ""
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, errPlist
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					s, err := xmlText(d)
					if err != nil {
						return nil, err
					}
					key = s
					continue
				}
				v, err := xmlValue(d, t, depth+1)
				if err != nil {
					return nil, err
				}
				m[key] = v
			case xml.EndElement:
				return m, nil
			}
		}
	case "array":
		var a []any
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, errPlist
			}
			switch t := tok.(type) {
			case xml.StartElement:
				v, err := xmlValue(d, t, depth+1)
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			case xml.EndElement:
				return a, nil
			}
		}
	case "true", "false":
		if err := d.Skip(); err != nil {
			return nil, errPlist
		}
		return se.Name.Local == "true", nil
	}
	s, err := xmlText(d)
	if err != nil {
		return nil, err
	}
	switch se.Name.Local {
	case "integer":
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return nil, errPlist
		}
		return n, nil
	case "real":
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return nil, errPlist
		}
		return f, nil
	case "data":
		b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
		if err != nil {
			return nil, errPlist
		}
		return b, nil
	}
	return s, nil // string, date and anything unknown
}

// xmlText reads the character data up to the end of the current element.
func xmlText(d *xml.Decoder) (string, error) {
	var b strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return "", errPlist
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			return b.String(), nil
		case xml.StartElement:
			if err := d.Skip(); err != nil {
				return "", errPlist
			}
		}
	}
}

type bplist struct {
	data      []byte
	offsets   []uint64
	refSize   int
	visiting  map[uint64]bool
	remaining int // object budget, against files that reference the same objects over and over
}

func parseBinaryPlist(data []byte) (any, error) {
	if len(data) < 8+32 {
		return nil, errPlist
	}
	tr := data[len(data)-32:]
	offSize := int(tr[6])
	refSize := int(tr[7])
	numObjects := binary.BigEndian.Uint64(tr[8:16])
	top := binary.BigEndian.Uint64(tr[16:24])
	tableOff := binary.BigEndian.Uint64(tr[24:32])
	if offSize < 1 || offSize > 8 || refSize < 1 || refSize > 8 || numObjects == 0 || top >= numObjects ||
		tableOff >= uint64(len(data)) || numObjects > uint64(len(data)) ||
		tableOff+numObjects*uint64(offSize) > uint64(len(data)-32) {
		return nil, errPlist
	}
	p := &bplist{data: data, refSize: refSize, visiting: map[uint64]bool{}, remaining: 1 << 20}
	p.offsets = make([]uint64, numObjects)
	for i := range p.offsets {
		p.offsets[i] = readUint(data[tableOff+uint64(i*offSize):], offSize)
	}
	return p.object(top)
}

func readUint(b []byte, n int) uint64 {
	var v uint64
	for i := 0; i < n; i++ {
		v = v<<8 | uint64(b[i])
	}
	return v
}

func (p *bplist) object(ref uint64) (any, error) {
	if ref >= uint64(len(p.offsets)) || p.visiting[ref] {
		return nil, errPlist
	}
	p.remaining--
	if p.remaining < 0 {
		return nil, errPlist
	}
	off := p.offsets[ref]
	if off >= uint64(len(p.data)-32) {
		return nil, errPlist
	}
	marker := p.data[off]
	hi, lo := marker>>4, int(marker&0x0F)
	pos := off + 1
	// count reads the length that follows container and string markers.
	count := func() (int, error) {
		if lo != 0x0F {
			return lo, nil
		}
		if pos >= uint64(len(p.data)) {
			return 0, errPlist
		}
		m := p.data[pos]
		if m>>4 != 0x1 {
			return 0, errPlist
		}
		n := 1 << (m & 0x0F)
		if n > 8 || pos+1+uint64(n) > uint64(len(p.data)) {
			return 0, errPlist
		}
		v := readUint(p.data[pos+1:], n)
		pos += 1 + uint64(n)
		if v > uint64(len(p.data)) {
			return 0, errPlist
		}
		return int(v), nil
	}
	need := func(n uint64) bool { return pos+n <= uint64(len(p.data)) }
	switch hi {
	case 0x0:
		switch marker {
		case 0x08:
			return false, nil
		case 0x09:
			return true, nil
		}
		return nil, nil
	case 0x1:
		n := uint64(1) << lo
		if n > 8 || !need(n) {
			return nil, errPlist
		}
		return int64(readUint(p.data[pos:], int(n))), nil
	case 0x2, 0x3:
		n := uint64(1) << lo
		if hi == 0x3 {
			n = 8
		}
		if !need(n) {
			return nil, errPlist
		}
		switch n {
		case 4:
			return float64(math.Float32frombits(uint32(readUint(p.data[pos:], 4)))), nil
		case 8:
			return math.Float64frombits(readUint(p.data[pos:], 8)), nil
		}
		return nil, errPlist
	case 0x4, 0x5:
		n, err := count()
		if err != nil || !need(uint64(n)) {
			return nil, errPlist
		}
		b := p.data[pos : pos+uint64(n)]
		if hi == 0x4 {
			return append([]byte(nil), b...), nil
		}
		return string(b), nil
	case 0x6:
		n, err := count()
		if err != nil || !need(uint64(n)*2) {
			return nil, errPlist
		}
		u := make([]uint16, n)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(p.data[pos+uint64(2*i):])
		}
		return string(utf16.Decode(u)), nil
	case 0x8:
		n := uint64(lo + 1)
		if !need(n) {
			return nil, errPlist
		}
		return int64(readUint(p.data[pos:], int(n))), nil
	case 0xA, 0xD:
		n, err := count()
		if err != nil {
			return nil, errPlist
		}
		refs := uint64(n)
		if hi == 0xD {
			refs *= 2
		}
		if !need(refs * uint64(p.refSize)) {
			return nil, errPlist
		}
		refAt := func(i uint64) uint64 { return readUint(p.data[pos+i*uint64(p.refSize):], p.refSize) }
		// A container that contains itself (directly or not) would recurse forever.
		p.visiting[ref] = true
		defer delete(p.visiting, ref)
		if hi == 0xA {
			a := make([]any, 0, n)
			for i := uint64(0); i < uint64(n); i++ {
				v, err := p.object(refAt(i))
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			}
			return a, nil
		}
		m := make(map[string]any, n)
		for i := uint64(0); i < uint64(n); i++ {
			k, err := p.object(refAt(i))
			if err != nil {
				return nil, err
			}
			ks, ok := k.(string)
			if !ok {
				return nil, errPlist
			}
			v, err := p.object(refAt(uint64(n) + i))
			if err != nil {
				return nil, err
			}
			m[ks] = v
		}
		return m, nil
	}
	return nil, errPlist
}
