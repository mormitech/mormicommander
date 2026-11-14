// This file finds the EXIF block of image formats beyond JPEG (issue #7), so the Pictures columns and the
// Multi-Rename tool's [=img.exifdate] fields work for them too. Every format keeps the same TIFF-structured block,
// only wrapped differently: a PNG "eXIf" chunk, a WebP "EXIF" chunk, the TIFF file itself (and the camera raw formats
// built on TIFF: DNG, CR2, NEF, ARW, PEF), or an "Exif" item of a HEIF/HEIC/AVIF file, which go4.org/media/heif (already
// in the module graph through the 7-Zip reader) locates. The image size of WebP, TIFF and BMP files comes from the
// golang.org/x/image decoders, which the thumbnails register in the app anyway (registered here too, so this package
// does not depend on that); the size of a HEIF file comes from its "ispe" property.
//
// Everything reads only the start of the file (HEIF: the boxes before the image data). A damaged file gives no date,
// never an error or a crash: the HEIF reader is third-party code fed with any file, so a panic in it is caught.

package listing

import (
	"bytes"
	"encoding/binary"
	"io"

	"go4.org/media/heif"
	_ "golang.org/x/image/bmp" // register decoders for DecodeConfig (image size)
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

var exifHeader = []byte("Exif\x00\x00")

// exifBlock returns the TIFF-structured EXIF block found at the start of an image file, or nil. HEIF files are not
// handled here: their EXIF item can be anywhere in the file (see heifFacts).
func exifBlock(data []byte) []byte {
	switch {
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8:
		return jpegExif(data)
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return pngExif(data)
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return webpExif(data)
	case isTIFF(data):
		return data // the offsets of a TIFF file count from its first byte, as those of an EXIF block do
	}
	return nil
}

// isTIFF says whether data starts with a TIFF header ("II*\0" or "MM\0*").
func isTIFF(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	switch string(data[:2]) {
	case "II":
		return binary.LittleEndian.Uint16(data[2:]) == 42
	case "MM":
		return binary.BigEndian.Uint16(data[2:]) == 42
	}
	return false
}

// pngExif returns the payload of a PNG's eXIf chunk (before the image data, where writers put it), or nil.
func pngExif(b []byte) []byte {
	i := 8
	for i+12 <= len(b) {
		n := int(binary.BigEndian.Uint32(b[i:]))
		typ := string(b[i+4 : i+8])
		if n < 0 || i+12+n > len(b) {
			return nil
		}
		switch typ {
		case "eXIf":
			return bytes.TrimPrefix(b[i+8:i+8+n], exifHeader)
		case "IDAT", "IEND":
			return nil
		}
		i += 12 + n
	}
	return nil
}

// webpExif returns the payload of the EXIF chunk of an extended (VP8X) WebP, or nil.
func webpExif(b []byte) []byte {
	i := 12
	for i+8 <= len(b) {
		typ := string(b[i : i+4])
		n := int(binary.LittleEndian.Uint32(b[i+4:]))
		if n < 0 || i+8+n > len(b) {
			return nil
		}
		if typ == "EXIF" {
			return bytes.TrimPrefix(b[i+8:i+8+n], exifHeader)
		}
		i += 8 + n + n&1 // chunks are padded to an even length
	}
	return nil
}

// isHEIF says whether data starts with the "ftyp" box of a HEIF image (HEIC from phones, AVIF, generic mif1).
func isHEIF(data []byte) bool {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return false
	}
	size := int(binary.BigEndian.Uint32(data))
	if size < 16 || size > len(data) {
		size = min(len(data), 64)
	}
	for i := 8; i+4 <= size; i += 4 {
		if i == 12 {
			continue // (the minor version, not a brand)
		}
		switch string(data[i : i+4]) {
		case "heic", "heix", "heim", "heis", "hevc", "hevx", "mif1", "msf1", "avif", "avis":
			return true
		}
	}
	return false
}

// heifFacts reads the image size and the EXIF block of a HEIF file. A panic in the reader (a damaged file) gives
// nothing.
func heifFacts(r io.ReaderAt) (w, h int, exif []byte) {
	defer func() {
		if recover() != nil {
			w, h, exif = 0, 0, nil
		}
	}()
	f := heif.Open(r)
	if it, err := f.PrimaryItem(); err == nil {
		w, h, _ = it.SpatialExtents()
	}
	if b, err := f.EXIF(); err == nil {
		// The item starts with a 4-byte offset to the TIFF header, which the reader skips; what is left usually begins
		// with "Exif\0\0".
		b = bytes.TrimPrefix(b, exifHeader)
		if !isTIFF(b) {
			if i := bytes.Index(b, []byte("MM\x00*")); i >= 0 {
				b = b[i:]
			} else if i := bytes.Index(b, []byte("II*\x00")); i >= 0 {
				b = b[i:]
			}
		}
		if isTIFF(b) {
			exif = b
		}
	}
	return w, h, exif
}
