// Tests for media probing. Every fixture is generated in the test (with the
// standard library encoders where Go has one, otherwise byte by byte from
// the format specifications), so no binary test files are needed.

package lister

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func probeBytes(t *testing.T, b []byte) *MediaInfo {
	t.Helper()
	m, err := ProbeMedia(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// --- builders ------------------------------------------------------------

type tEntry struct {
	tag, typ uint16
	count    uint32
	val      []byte
}

func ascii(tag uint16, s string) tEntry {
	return tEntry{tag, 2, uint32(len(s) + 1), append([]byte(s), 0)}
}
func short(tag uint16, v uint16) tEntry {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	return tEntry{tag, 3, 1, b}
}
func long(tag uint16, v uint32) tEntry {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return tEntry{tag, 4, 1, b}
}
func rationals(tag uint16, v ...uint32) tEntry {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], x)
	}
	return tEntry{tag, 5, uint32(len(v) / 2), b}
}

// buildIFD lays out one little-endian IFD at absolute offset base.
func buildIFD(base int, entries []tEntry, next uint32) []byte {
	head := 2 + 12*len(entries) + 4
	var ifd, data []byte
	ifd = binary.LittleEndian.AppendUint16(ifd, uint16(len(entries)))
	for _, e := range entries {
		ifd = binary.LittleEndian.AppendUint16(ifd, e.tag)
		ifd = binary.LittleEndian.AppendUint16(ifd, e.typ)
		ifd = binary.LittleEndian.AppendUint32(ifd, e.count)
		if len(e.val) <= 4 {
			v := make([]byte, 4)
			copy(v, e.val)
			ifd = append(ifd, v...)
		} else {
			ifd = binary.LittleEndian.AppendUint32(ifd, uint32(base+head+len(data)))
			data = append(data, e.val...)
			if len(data)%2 == 1 {
				data = append(data, 0)
			}
		}
	}
	ifd = binary.LittleEndian.AppendUint32(ifd, next)
	return append(ifd, data...)
}

// buildEXIF makes a TIFF block with IFD0 (+ extra entries), an EXIF IFD
// and a GPS IFD, like a phone photo carries.
func buildEXIF(extra []tEntry, secondPage bool) []byte {
	exifEntries := []tEntry{
		rationals(0x829A, 1, 250),
		rationals(0x829D, 28, 10),
		short(0x8827, 400),
		ascii(0x9003, "2024:05:01 12:34:56"),
		rationals(0x920A, 50, 1),
		ascii(0xA434, "Test Lens 50mm"),
	}
	gpsEntries := []tEntry{
		ascii(0x0001, "N"), rationals(0x0002, 47, 1, 30, 1, 0, 1),
		ascii(0x0003, "W"), rationals(0x0004, 19, 1, 3, 1, 36, 1),
	}
	ifd0 := func(exifOff, gpsOff, next uint32) []tEntry {
		e := append([]tEntry{}, extra...)
		e = append(e, ascii(0x010F, "TestMake"), ascii(0x0110, "TestCam 1"), short(0x0112, 6), long(0x8769, exifOff), long(0x8825, gpsOff))
		return e
	}
	tmp := buildIFD(8, ifd0(0, 0, 0), 0)
	exifAt := 8 + len(tmp)
	exif := buildIFD(exifAt, exifEntries, 0)
	gpsAt := exifAt + len(exif)
	gps := buildIFD(gpsAt, gpsEntries, 0)
	page2At := gpsAt + len(gps)
	next := uint32(0)
	if secondPage {
		next = uint32(page2At)
	}
	out := []byte("II*\x00\x08\x00\x00\x00")
	out = append(out, buildIFD(8, ifd0(uint32(exifAt), uint32(gpsAt), next), next)...)
	out = append(out, exif...)
	out = append(out, gps...)
	if secondPage {
		out = append(out, buildIFD(page2At, []tEntry{long(0x0100, 10), long(0x0101, 10)}, 0)...)
	}
	return out
}

func checkEXIF(t *testing.T, m *MediaInfo) {
	t.Helper()
	want := map[string]string{
		"Camera make": "TestMake", "Camera model": "TestCam 1", "Exposure time": "1/250 s",
		"F-number": "f/2.8", "ISO": "400", "Date taken": "2024:05:01 12:34:56",
		"Focal length": "50 mm", "Lens": "Test Lens 50mm", "GPS position": "47.500000, -19.060000",
	}
	for k, v := range want {
		if got := m.Tag(k); got != v {
			t.Errorf("tag %q = %q want %q", k, got, v)
		}
	}
	if m.Orientation != 6 {
		t.Errorf("orientation %d", m.Orientation)
	}
}

func box(typ string, body ...[]byte) []byte {
	b := bytes.Join(body, nil)
	out := binary.BigEndian.AppendUint32(nil, uint32(8+len(b)))
	out = append(out, typ...)
	return append(out, b...)
}

func be32b(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
func le32b(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
func le16b(v uint16) []byte { return binary.LittleEndian.AppendUint16(nil, v) }

func ebml(id []byte, body ...[]byte) []byte {
	b := bytes.Join(body, nil)
	out := append([]byte{}, id...)
	if len(b) < 127 {
		out = append(out, 0x80|byte(len(b)))
	} else {
		out = append(out, 0x40|byte(len(b)>>8), byte(len(b)))
	}
	return append(out, b...)
}

func oggPage(granule uint64, packet []byte) []byte {
	p := []byte("OggS\x00\x02")
	p = binary.LittleEndian.AppendUint64(p, granule)
	p = append(p, make([]byte, 12)...) // serial, sequence, crc
	var segs []byte
	n := len(packet)
	for n >= 255 {
		segs = append(segs, 255)
		n -= 255
	}
	segs = append(segs, byte(n))
	p = append(p, byte(len(segs)))
	p = append(p, segs...)
	return append(p, packet...)
}

// --- tests ----------------------------------------------------------------

func TestProbeImagesStdlib(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 37, 21))
	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, img)
	m := probeBytes(t, pngBuf.Bytes())
	if m.Format != "PNG" || m.Width != 37 || m.Height != 21 || m.Kind != "image" || m.MIME != "image/png" || m.Frames != 1 {
		t.Fatalf("png %+v", m)
	}

	var jpgBuf bytes.Buffer
	jpeg.Encode(&jpgBuf, image.NewGray(image.Rect(0, 0, 40, 30)), nil)
	m = probeBytes(t, jpgBuf.Bytes())
	if m.Format != "JPEG" || m.Width != 40 || m.Height != 30 || m.BitDepth != 8 {
		t.Fatalf("jpeg %+v", m)
	}

	// JPEG with an EXIF APP1 segment inserted after SOI.
	exif := append([]byte("Exif\x00\x00"), buildEXIF(nil, false)...)
	app1 := append([]byte{0xFF, 0xE1}, byte((len(exif)+2)>>8), byte(len(exif)+2))
	app1 = append(app1, exif...)
	withExif := append([]byte{0xFF, 0xD8}, app1...)
	withExif = append(withExif, jpgBuf.Bytes()[2:]...)
	m = probeBytes(t, withExif)
	if m.Width != 40 || m.Height != 30 {
		t.Fatalf("jpeg+exif %+v", m)
	}
	checkEXIF(t, m)

	pal := image.NewPaletted(image.Rect(0, 0, 16, 9), palette.Plan9)
	anim := &gif.GIF{Image: []*image.Paletted{pal, pal, pal}, Delay: []int{10, 10, 10}}
	var gifBuf bytes.Buffer
	gif.EncodeAll(&gifBuf, anim)
	m = probeBytes(t, gifBuf.Bytes())
	if m.Format != "GIF" || m.Width != 16 || m.Height != 9 || m.Frames != 3 {
		t.Fatalf("gif %+v", m)
	}
	_ = color.Black
}

func TestProbeImagesHandmade(t *testing.T) {
	bmp := append([]byte("BM"), le32b(70)...)
	bmp = append(bmp, 0, 0, 0, 0)
	bmp = append(bmp, le32b(54)...)
	bmp = append(bmp, le32b(40)...)
	bmp = append(bmp, le32b(10)...)
	bmp = append(bmp, le32b(uint32(0xFFFFFFFB))...) // height -5 (top-down)
	bmp = append(bmp, le16b(1)...)
	bmp = append(bmp, le16b(24)...)
	bmp = append(bmp, make([]byte, 24)...)

	vp8x := append([]byte("VP8X"), le32b(10)...)
	vp8x = append(vp8x, 0x02, 0, 0, 0, 0x3F, 0x01, 0, 0xC7, 0, 0) // 320x200, animated
	anmf := append([]byte("ANMF"), le32b(16)...)
	anmf = append(anmf, make([]byte, 16)...)
	webpBody := append([]byte("WEBP"), vp8x...)
	webpBody = append(webpBody, anmf...)
	webpBody = append(webpBody, anmf...)
	webpAnim := append(append([]byte("RIFF"), le32b(uint32(len(webpBody)))...), webpBody...)

	vp8l := append([]byte("VP8L"), le32b(5)...)
	bits := uint32(99) | uint32(49)<<14 // 100 x 50
	vp8l = append(vp8l, 0x2F)
	vp8l = append(vp8l, le32b(bits)...)
	vp8l = append(vp8l, 0)
	webpLL := append(append([]byte("RIFF"), le32b(uint32(4+len(vp8l)))...), append([]byte("WEBP"), vp8l...)...)

	vp8 := append([]byte("VP8 "), le32b(10)...)
	vp8 = append(vp8, 0, 0, 0, 0x9D, 0x01, 0x2A)
	vp8 = append(vp8, le16b(64)...)
	vp8 = append(vp8, le16b(48)...)
	webpLossy := append(append([]byte("RIFF"), le32b(uint32(4+len(vp8)))...), append([]byte("WEBP"), vp8...)...)

	tiff := buildEXIF([]tEntry{long(0x0100, 640), short(0x0101, 480), short(0x0102, 8), short(0x0115, 3)}, true)

	ico := []byte{0, 0, 1, 0, 2, 0}
	ico = append(ico, 16, 16, 0, 0, 1, 0, 32, 0)
	ico = append(ico, make([]byte, 8)...)
	ico = append(ico, 0, 0, 0, 0, 1, 0, 32, 0)
	ico = append(ico, make([]byte, 8)...)

	psd := append([]byte("8BPS\x00\x01"), make([]byte, 6)...)
	psd = binary.BigEndian.AppendUint16(psd, 3)
	psd = append(psd, be32b(100)...)
	psd = append(psd, be32b(200)...)
	psd = binary.BigEndian.AppendUint16(psd, 8)
	psd = binary.BigEndian.AppendUint16(psd, 3)

	heic := box("ftyp", []byte("heic"), be32b(0), []byte("mif1heic"))
	ispe := box("ispe", be32b(0), be32b(4032), be32b(3024))
	heic = append(heic, box("meta", be32b(0), box("hdlr", be32b(0), be32b(0), []byte("pict"), make([]byte, 13)), box("iprp", box("ipco", ispe)))...)

	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="120px" height="80"><rect/></svg>`)
	svgVB := []byte(`<svg viewBox="0 0 300.5 150"></svg>`)

	tests := []struct {
		name         string
		data         []byte
		format       string
		w, h, frames int
		depth        int
	}{
		{"bmp", bmp, "BMP", 10, 5, 1, 24},
		{"webp animated", webpAnim, "WebP", 320, 200, 2, 0},
		{"webp lossless", webpLL, "WebP", 100, 50, 1, 0},
		{"webp lossy", webpLossy, "WebP", 64, 48, 1, 0},
		{"tiff", tiff, "TIFF", 640, 480, 2, 24},
		{"ico", ico, "ICO", 256, 256, 2, 32},
		{"psd", psd, "PSD", 200, 100, 1, 24},
		{"heic", heic, "HEIC", 4032, 3024, 1, 0},
		{"svg", svg, "SVG", 120, 80, 1, 0},
		{"svg viewBox", svgVB, "SVG", 301, 150, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := probeBytes(t, tt.data)
			if m.Format != tt.format || m.Width != tt.w || m.Height != tt.h || m.Frames != tt.frames || m.Kind != "image" {
				t.Fatalf("got %+v", m)
			}
			if tt.depth != 0 && m.BitDepth != tt.depth {
				t.Fatalf("depth %d want %d", m.BitDepth, tt.depth)
			}
			if tt.name == "tiff" {
				checkEXIF(t, m)
			}
		})
	}
}

func TestProbeAudio(t *testing.T) {
	// WAV: 2 s of 16-bit stereo silence.
	wav := []byte("RIFF\x00\x00\x00\x00WAVEfmt ")
	wav = append(wav, le32b(16)...)
	wav = append(wav, le16b(1)...)
	wav = append(wav, le16b(2)...)
	wav = append(wav, le32b(44100)...)
	wav = append(wav, le32b(176400)...)
	wav = append(wav, le16b(4)...)
	wav = append(wav, le16b(16)...)
	wav = append(wav, []byte("data")...)
	wav = append(wav, le32b(352800)...)
	wav = append(wav, make([]byte, 352800)...)

	// FLAC: STREAMINFO for 3 s at 48 kHz, 24-bit, 6 channels + comments.
	si := make([]byte, 34)
	x := uint64(48000)<<44 | uint64(5)<<41 | uint64(23)<<36 | uint64(3*48000)
	binary.BigEndian.PutUint64(si[10:], x)
	flac := append([]byte("fLaC\x00\x00\x00\x22"), si...)
	vc := append(le32b(4), "test"...)
	vc = append(vc, le32b(2)...)
	vc = append(vc, le32b(11)...)
	vc = append(vc, "TITLE=Song1"...)
	vc = append(vc, le32b(12)...)
	vc = append(vc, "artist=Band2"...)
	flac = append(flac, 0x84, 0, 0, byte(len(vc)))
	flac = append(flac, vc...)

	// MP3: ID3v2.3 tag + 100 CBR frames of 128 kbit/s at 44.1 kHz.
	frame := func(id, text string) []byte {
		f := append([]byte(id), be32b(uint32(len(text)+1))...)
		f = append(f, 0, 0, 3)
		return append(f, text...)
	}
	tags := append(frame("TIT2", "Tune"), frame("TPE1", "Singer")...)
	mp3 := []byte{'I', 'D', '3', 3, 0, 0, 0, 0, byte(len(tags) >> 7), byte(len(tags) & 0x7F)}
	mp3 = append(mp3, tags...)
	for i := 0; i < 100; i++ {
		f := make([]byte, 417)
		copy(f, []byte{0xFF, 0xFB, 0x90, 0x00})
		mp3 = append(mp3, f...)
	}

	// Ogg Vorbis, 10 s at 44.1 kHz.
	id := []byte("\x01vorbis\x00\x00\x00\x00\x02")
	id = append(id, le32b(44100)...)
	id = append(id, le32b(0)...)
	id = append(id, le32b(128000)...)
	id = append(id, le32b(0)...)
	id = append(id, 0xB8, 1)
	cm := append([]byte("\x03vorbis"), le32b(1)...)
	cm = append(cm, 'v')
	cm = append(cm, le32b(1)...)
	cm = append(cm, le32b(8)...)
	cm = append(cm, "TITLE=Hi"...)
	ogg := append(oggPage(0, id), oggPage(0, cm)...)
	ogg = append(ogg, oggPage(441000, make([]byte, 100))...)

	// M4A: audio-only MP4.
	mvhd := box("mvhd", be32b(0), be32b(0), be32b(0), be32b(600), be32b(600*7), make([]byte, 80))
	hdlr := box("hdlr", be32b(0), be32b(0), []byte("soun"), make([]byte, 13))
	m4a := append(box("ftyp", []byte("M4A "), be32b(0)), box("moov", mvhd, box("trak", box("mdia", hdlr)))...)

	tests := []struct {
		name     string
		data     []byte
		format   string
		rate, ch int
		dur      time.Duration
		tag      [2]string
	}{
		{"wav", wav, "WAV", 44100, 2, 2 * time.Second, [2]string{}},
		{"flac", flac, "FLAC", 48000, 6, 3 * time.Second, [2]string{"Artist", "Band2"}},
		{"mp3", mp3, "MP3", 44100, 2, time.Duration(float64(41700*8) / 128000 * float64(time.Second)), [2]string{"Title", "Tune"}},
		{"ogg", ogg, "Ogg Vorbis", 44100, 2, 10 * time.Second, [2]string{"Title", "Hi"}},
		{"m4a", m4a, "M4A", 0, 0, 7 * time.Second, [2]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := probeBytes(t, tt.data)
			if m.Format != tt.format || m.Kind != "audio" || m.SampleRate != tt.rate || m.Channels != tt.ch {
				t.Fatalf("got %+v", m)
			}
			if d := m.Duration - tt.dur; d > 5*time.Millisecond || d < -5*time.Millisecond {
				t.Fatalf("duration %v want %v", m.Duration, tt.dur)
			}
			if tt.tag[0] != "" && m.Tag(tt.tag[0]) != tt.tag[1] {
				t.Fatalf("tags %v", m.Tags)
			}
		})
	}
}

func TestProbeVideoAndDocs(t *testing.T) {
	mvhd := box("mvhd", be32b(0), be32b(0), be32b(0), be32b(1000), be32b(5000), make([]byte, 80))
	tkhdBody := make([]byte, 84)
	binary.BigEndian.PutUint32(tkhdBody[76:], 1920<<16)
	binary.BigEndian.PutUint32(tkhdBody[80:], 1080<<16)
	trak := box("trak", box("tkhd", tkhdBody), box("mdia", box("hdlr", be32b(0), be32b(0), []byte("vide"), make([]byte, 13))))
	mp4 := append(box("ftyp", []byte("isom"), be32b(0x200), []byte("isomiso2mp41")), box("moov", mvhd, trak)...)
	mov := append(box("ftyp", []byte("qt  "), be32b(0)), box("moov", mvhd, trak)...)

	avih := append([]byte("avih"), le32b(56)...)
	for _, v := range []uint32{40000, 0, 0, 0, 250, 0, 1, 0, 320, 240, 0, 0, 0, 0} {
		avih = append(avih, le32b(v)...)
	}
	hdrl := append(append([]byte("LIST"), le32b(uint32(4+len(avih)))...), append([]byte("hdrl"), avih...)...)
	avi := append(append([]byte("RIFF"), le32b(uint32(4+len(hdrl)))...), append([]byte("AVI "), hdrl...)...)

	dur := make([]byte, 8)
	binary.BigEndian.PutUint64(dur, math.Float64bits(3000))
	header := ebml([]byte{0x1A, 0x45, 0xDF, 0xA3}, ebml([]byte{0x42, 0x82}, []byte("webm")))
	info := ebml([]byte{0x15, 0x49, 0xA9, 0x66}, ebml([]byte{0x2A, 0xD7, 0xB1}, []byte{0x0F, 0x42, 0x40}), ebml([]byte{0x44, 0x89}, dur))
	tracks := ebml([]byte{0x16, 0x54, 0xAE, 0x6B}, ebml([]byte{0xAE}, ebml([]byte{0xE0}, ebml([]byte{0xB0}, []byte{0x05, 0x00}), ebml([]byte{0xBA}, []byte{0x02, 0xD0}))))
	segment := append([]byte{0x18, 0x53, 0x80, 0x67, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, append(info, tracks...)...)
	webm := append(header, segment...)

	pdf := []byte("%PDF-1.7\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n2 0 obj << /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >> endobj\n3 0 obj << /Type /Page >> endobj\n6 0 obj << /Title (Hello PDF) >> endobj\n")

	tests := []struct {
		name, format, kind string
		data               []byte
		w, h, frames       int
		dur                time.Duration
	}{
		{"mp4", "MP4", "video", mp4, 1920, 1080, 0, 5 * time.Second},
		{"mov", "QuickTime", "video", mov, 1920, 1080, 0, 5 * time.Second},
		{"avi", "AVI", "video", avi, 320, 240, 250, 10 * time.Second},
		{"webm", "WebM", "video", webm, 1280, 720, 0, 3 * time.Second},
		{"pdf", "PDF", "document", pdf, 0, 0, 3, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := probeBytes(t, tt.data)
			if m.Format != tt.format || m.Kind != tt.kind || m.Width != tt.w || m.Height != tt.h || m.Frames != tt.frames || m.Duration != tt.dur {
				t.Fatalf("got %+v", m)
			}
		})
	}
	m := probeBytes(t, pdf)
	if m.Tag("PDF version") != "1.7" || m.Tag("Title") != "Hello PDF" {
		t.Fatalf("pdf tags %v", m.Tags)
	}
}

func TestProbeUnknownAndTruncated(t *testing.T) {
	if _, err := ProbeMedia(bytes.NewReader([]byte("just text")), 9); err != ErrUnknownMedia {
		t.Fatalf("got %v", err)
	}
	// Truncated headers must not panic and still report the format.
	for _, b := range [][]byte{
		[]byte("\x89PNG\r\n\x1a\n"), {0xFF, 0xD8, 0xFF}, []byte("GIF89a"), []byte("RIFF\x00\x00\x00\x00WEBPVP8X"),
		[]byte("II*\x00\xFF\xFF\xFF\xFF"), []byte("fLaC"), []byte("OggS"), []byte("ID3\x03\x00\x00\x7f\x7f\x7f\x7f"),
		{0x1A, 0x45, 0xDF, 0xA3, 0xFF}, []byte("\x00\x00\x00\x18ftypisom"), []byte("RIFF\x00\x00\x00\x00AVI LIST"),
		[]byte("RIFF\x00\x00\x00\x00WAVEfmt "), []byte("8BPS"), []byte("%PDF-"),
	} {
		if _, err := ProbeMedia(bytes.NewReader(b), int64(len(b))); err != nil {
			t.Fatalf("%q: %v", b, err)
		}
	}
}

func TestProbeMediaFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.png")
	var buf bytes.Buffer
	png.Encode(&buf, image.NewGray(image.Rect(0, 0, 3, 4)))
	os.WriteFile(path, buf.Bytes(), 0o644)
	m, err := ProbeMediaFile(path)
	if err != nil || m.Width != 3 || m.Height != 4 {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err := ProbeMediaFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing file")
	}
}
