// EXIF and TIFF tag reading. Photos from cameras and phones carry their
// metadata as a small TIFF structure (inside the JPEG APP1 segment, a PNG
// eXIf chunk, a WebP EXIF chunk, or the TIFF file itself). Total Commander's
// Lister shows it through its image plugin as "EXIF info": camera make and
// model, date taken, exposure, aperture, ISO, focal length, lens, GPS
// position and the orientation flag that says how to turn the image upright.
// This file decodes just those tags; anything unusual is skipped rather than
// treated as an error, because metadata written by odd software is common.

package lister

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

type tiffEntry struct {
	tag, typ uint16
	count    uint32
	data     []byte // the value bytes (inline or pointed to)
}

type tiffInfo struct {
	order binary.ByteOrder
	ifd0  map[uint16]uint64 // first numeric value of each IFD0 tag
	tags  map[uint16]tiffEntry
	exif  map[uint16]tiffEntry
	gps   map[uint16]tiffEntry
	pages int
}

var tiffTypeSize = map[uint16]int{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 6: 1, 7: 1, 8: 2, 9: 4, 10: 8, 11: 4, 12: 8}

// readIFD parses one image file directory at off and returns its entries
// and the offset of the next IFD.
func readIFD(d []byte, off uint32, order binary.ByteOrder) (map[uint16]tiffEntry, uint32) {
	if int64(off)+2 > int64(len(d)) {
		return nil, 0
	}
	n := int(order.Uint16(d[off:]))
	out := make(map[uint16]tiffEntry, n)
	pos := int(off) + 2
	for i := 0; i < n && pos+12 <= len(d); i++ {
		e := tiffEntry{tag: order.Uint16(d[pos:]), typ: order.Uint16(d[pos+2:]), count: order.Uint32(d[pos+4:])}
		sz := tiffTypeSize[e.typ] * int(e.count)
		if sz > 0 && sz <= 1<<20 {
			if sz <= 4 {
				e.data = d[pos+8 : pos+8+sz]
			} else if vo := int64(order.Uint32(d[pos+8:])); vo+int64(sz) <= int64(len(d)) {
				e.data = d[vo : vo+int64(sz)]
			}
		}
		out[e.tag] = e
		pos += 12
	}
	next := uint32(0)
	if pos+4 <= len(d) {
		next = order.Uint32(d[pos:])
	}
	return out, next
}

func (e tiffEntry) uint(order binary.ByteOrder, i int) uint64 {
	switch e.typ {
	case 1, 7:
		if i < len(e.data) {
			return uint64(e.data[i])
		}
	case 3, 8:
		if 2*i+2 <= len(e.data) {
			return uint64(order.Uint16(e.data[2*i:]))
		}
	case 4, 9:
		if 4*i+4 <= len(e.data) {
			return uint64(order.Uint32(e.data[4*i:]))
		}
	}
	return 0
}

func (e tiffEntry) rational(order binary.ByteOrder, i int) (num, den float64, ok bool) {
	if (e.typ != 5 && e.typ != 10) || 8*i+8 > len(e.data) {
		return 0, 0, false
	}
	if e.typ == 10 {
		return float64(int32(order.Uint32(e.data[8*i:]))), float64(int32(order.Uint32(e.data[8*i+4:]))), true
	}
	return float64(order.Uint32(e.data[8*i:])), float64(order.Uint32(e.data[8*i+4:])), true
}

func (e tiffEntry) str() string {
	if e.typ != 2 {
		return ""
	}
	s := string(e.data)
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// parseTIFF reads the TIFF header and the IFDs we care about.
func parseTIFF(d []byte) *tiffInfo {
	if len(d) < 8 {
		return nil
	}
	var order binary.ByteOrder
	switch string(d[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil
	}
	if order.Uint16(d[2:]) != 42 {
		return nil
	}
	t := &tiffInfo{order: order, ifd0: map[uint16]uint64{}}
	first := order.Uint32(d[4:])
	t.tags, _ = readIFD(d, first, order)
	if t.tags == nil {
		return nil
	}
	for tag, e := range t.tags {
		t.ifd0[tag] = e.uint(order, 0)
	}
	// Count the pages (IFD chain), guarding against loops.
	seen := map[uint32]bool{}
	for off := first; off != 0 && !seen[off] && t.pages < 10000; {
		seen[off] = true
		t.pages++
		_, off = readIFD(d, off, order)
	}
	if e, ok := t.tags[0x8769]; ok {
		t.exif, _ = readIFD(d, uint32(e.uint(order, 0)), order)
	}
	if e, ok := t.tags[0x8825]; ok {
		t.gps, _ = readIFD(d, uint32(e.uint(order, 0)), order)
	}
	return t
}

// exif parses an EXIF TIFF block and adds its tags to the result.
func (p *prober) exif(d []byte) {
	if t := parseTIFF(d); t != nil {
		p.applyEXIF(t)
	}
}

func (p *prober) applyEXIF(t *tiffInfo) {
	o := t.order
	m := p.m
	if e, ok := t.tags[0x0112]; ok {
		if v := int(e.uint(o, 0)); v >= 1 && v <= 8 {
			m.Orientation = v
		}
	}
	str := func(tags map[uint16]tiffEntry, tag uint16) string {
		if e, ok := tags[tag]; ok {
			return e.str()
		}
		return ""
	}
	m.addTag("Camera make", str(t.tags, 0x010F))
	m.addTag("Camera model", str(t.tags, 0x0110))
	m.addTag("Software", str(t.tags, 0x0131))
	m.addTag("Artist", str(t.tags, 0x013B))
	m.addTag("Copyright", str(t.tags, 0x8298))
	m.addTag("Description", str(t.tags, 0x010E))
	if e, ok := t.tags[0x9C9F]; ok && e.typ == 1 { // XPSubject / Windows title
		m.addTag("Title", decodeUTF16Units(e.data))
	}
	date := str(t.exif, 0x9003)
	if date == "" {
		date = str(t.tags, 0x0132)
	}
	m.addTag("Date taken", date)
	if e, ok := t.exif[0x829A]; ok {
		if n, d, ok := e.rational(o, 0); ok && n > 0 && d > 0 {
			if n/d >= 1 {
				m.addTag("Exposure time", trimFloat(n/d)+" s")
			} else {
				m.addTag("Exposure time", fmt.Sprintf("1/%d s", int(math.Round(d/n))))
			}
		}
	}
	if e, ok := t.exif[0x829D]; ok {
		if n, d, ok := e.rational(o, 0); ok && d > 0 {
			m.addTag("F-number", "f/"+trimFloat(n/d))
		}
	}
	if e, ok := t.exif[0x8827]; ok {
		m.addTag("ISO", fmt.Sprint(e.uint(o, 0)))
	}
	if e, ok := t.exif[0x920A]; ok {
		if n, d, ok := e.rational(o, 0); ok && d > 0 {
			m.addTag("Focal length", trimFloat(n/d)+" mm")
		}
	}
	if e, ok := t.exif[0x9209]; ok {
		if e.uint(o, 0)&1 != 0 {
			m.addTag("Flash", "Fired")
		} else {
			m.addTag("Flash", "Did not fire")
		}
	}
	m.addTag("Lens", str(t.exif, 0xA434))
	if m.Width == 0 {
		if e, ok := t.exif[0xA002]; ok {
			m.Width = int(e.uint(o, 0))
		}
		if e, ok := t.exif[0xA003]; ok {
			m.Height = int(e.uint(o, 0))
		}
	}
	if lat, ok := gpsCoord(t.gps, 0x0002, 0x0001, o); ok {
		if lon, ok := gpsCoord(t.gps, 0x0004, 0x0003, o); ok {
			m.addTag("GPS position", fmt.Sprintf("%.6f, %.6f", lat, lon))
		}
	}
	if e, ok := t.gps[0x0006]; ok {
		if n, d, ok := e.rational(o, 0); ok && d > 0 {
			m.addTag("GPS altitude", trimFloat(math.Round(n/d*10)/10)+" m")
		}
	}
}

func gpsCoord(g map[uint16]tiffEntry, valTag, refTag uint16, o binary.ByteOrder) (float64, bool) {
	e, ok := g[valTag]
	if !ok || e.count < 3 {
		return 0, false
	}
	var parts [3]float64
	for i := range parts {
		n, d, ok := e.rational(o, i)
		if !ok || d == 0 {
			return 0, false
		}
		parts[i] = n / d
	}
	v := parts[0] + parts[1]/60 + parts[2]/3600
	if r, ok := g[refTag]; ok {
		if s := r.str(); s == "S" || s == "W" {
			v = -v
		}
	}
	return v, true
}

func trimFloat(f float64) string {
	s := fmt.Sprintf("%.2f", f)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
