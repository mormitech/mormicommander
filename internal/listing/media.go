// This file provides the media fields TC users normally get from content plugins: image width and height, the EXIF
// "date taken" of photos, and ID3 tags of MP3 files (more formats: media_exif.go and media_vorbis.go). They are read lazily, only for rows whose column set actually
// uses one of these fields, and only from the first bytes of the file (image headers, the EXIF block, the ID3v2 header
// or the 128-byte ID3v1 trailer), so a Pictures column set stays fast. Results are cached in Item.Extra, so sorting by
// such a column reads each file once.

package listing

import (
	"bytes"
	"encoding/binary"
	"image"
	_ "image/gif" // register decoders for DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"mormicommander/internal/imgconv"
)

// Keys under which media facts are cached in Item.Extra.
const (
	ExtraImgWidth  = "img.width"
	ExtraImgHeight = "img.height"
	ExtraExifDate  = "img.exifdate"
	ExtraTitle     = "audio.title"
	ExtraArtist    = "audio.artist"
	ExtraAlbum     = "audio.album"
	ExtraYear      = "audio.year"
	ExtraTrack     = "audio.track"
	ExtraGenre     = "audio.genre"
	extraImgDone   = "img.loaded"
	extraAudioDone = "audio.loaded"
)

func itemPath(it *Item) string { return filepath.Join(it.Dir, it.Name) }

func setExtra(it *Item, k string, v any) {
	if it.Extra == nil {
		it.Extra = map[string]any{}
	}
	it.Extra[k] = v
}

// openRegular opens an item's file for reading only when it is a regular file
// (directly or through a symlink). Media fields must never open a named pipe
// or a device: opening a FIFO with no writer blocks forever and would freeze
// the panel. The file is opened non-blocking where the platform supports it
// and checked again after opening, so a file swapped for a FIFO between the
// check and the open cannot hang us either.
func openRegular(it *Item) (*os.File, bool) {
	if cloudSkipped(it) {
		return nil, false
	}
	m := it.Mode
	if m&os.ModeSymlink == 0 && !m.IsRegular() {
		return nil, false
	}
	p := itemPath(it)
	if m&os.ModeSymlink != 0 {
		if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
			return nil, false
		}
	}
	f, err := os.OpenFile(p, os.O_RDONLY|openNonblock, 0)
	if err != nil {
		return nil, false
	}
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		f.Close()
		return nil, false
	}
	return f, true
}

// SkipCloudContent, when set and true, keeps the media fields (image size, EXIF date, audio tags) from reading
// online-only cloud placeholders (AttrCloud), since reading one makes the system download it (#30). The program sets
// it to Options > Operation > "Don't download online-only cloud files…"; nil (the default) reads them as before.
var SkipCloudContent func() bool

func cloudSkipped(it *Item) bool {
	return it.Attr&AttrCloud != 0 && SkipCloudContent != nil && SkipCloudContent()
}

// LoadImageFacts reads an item's image size and EXIF date into Item.Extra (keys ExtraImgWidth, ExtraImgHeight,
// ExtraExifDate), once per item. Other tools, such as the Multi-Rename tool's [=img.*] fields, use it to get the same
// values the file list shows.
func LoadImageFacts(it *Item) { loadImage(it) }

// LoadAudioFacts reads an item's audio tags (ID3, or the Vorbis comments of FLAC and Ogg files) into Item.Extra (keys ExtraTitle ... ExtraGenre), once per item.
func LoadAudioFacts(it *Item) { loadAudio(it) }

// ExternalImageFacts, when set, is asked for an image's size and date when the built-in readers found neither (camera
// raw files and other formats they don't know): the program sets it to exiftool when that is configured
// (Options > Plugins). ok false means it knows nothing either.
var ExternalImageFacts func(path string) (w, h int, date time.Time, ok bool)

// loadImage reads dimensions and EXIF date once per item.
func loadImage(it *Item) {
	done := it.Extra[extraImgDone] != nil
	loadImageOwn(it)
	if done || ExternalImageFacts == nil || it.IsDir() || it.Dir == "" || cloudSkipped(it) {
		return
	}
	_, hasW := it.Extra[ExtraImgWidth]
	_, hasD := it.Extra[ExtraExifDate]
	if hasW && hasD {
		return
	}
	if w, h, t, ok := ExternalImageFacts(itemPath(it)); ok {
		if !hasW && w > 0 && h > 0 {
			setExtra(it, ExtraImgWidth, int64(w))
			setExtra(it, ExtraImgHeight, int64(h))
		}
		if !hasD && !t.IsZero() {
			setExtra(it, ExtraExifDate, t)
		}
	}
}

func loadImageOwn(it *Item) {
	if it.Extra[extraImgDone] != nil || it.IsDir() || it.Dir == "" {
		return
	}
	setExtra(it, extraImgDone, true)
	f, ok := openRegular(it)
	if !ok {
		return
	}
	defer f.Close()
	if cfg, _, err := image.DecodeConfig(f); err == nil {
		setExtra(it, ExtraImgWidth, int64(cfg.Width))
		setExtra(it, ExtraImgHeight, int64(cfg.Height))
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return
	}
	head := make([]byte, 128<<10)
	n, _ := io.ReadFull(f, head)
	if t, ok := ExifDate(head[:n]); ok {
		setExtra(it, ExtraExifDate, t)
		return
	}
	if imgconv.Sniff(head[:n]) == imgconv.JXL {
		// JPEG XL: the size from its header (Go has no JPEG XL decoder)
		size := it.Size
		if fi, err := f.Stat(); err == nil {
			size = fi.Size()
		}
		if w, h, ok := imgconv.JXLSize(f, size); ok {
			setExtra(it, ExtraImgWidth, int64(w))
			setExtra(it, ExtraImgHeight, int64(h))
		}
		return
	}
	if isHEIF(head[:n]) {
		// HEIF/HEIC (media_exif.go): the size and the EXIF item come from the file's boxes, not from a decoder
		w, h, exif := heifFacts(f)
		if _, ok := it.Extra[ExtraImgWidth]; !ok && w > 0 && h > 0 {
			setExtra(it, ExtraImgWidth, int64(w))
			setExtra(it, ExtraImgHeight, int64(h))
		}
		if t, ok := parseTIFFDate(exif); ok {
			setExtra(it, ExtraExifDate, t)
		}
	}
}

// ExifDate finds DateTimeOriginal (or DateTime) in the EXIF block of an image file: JPEG, and since issue #7 also
// PNG, WebP and TIFF (with the camera raw formats built on TIFF, such as DNG, CR2, NEF and ARW; see media_exif.go).
// data is the start of the file. EXIF dates have no time zone, so the result is in time.Local, matching what cameras
// and Explorer show.
func ExifDate(data []byte) (time.Time, bool) {
	if t := exifBlock(data); t != nil {
		return parseTIFFDate(t)
	}
	return time.Time{}, false
}

// jpegExif returns the TIFF block of a JPEG's EXIF segment (APP1 "Exif"), or nil.
func jpegExif(data []byte) []byte {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil
	}
	p := 2
	for p+4 <= len(data) {
		if data[p] != 0xFF {
			return nil
		}
		marker := data[p+1]
		if marker == 0xDA || marker == 0xD9 { // start of scan / end: no EXIF before the image data
			return nil
		}
		segLen := int(binary.BigEndian.Uint16(data[p+2:]))
		if segLen < 2 || p+2+segLen > len(data) {
			return nil
		}
		seg := data[p+4 : p+2+segLen]
		if marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return seg[6:]
		}
		p += 2 + segLen
	}
	return nil
}

func parseTIFFDate(t []byte) (time.Time, bool) {
	if len(t) < 8 {
		return time.Time{}, false
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return time.Time{}, false
	}
	readIFD := func(off uint32) (map[uint16][]byte, uint32) {
		tags := map[uint16][]byte{}
		var exifPtr uint32
		if int(off)+2 > len(t) {
			return tags, 0
		}
		n := int(bo.Uint16(t[off:]))
		for i := 0; i < n; i++ {
			e := int(off) + 2 + i*12
			if e+12 > len(t) {
				break
			}
			tag := bo.Uint16(t[e:])
			typ := bo.Uint16(t[e+2:])
			cnt := bo.Uint32(t[e+4:])
			val := t[e+8 : e+12]
			if tag == 0x8769 { // ExifIFD pointer
				exifPtr = bo.Uint32(val)
				continue
			}
			if typ == 2 && cnt == 20 { // ASCII "YYYY:MM:DD HH:MM:SS\0"
				o := bo.Uint32(val)
				if int(o)+20 <= len(t) {
					tags[tag] = t[o : o+19]
				}
			}
		}
		return tags, exifPtr
	}
	ifd0, exifPtr := readIFD(bo.Uint32(t[4:]))
	var raw []byte
	if exifPtr != 0 {
		ex, _ := readIFD(exifPtr)
		raw = ex[0x9003] // DateTimeOriginal
		if raw == nil {
			raw = ex[0x9004] // DateTimeDigitized
		}
	}
	if raw == nil {
		raw = ifd0[0x0132] // DateTime
	}
	if raw == nil {
		return time.Time{}, false
	}
	tm, err := time.ParseInLocation("2006:01:02 15:04:05", string(raw), time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return tm, true
}

// AudioTags are the ID3 facts shown in audio columns.
type AudioTags struct {
	Title, Artist, Album, Year, Genre string
	Track                             int
}

// ReadID3 reads ID3v2 (2.2, 2.3, 2.4) text frames from the start of r, falling back to the ID3v1 trailer.
func ReadID3(r io.ReadSeeker) (AudioTags, bool) {
	var t AudioTags
	hdr := make([]byte, 10)
	if _, err := io.ReadFull(r, hdr); err == nil && string(hdr[:3]) == "ID3" {
		ver := hdr[3]
		size := syncsafe(hdr[6:10])
		if size > 0 && size < 16<<20 {
			body := make([]byte, size)
			if n, _ := io.ReadFull(r, body); n > 0 {
				parseID3v2(body[:n], ver, hdr[5], &t)
			}
		}
		if t != (AudioTags{}) {
			return t, true
		}
	}
	if _, err := r.Seek(-128, io.SeekEnd); err == nil {
		v1 := make([]byte, 128)
		if _, err := io.ReadFull(r, v1); err == nil && string(v1[:3]) == "TAG" {
			s := func(b []byte) string { return strings.TrimRight(decodeLatin1(b), " \x00") }
			t.Title, t.Artist, t.Album, t.Year = s(v1[3:33]), s(v1[33:63]), s(v1[63:93]), s(v1[93:97])
			if v1[125] == 0 && v1[126] != 0 {
				t.Track = int(v1[126])
			}
			return t, true
		}
	}
	return t, false
}

func syncsafe(b []byte) int {
	return int(b[0]&0x7f)<<21 | int(b[1]&0x7f)<<14 | int(b[2]&0x7f)<<7 | int(b[3]&0x7f)
}

func parseID3v2(b []byte, ver, flags byte, t *AudioTags) {
	p := 0
	if flags&0x40 != 0 && ver >= 3 && len(b) >= 4 { // extended header
		var n int
		if ver == 4 {
			n = syncsafe(b[:4])
		} else {
			n = int(binary.BigEndian.Uint32(b[:4])) + 4
		}
		p = n
	}
	idLen, hdrLen := 4, 10
	if ver == 2 {
		idLen, hdrLen = 3, 6
	}
	for p+hdrLen <= len(b) {
		id := string(b[p : p+idLen])
		if id[0] == 0 {
			break
		}
		var size int
		switch ver {
		case 2:
			size = int(b[p+3])<<16 | int(b[p+4])<<8 | int(b[p+5])
		case 4:
			size = syncsafe(b[p+4 : p+8])
		default:
			size = int(binary.BigEndian.Uint32(b[p+4 : p+8]))
		}
		p += hdrLen
		if size <= 0 || p+size > len(b) {
			break
		}
		data := b[p : p+size]
		p += size
		if !strings.HasPrefix(id, "T") {
			continue
		}
		text := decodeID3Text(data)
		switch id {
		case "TIT2", "TT2":
			t.Title = text
		case "TPE1", "TP1":
			t.Artist = text
		case "TALB", "TAL":
			t.Album = text
		case "TYER", "TDRC", "TYE":
			if len(text) >= 4 {
				text = text[:4]
			}
			t.Year = text
		case "TRCK", "TRK":
			n, _, _ := strings.Cut(text, "/")
			t.Track, _ = strconv.Atoi(strings.TrimSpace(n))
		case "TCON", "TCO":
			t.Genre = text
		}
	}
}

func decodeLatin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// decodeID3Text decodes a text frame: the first byte is the encoding (0 Latin-1, 1 UTF-16 with BOM, 2 UTF-16BE,
// 3 UTF-8). Multiple values separated by NUL keep only the first.
func decodeID3Text(d []byte) string {
	if len(d) == 0 {
		return ""
	}
	enc, d := d[0], d[1:]
	var s string
	switch enc {
	case 1, 2:
		big := enc == 2
		if len(d) >= 2 {
			if d[0] == 0xFF && d[1] == 0xFE {
				big, d = false, d[2:]
			} else if d[0] == 0xFE && d[1] == 0xFF {
				big, d = true, d[2:]
			}
		}
		u := make([]uint16, 0, len(d)/2)
		for i := 0; i+1 < len(d); i += 2 {
			var v uint16
			if big {
				v = binary.BigEndian.Uint16(d[i:])
			} else {
				v = binary.LittleEndian.Uint16(d[i:])
			}
			if v == 0 {
				break
			}
			u = append(u, v)
		}
		s = string(utf16.Decode(u))
	case 3:
		s, _, _ = strings.Cut(string(d), "\x00")
	default:
		s, _, _ = strings.Cut(decodeLatin1(d), "\x00")
	}
	return strings.TrimSpace(s)
}

func loadAudio(it *Item) {
	if it.Extra[extraAudioDone] != nil || it.IsDir() || it.Dir == "" {
		return
	}
	setExtra(it, extraAudioDone, true)
	f, ok := openRegular(it)
	if !ok {
		return
	}
	defer f.Close()
	t, ok := ReadAudioTags(f) // ID3 of MP3 files, Vorbis comments of FLAC and Ogg files (media_vorbis.go)
	if !ok {
		return
	}
	setExtra(it, ExtraTitle, t.Title)
	setExtra(it, ExtraArtist, t.Artist)
	setExtra(it, ExtraAlbum, t.Album)
	setExtra(it, ExtraYear, t.Year)
	setExtra(it, ExtraGenre, t.Genre)
	if t.Track > 0 {
		setExtra(it, ExtraTrack, int64(t.Track))
	}
}

func init() {
	img := func(name, key string, kind Kind) {
		RegisterField("img", Field{Name: name, Kind: kind, Get: func(it *Item, unit string, f *Format) Value {
			loadImage(it)
			return extraValue(it.Extra[key], kind, unit, f)
		}})
	}
	img("width", ExtraImgWidth, KindNumber)
	img("height", ExtraImgHeight, KindNumber)
	img("exifdate", ExtraExifDate, KindTime)
	RegisterField("img", Field{Name: "size", Kind: KindNumber, Help: "width x height; sorts by pixel count",
		Get: func(it *Item, _ string, _ *Format) Value {
			loadImage(it)
			w, ok1 := it.Extra[ExtraImgWidth].(int64)
			h, ok2 := it.Extra[ExtraImgHeight].(int64)
			if !ok1 || !ok2 {
				return Value{Kind: KindNumber, Empty: true}
			}
			return NumValue(w*h, strconv.FormatInt(w, 10)+"x"+strconv.FormatInt(h, 10))
		}})
	audio := func(name, key string, kind Kind) {
		RegisterField("audio", Field{Name: name, Kind: kind, Get: func(it *Item, unit string, f *Format) Value {
			loadAudio(it)
			return extraValue(it.Extra[key], kind, unit, f)
		}})
	}
	audio("title", ExtraTitle, KindText)
	audio("artist", ExtraArtist, KindText)
	audio("album", ExtraAlbum, KindText)
	audio("year", ExtraYear, KindText)
	audio("track", ExtraTrack, KindNumber)
	audio("genre", ExtraGenre, KindText)
}
