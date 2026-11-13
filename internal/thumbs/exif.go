// This file finds the EXIF orientation tag inside an image file. Cameras and
// phones store the sensor data the way the sensor was mounted and record in
// the tag how the picture must be turned to look right; every image viewer,
// Total Commander's thumbnails included, honours it. We only need one 16-bit
// value, so instead of pulling in a full EXIF library we walk the container
// (JPEG APP1 segment, PNG eXIf chunk, WebP EXIF chunk, or the TIFF file
// itself) to the TIFF-structured EXIF block and read tag 0x0112 from IFD0.
// Anything malformed simply yields orientation 1 (upright).

package thumbs

import (
	"bytes"
	"encoding/binary"
)

const tagOrientation = 0x0112

// exifOrientation returns the orientation (1..8) stored in data, which is a
// complete encoded file of the given decoder format.
func exifOrientation(data []byte, format string) int {
	var tiff []byte
	switch format {
	case "jpeg":
		tiff = jpegExif(data)
	case "png":
		tiff = pngExif(data)
	case "webp":
		tiff = webpExif(data)
	case "tiff":
		tiff = data
	}
	if tiff == nil {
		return 1
	}
	return tiffOrientation(tiff)
}

var exifHeader = []byte("Exif\x00\x00")

// jpegExif walks the JPEG marker segments up to the start of scan and
// returns the TIFF block of the first APP1 "Exif" segment.
func jpegExif(b []byte) []byte {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return nil
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return nil
		}
		m := b[i+1]
		switch {
		case m == 0xFF: // fill byte
			i++
			continue
		case m == 0x01 || (m >= 0xD0 && m <= 0xD8): // markers without length
			i += 2
			continue
		case m == 0xDA || m == 0xD9: // start of scan / end of image
			return nil
		}
		l := int(b[i+2])<<8 | int(b[i+3])
		if l < 2 || i+2+l > len(b) {
			return nil
		}
		seg := b[i+4 : i+2+l]
		if m == 0xE1 && bytes.HasPrefix(seg, exifHeader) {
			return seg[len(exifHeader):]
		}
		i += 2 + l
	}
	return nil
}

// pngExif returns the payload of the eXIf chunk, if any.
func pngExif(b []byte) []byte {
	const sig = "\x89PNG\r\n\x1a\n"
	if !bytes.HasPrefix(b, []byte(sig)) {
		return nil
	}
	i := len(sig)
	for i+12 <= len(b) {
		n := int(binary.BigEndian.Uint32(b[i:]))
		typ := string(b[i+4 : i+8])
		if n < 0 || i+12+n > len(b) {
			return nil
		}
		if typ == "eXIf" {
			return trimExifHeader(b[i+8 : i+8+n])
		}
		if typ == "IEND" {
			return nil
		}
		i += 12 + n
	}
	return nil
}

// webpExif returns the payload of the EXIF chunk of an extended (VP8X) WebP.
func webpExif(b []byte) []byte {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return nil
	}
	i := 12
	for i+8 <= len(b) {
		typ := string(b[i : i+4])
		n := int(binary.LittleEndian.Uint32(b[i+4:]))
		if n < 0 || i+8+n > len(b) {
			return nil
		}
		if typ == "EXIF" {
			return trimExifHeader(b[i+8 : i+8+n])
		}
		i += 8 + n + n&1 // chunks are padded to even length
	}
	return nil
}

// trimExifHeader drops the optional "Exif\0\0" prefix some writers put in
// front of the TIFF block even where the container does not require it.
func trimExifHeader(b []byte) []byte {
	return bytes.TrimPrefix(b, exifHeader)
}

// tiffOrientation reads tag 0x0112 from IFD0 of a TIFF-structured block.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[0:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	if bo.Uint16(t[2:]) != 42 {
		return 1
	}
	off := int64(bo.Uint32(t[4:]))
	if off < 8 || off+2 > int64(len(t)) {
		return 1
	}
	n := int64(bo.Uint16(t[off:]))
	for k := int64(0); k < n; k++ {
		e := off + 2 + k*12
		if e+12 > int64(len(t)) {
			return 1
		}
		if bo.Uint16(t[e:]) != tagOrientation {
			continue
		}
		typ := bo.Uint16(t[e+2:])
		var v int
		switch typ {
		case 3: // SHORT
			v = int(bo.Uint16(t[e+8:]))
		case 4: // LONG, seen in a few broken writers
			v = int(bo.Uint32(t[e+8:]))
		default:
			return 1
		}
		if v < 1 || v > 8 {
			return 1
		}
		return v
	}
	return 1
}
