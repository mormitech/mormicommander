// Image and media information for the Lister. Key 4 (multimedia mode)
// shows images, audio and video through the web view, and the Lister's
// "Image info"/EXIF command and status bar show what the file is: format,
// pixel size, colour depth, frame count, play time, sample rate and the
// camera's EXIF data. Reading that information must not decode the whole
// file (a 200 MB video or a 100-megapixel photo), so each format is
// recognised by its magic bytes and only its header structures are read:
// the PNG IHDR chunk, JPEG SOF and APP1 segments, the WebP/AVI/WAV RIFF
// chunks, ISO media boxes (MP4, MOV, HEIC, AVIF), Matroska EBML elements,
// FLAC STREAMINFO, MP3 ID3 tags and frame headers, Ogg pages and so on.

package lister

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// MediaInfo describes an image, audio, video or document file.
type MediaInfo struct {
	Kind       string        // "image", "audio", "video" or "document"
	Format     string        // short name: "PNG", "JPEG", "MP4", ...
	MIME       string        // MIME type for the web view
	Width      int           // pixels (images, video)
	Height     int           // pixels
	BitDepth   int           // bits per sample or per pixel, when known
	Frames     int           // images in the file: animation frames, icon sizes, PDF pages
	Duration   time.Duration // play time (audio, video)
	SampleRate int           // Hz (audio)
	Channels   int           // audio channels
	Bitrate    int           // bits per second, when known
	// Orientation is the EXIF orientation (1-8, 0 if absent); 6 and 8 mean
	// the image must be turned 90 degrees to show upright.
	Orientation int
	Tags        []Tag // EXIF, ID3 and similar metadata, in file order
}

// Tag is one metadata item, e.g. {"Camera model", "Canon EOS 5D"}.
type Tag struct {
	Name, Value string
}

// Tag returns the value of the first tag with the given name.
func (m *MediaInfo) Tag(name string) string {
	for _, t := range m.Tags {
		if t.Name == name {
			return t.Value
		}
	}
	return ""
}

func (m *MediaInfo) addTag(name, value string) {
	value = strings.TrimSpace(strings.TrimRight(value, "\x00"))
	if value != "" {
		m.Tags = append(m.Tags, Tag{name, value})
	}
}

// ErrUnknownMedia is returned for data that is no recognised media format.
var ErrUnknownMedia = errors.New("lister: not a recognised image or media file")

type mediaFormat struct {
	name, mime, kind string
}

// sniffMedia recognises a media format from the first bytes of a file.
func sniffMedia(b []byte) (mediaFormat, bool) {
	has := func(off int, s string) bool {
		return len(b) >= off+len(s) && string(b[off:off+len(s)]) == s
	}
	switch {
	case has(0, "\x89PNG\r\n\x1a\n"):
		return mediaFormat{"PNG", "image/png", "image"}, true
	case has(0, "\xff\xd8\xff"):
		return mediaFormat{"JPEG", "image/jpeg", "image"}, true
	case has(0, "GIF87a"), has(0, "GIF89a"):
		return mediaFormat{"GIF", "image/gif", "image"}, true
	case has(0, "BM") && len(b) >= 26 && binary.LittleEndian.Uint32(b[14:]) >= 12 && binary.LittleEndian.Uint32(b[14:]) <= 124:
		return mediaFormat{"BMP", "image/bmp", "image"}, true
	case has(0, "RIFF") && has(8, "WEBP"):
		return mediaFormat{"WebP", "image/webp", "image"}, true
	case has(0, "II*\x00"), has(0, "MM\x00*"):
		return mediaFormat{"TIFF", "image/tiff", "image"}, true
	case has(0, "\x00\x00\x01\x00") && len(b) >= 6 && b[4]|b[5] != 0:
		return mediaFormat{"ICO", "image/x-icon", "image"}, true
	case has(0, "\x00\x00\x02\x00") && len(b) >= 6 && b[4]|b[5] != 0:
		return mediaFormat{"CUR", "image/x-icon", "image"}, true
	case has(0, "8BPS"):
		return mediaFormat{"PSD", "image/vnd.adobe.photoshop", "image"}, true
	case has(0, "%PDF-"):
		return mediaFormat{"PDF", "application/pdf", "document"}, true
	case has(0, "RIFF") && has(8, "WAVE"):
		return mediaFormat{"WAV", "audio/wav", "audio"}, true
	case has(0, "RIFF") && has(8, "AVI "):
		return mediaFormat{"AVI", "video/x-msvideo", "video"}, true
	case has(0, "fLaC"):
		return mediaFormat{"FLAC", "audio/flac", "audio"}, true
	case has(0, "OggS"):
		return mediaFormat{"Ogg", "audio/ogg", "audio"}, true
	case has(0, "ID3"), len(b) >= 2 && b[0] == 0xFF && b[1]&0xE6 == 0xE2 && b[1]&0x18 != 0x08:
		return mediaFormat{"MP3", "audio/mpeg", "audio"}, true
	case has(0, "\x1a\x45\xdf\xa3"):
		if bytes.Contains(b[:min(len(b), 64)], []byte("webm")) {
			return mediaFormat{"WebM", "video/webm", "video"}, true
		}
		return mediaFormat{"Matroska", "video/x-matroska", "video"}, true
	case has(4, "ftyp"):
		brand := ""
		if len(b) >= 12 {
			brand = string(b[8:12])
		}
		switch brand {
		case "heic", "heix", "heim", "heis", "hevc", "hevx", "mif1", "msf1":
			return mediaFormat{"HEIC", "image/heic", "image"}, true
		case "avif", "avis":
			return mediaFormat{"AVIF", "image/avif", "image"}, true
		case "qt  ":
			return mediaFormat{"QuickTime", "video/quicktime", "video"}, true
		case "M4A ", "M4B ", "M4P ":
			return mediaFormat{"M4A", "audio/mp4", "audio"}, true
		}
		return mediaFormat{"MP4", "video/mp4", "video"}, true
	case has(4, "moov"), has(4, "mdat"), has(4, "wide"), has(4, "free"):
		return mediaFormat{"QuickTime", "video/quicktime", "video"}, true
	}
	head := bytes.ToLower(b[:min(len(b), 1024)])
	if i := bytes.Index(head, []byte("<svg")); i >= 0 && !LooksBinary(head) {
		return mediaFormat{"SVG", "image/svg+xml", "image"}, true
	}
	return mediaFormat{}, false
}

// ProbeMediaFile is ProbeMedia for a file on disk.
func ProbeMediaFile(path string) (*MediaInfo, error) {
	f, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ProbeMedia(f, f.Size())
}

// ProbeMedia identifies the media format of r (size bytes long) and reads
// its header information. It returns ErrUnknownMedia for other data. A
// damaged file still returns whatever could be read before the damage.
func ProbeMedia(r io.ReaderAt, size int64) (*MediaInfo, error) {
	head := make([]byte, min(size, SampleSize))
	n, err := r.ReadAt(head, 0)
	if n == 0 && err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	head = head[:n]
	f, ok := sniffMedia(head)
	if !ok {
		return nil, ErrUnknownMedia
	}
	m := &MediaInfo{Kind: f.kind, Format: f.name, MIME: f.mime}
	p := &prober{r: r, size: size, head: head, m: m}
	switch f.name {
	case "PNG":
		p.png()
	case "JPEG":
		p.jpeg()
	case "GIF":
		p.gif()
	case "BMP":
		p.bmp()
	case "WebP":
		p.webp()
	case "TIFF":
		p.tiff()
	case "ICO", "CUR":
		p.ico()
	case "PSD":
		p.psd()
	case "PDF":
		p.pdf()
	case "WAV":
		p.wav()
	case "AVI":
		p.avi()
	case "FLAC":
		p.flac()
	case "Ogg":
		p.ogg()
	case "MP3":
		p.mp3()
	case "WebM", "Matroska":
		p.mkv()
	case "HEIC", "AVIF", "MP4", "QuickTime", "M4A":
		p.isobmff()
	case "SVG":
		p.svg()
	}
	return m, nil
}

type prober struct {
	r    io.ReaderAt
	size int64
	head []byte
	m    *MediaInfo
}

// read returns n bytes at off, or fewer at the end of the data.
func (p *prober) read(off int64, n int) []byte {
	if off < 0 || off >= p.size || n <= 0 {
		return nil
	}
	if off+int64(n) <= int64(len(p.head)) {
		return p.head[off : off+int64(n)]
	}
	n = int(min(int64(n), p.size-off))
	b := make([]byte, n)
	m, _ := p.r.ReadAt(b, off)
	return b[:m]
}

var (
	be16 = binary.BigEndian.Uint16
	be32 = binary.BigEndian.Uint32
	be64 = binary.BigEndian.Uint64
	le16 = binary.LittleEndian.Uint16
	le32 = binary.LittleEndian.Uint32
	le64 = binary.LittleEndian.Uint64
)

// --- images ----------------------------------------------------------------

func (p *prober) png() {
	h := p.read(8, 25)
	if len(h) < 25 || string(h[4:8]) != "IHDR" {
		return
	}
	p.m.Width, p.m.Height = int(be32(h[8:])), int(be32(h[12:]))
	depth, ctype := int(h[16]), h[17]
	channels := map[byte]int{0: 1, 2: 3, 3: 1, 4: 2, 6: 4}[ctype]
	p.m.BitDepth = depth * max(channels, 1)
	p.m.addTag("Colour type", map[byte]string{0: "Greyscale", 2: "RGB", 3: "Palette", 4: "Greyscale + alpha", 6: "RGBA"}[ctype])
	if h[24] == 1 {
		p.m.addTag("Interlaced", "Yes")
	}
	p.m.Frames = 1
	// Walk the chunks before the image data for APNG frames and text.
	off := int64(8)
	for i := 0; i < 1000 && off+12 <= p.size; i++ {
		c := p.read(off, 8)
		if len(c) < 8 {
			return
		}
		ln := int64(be32(c))
		typ := string(c[4:8])
		switch typ {
		case "acTL":
			if d := p.read(off+8, 8); len(d) == 8 {
				p.m.Frames = int(be32(d))
				p.m.Format = "APNG"
				p.m.MIME = "image/apng"
			}
		case "tEXt", "iTXt":
			if ln < 4096 {
				d := p.read(off+8, int(ln))
				if k := bytes.IndexByte(d, 0); k > 0 {
					val := d[k+1:]
					if typ == "iTXt" {
						// compression flag, method, language\0, translated keyword\0
						if len(val) >= 2 && val[0] == 0 {
							val = val[2:]
							for skip := 0; skip < 2; skip++ {
								if z := bytes.IndexByte(val, 0); z >= 0 {
									val = val[z+1:]
								}
							}
						} else {
							val = nil
						}
					}
					p.m.addTag(string(d[:k]), string(val))
				}
			}
		case "eXIf":
			if ln < 1<<20 {
				p.exif(p.read(off+8, int(ln)))
			}
		case "IDAT", "IEND":
			return
		}
		off += 12 + ln
	}
}

func (p *prober) jpeg() {
	off := int64(2)
	p.m.Frames = 1
	for i := 0; i < 10000 && off+4 <= p.size; i++ {
		h := p.read(off, 4)
		if len(h) < 4 {
			return
		}
		if h[0] != 0xFF {
			return
		}
		marker := h[1]
		if marker == 0xFF { // fill byte
			off++
			continue
		}
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			off += 2
			continue
		}
		if marker == 0xD9 || marker == 0xDA { // end of image, start of scan
			return
		}
		ln := int64(be16(h[2:]))
		switch {
		case marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC:
			if d := p.read(off+4, 6); len(d) == 6 {
				p.m.Height = int(be16(d[1:]))
				p.m.Width = int(be16(d[3:]))
				p.m.BitDepth = int(d[0]) * int(d[5])
				if marker == 0xC2 || marker == 0xC6 || marker == 0xCA || marker == 0xCE {
					p.m.addTag("Progressive", "Yes")
				}
			}
		case marker == 0xE1 && ln > 8:
			d := p.read(off+4, int(ln-2))
			if bytes.HasPrefix(d, []byte("Exif\x00\x00")) {
				p.exif(d[6:])
			}
		case marker == 0xFE && ln > 2:
			p.m.addTag("Comment", string(p.read(off+4, int(min(ln-2, 4096)))))
		}
		off += 2 + ln
	}
}

func (p *prober) gif() {
	h := p.read(6, 7)
	if len(h) < 7 {
		return
	}
	p.m.Width, p.m.Height = int(le16(h)), int(le16(h[2:]))
	p.m.BitDepth = int(h[4]&7) + 1
	// Count the image descriptors (frames); limited so huge GIFs stay quick.
	off := int64(13)
	if h[4]&0x80 != 0 {
		off += 3 << (int(h[4]&7) + 1)
	}
	frames := 0
	skipBlocks := func() bool {
		for i := 0; i < 1<<20; i++ {
			b := p.read(off, 1)
			if len(b) == 0 {
				return false
			}
			off += 1 + int64(b[0])
			if b[0] == 0 {
				return true
			}
		}
		return false
	}
	for steps := 0; steps < 100000 && off < p.size; steps++ {
		b := p.read(off, 1)
		if len(b) == 0 {
			break
		}
		switch b[0] {
		case 0x2C:
			d := p.read(off, 10)
			if len(d) < 10 {
				steps = 1 << 30
				continue
			}
			frames++
			off += 10
			if d[9]&0x80 != 0 {
				off += 3 << (int(d[9]&7) + 1)
			}
			off++ // LZW minimum code size
			if !skipBlocks() {
				steps = 1 << 30
			}
		case 0x21:
			off += 2
			if !skipBlocks() {
				steps = 1 << 30
			}
		case 0x3B:
			steps = 1 << 30
		default:
			steps = 1 << 30
		}
	}
	p.m.Frames = max(frames, 1)
}

func (p *prober) bmp() {
	h := p.read(14, 26)
	if len(h) < 16 {
		return
	}
	if le32(h) == 12 { // OS/2 BITMAPCOREHEADER
		p.m.Width, p.m.Height = int(le16(h[4:])), int(le16(h[6:]))
		p.m.BitDepth = int(le16(h[10:]))
	} else {
		w, hh := int32(le32(h[4:])), int32(le32(h[8:]))
		if hh < 0 {
			hh = -hh
		}
		p.m.Width, p.m.Height = int(w), int(hh)
		p.m.BitDepth = int(le16(h[14:]))
	}
	p.m.Frames = 1
}

func (p *prober) webp() {
	h := p.read(12, 30)
	if len(h) < 13 {
		return
	}
	p.m.Frames = 1
	switch string(h[:4]) {
	case "VP8 ":
		if len(h) >= 18 {
			p.m.Width = int(le16(h[14:]) & 0x3FFF)
			p.m.Height = int(le16(h[16:]) & 0x3FFF)
		}
		p.m.addTag("Compression", "Lossy")
	case "VP8L":
		if len(h) >= 13 {
			v := le32(h[9:])
			p.m.Width = int(v&0x3FFF) + 1
			p.m.Height = int(v>>14&0x3FFF) + 1
		}
		p.m.addTag("Compression", "Lossless")
	case "VP8X":
		if len(h) < 18 {
			return
		}
		flags := h[8]
		p.m.Width = int(uint32(h[12])|uint32(h[13])<<8|uint32(h[14])<<16) + 1
		p.m.Height = int(uint32(h[15])|uint32(h[16])<<8|uint32(h[17])<<16) + 1
		if flags&0x02 != 0 {
			// Animated: count ANMF chunks.
			frames := 0
			off := int64(12)
			for i := 0; i < 100000 && off+8 <= p.size; i++ {
				c := p.read(off, 8)
				if len(c) < 8 {
					break
				}
				if string(c[:4]) == "ANMF" {
					frames++
				}
				if string(c[:4]) == "EXIF" {
					p.exif(p.read(off+8, int(min(int64(le32(c[4:])), 1<<20))))
				}
				off += 8 + int64(le32(c[4:])+le32(c[4:])&1)
			}
			p.m.Frames = max(frames, 1)
		} else if flags&0x08 != 0 {
			off := int64(12)
			for i := 0; i < 64 && off+8 <= p.size; i++ {
				c := p.read(off, 8)
				if len(c) < 8 {
					break
				}
				if string(c[:4]) == "EXIF" {
					p.exif(p.read(off+8, int(min(int64(le32(c[4:])), 1<<20))))
					break
				}
				off += 8 + int64(le32(c[4:])+le32(c[4:])&1)
			}
		}
	}
}

func (p *prober) tiff() {
	d := p.read(0, int(min(p.size, 1<<20)))
	p.m.Frames = 1
	t := parseTIFF(d)
	if t == nil {
		return
	}
	p.m.Width = int(t.ifd0[0x0100])
	p.m.Height = int(t.ifd0[0x0101])
	p.m.BitDepth = int(t.ifd0[0x0102])
	if spp := int(t.ifd0[0x0115]); spp > 1 {
		p.m.BitDepth *= spp
	}
	p.m.Frames = t.pages
	p.applyEXIF(t)
}

func (p *prober) ico() {
	h := p.read(4, 2)
	if len(h) < 2 {
		return
	}
	count := int(le16(h))
	p.m.Frames = count
	for i := 0; i < count && i < 256; i++ {
		e := p.read(6+int64(i)*16, 16)
		if len(e) < 16 {
			break
		}
		w, hh := int(e[0]), int(e[1])
		if w == 0 {
			w = 256
		}
		if hh == 0 {
			hh = 256
		}
		if w*hh > p.m.Width*p.m.Height {
			p.m.Width, p.m.Height = w, hh
			p.m.BitDepth = int(le16(e[6:]))
		}
	}
}

func (p *prober) psd() {
	h := p.read(0, 26)
	if len(h) < 26 {
		return
	}
	p.m.Height, p.m.Width = int(be32(h[14:])), int(be32(h[18:]))
	p.m.BitDepth = int(be16(h[22:])) * int(be16(h[12:]))
	p.m.Frames = 1
}

var (
	svgAttr    = regexp.MustCompile(`(?is)<svg\b[^>]*>`)
	svgWidth   = regexp.MustCompile(`(?i)\swidth\s*=\s*["']\s*([0-9.]+)\s*(px)?\s*["']`)
	svgHeight  = regexp.MustCompile(`(?i)\sheight\s*=\s*["']\s*([0-9.]+)\s*(px)?\s*["']`)
	svgViewBox = regexp.MustCompile(`(?i)\sviewBox\s*=\s*["']\s*[-0-9.]+[\s,]+[-0-9.]+[\s,]+([0-9.]+)[\s,]+([0-9.]+)\s*["']`)
)

func (p *prober) svg() {
	tag := svgAttr.Find(p.head)
	if tag == nil {
		return
	}
	p.m.Frames = 1
	num := func(s []byte) int { f, _ := strconv.ParseFloat(string(s), 64); return int(math.Round(f)) }
	if m := svgWidth.FindSubmatch(tag); m != nil {
		p.m.Width = num(m[1])
	}
	if m := svgHeight.FindSubmatch(tag); m != nil {
		p.m.Height = num(m[1])
	}
	if m := svgViewBox.FindSubmatch(tag); m != nil && (p.m.Width == 0 || p.m.Height == 0) {
		p.m.Width, p.m.Height = num(m[1]), num(m[2])
	}
}

// --- documents ----------------------------------------------------------------

var pdfPage = regexp.MustCompile(`/Type\s*/Page[^s]`)
var pdfCount = regexp.MustCompile(`/Type\s*/Pages\b[^>]*?/Count\s+(\d+)|/Count\s+(\d+)[^>]*?/Type\s*/Pages\b`)

func (p *prober) pdf() {
	if len(p.head) >= 8 {
		p.m.addTag("PDF version", strings.TrimSpace(string(p.head[5:8])))
	}
	// Prefer the page tree's /Count (largest one is the root); fall back to
	// counting page objects. Both scan at most the first 64 MiB.
	limit := min(p.size, 64<<20)
	maxCount, pages := 0, 0
	const chunk = 1 << 20
	for off := int64(0); off < limit; off += chunk - 64 {
		d := p.read(off, chunk)
		for _, m := range pdfCount.FindAllSubmatch(d, -1) {
			s := m[1]
			if len(s) == 0 {
				s = m[2]
			}
			if c, err := strconv.Atoi(string(s)); err == nil && c > maxCount {
				maxCount = c
			}
		}
		// Count only matches that start in this chunk's non-overlapping part.
		for _, loc := range pdfPage.FindAllIndex(d, -1) {
			if off == 0 || loc[0] >= 64 {
				pages++
			}
		}
		if len(d) < chunk {
			break
		}
	}
	if maxCount > 0 {
		p.m.Frames = maxCount
	} else {
		p.m.Frames = pages
	}
	if i := bytes.Index(p.head, []byte("/Title")); i >= 0 {
		rest := p.head[i+6:]
		rest = bytes.TrimLeft(rest, " \r\n")
		if len(rest) > 0 && rest[0] == '(' {
			if j := bytes.IndexByte(rest, ')'); j > 0 {
				t := rest[1:j]
				if bytes.HasPrefix(t, []byte("\xfe\xff")) {
					p.m.addTag("Title", DecodeString(UTF16BE, t[2:]))
				} else {
					p.m.addTag("Title", string(t))
				}
			}
		}
	}
}

// --- audio ----------------------------------------------------------------

func (p *prober) wav() {
	off := int64(12)
	var byteRate int64
	for i := 0; i < 1000 && off+8 <= p.size; i++ {
		c := p.read(off, 8)
		if len(c) < 8 {
			return
		}
		ln := int64(le32(c[4:]))
		switch string(c[:4]) {
		case "fmt ":
			if d := p.read(off+8, 16); len(d) == 16 {
				p.m.Channels = int(le16(d[2:]))
				p.m.SampleRate = int(le32(d[4:]))
				byteRate = int64(le32(d[8:]))
				p.m.BitDepth = int(le16(d[14:]))
				p.m.Bitrate = int(byteRate * 8)
			}
		case "data":
			if ln == 0xFFFFFFFF || off+8+ln > p.size {
				ln = p.size - off - 8
			}
			if byteRate > 0 {
				p.m.Duration = time.Duration(float64(ln) / float64(byteRate) * float64(time.Second))
			}
			return
		}
		off += 8 + ln + ln&1
	}
}

func (p *prober) flac() {
	off := int64(4)
	for i := 0; i < 128 && off+4 <= p.size; i++ {
		h := p.read(off, 4)
		if len(h) < 4 {
			return
		}
		last := h[0]&0x80 != 0
		typ := h[0] & 0x7F
		ln := int64(h[1])<<16 | int64(h[2])<<8 | int64(h[3])
		switch typ {
		case 0:
			if d := p.read(off+4, 18); len(d) == 18 {
				x := be64(d[10:])
				p.m.SampleRate = int(x >> 44)
				p.m.Channels = int(x>>41&7) + 1
				p.m.BitDepth = int(x>>36&31) + 1
				total := x & (1<<36 - 1)
				if p.m.SampleRate > 0 {
					p.m.Duration = time.Duration(float64(total) / float64(p.m.SampleRate) * float64(time.Second))
					if p.m.Duration > 0 {
						p.m.Bitrate = int(float64(p.size*8) / p.m.Duration.Seconds())
					}
				}
			}
		case 4:
			if ln < 1<<20 {
				p.vorbisComments(p.read(off+4, int(ln)))
			}
		}
		if last {
			return
		}
		off += 4 + ln
	}
}

// vorbisComments reads a Vorbis comment block (FLAC, Ogg Vorbis, Opus).
func (p *prober) vorbisComments(d []byte) {
	if len(d) < 8 {
		return
	}
	vl := int(le32(d))
	if 4+vl+4 > len(d) {
		return
	}
	pos := 4 + vl
	n := int(le32(d[pos:]))
	pos += 4
	names := map[string]string{"TITLE": "Title", "ARTIST": "Artist", "ALBUM": "Album", "DATE": "Year", "GENRE": "Genre", "TRACKNUMBER": "Track"}
	for i := 0; i < n && pos+4 <= len(d); i++ {
		l := int(le32(d[pos:]))
		pos += 4
		if pos+l > len(d) {
			return
		}
		kv := string(d[pos : pos+l])
		pos += l
		if k, v, ok := strings.Cut(kv, "="); ok {
			if name, ok := names[strings.ToUpper(k)]; ok {
				p.m.addTag(name, v)
			}
		}
	}
}

func (p *prober) ogg() {
	// First page: identification header.
	h := p.read(0, 27)
	if len(h) < 27 {
		return
	}
	nseg := int(h[26])
	segs := p.read(27, nseg)
	body := int64(27 + nseg)
	plen := 0
	for _, s := range segs {
		plen += int(s)
	}
	pkt := p.read(body, plen)
	rate := 0
	switch {
	case bytes.HasPrefix(pkt, []byte("\x01vorbis")) && len(pkt) >= 16:
		p.m.Format, p.m.MIME = "Ogg Vorbis", "audio/ogg"
		p.m.Channels = int(pkt[11])
		p.m.SampleRate = int(le32(pkt[12:]))
		rate = p.m.SampleRate
		if len(pkt) >= 24 {
			p.m.Bitrate = int(int32(le32(pkt[20:])))
		}
	case bytes.HasPrefix(pkt, []byte("OpusHead")) && len(pkt) >= 16:
		p.m.Format, p.m.MIME = "Opus", "audio/ogg"
		p.m.Channels = int(pkt[9])
		p.m.SampleRate = int(le32(pkt[12:]))
		rate = 48000 // Opus granule positions always count 48 kHz samples
	case bytes.HasPrefix(pkt, []byte("\x80theora")):
		p.m.Format, p.m.MIME, p.m.Kind = "Ogg Theora", "video/ogg", "video"
		if len(pkt) >= 20 {
			p.m.Width = int(pkt[14])<<16 | int(pkt[15])<<8 | int(pkt[16])
			p.m.Height = int(pkt[17])<<16 | int(pkt[18])<<8 | int(pkt[19])
		}
	case bytes.HasPrefix(pkt, []byte("\x7fFLAC")):
		p.m.Format, p.m.MIME = "Ogg FLAC", "audio/ogg"
	}
	// Second page usually holds the comment header.
	off2 := body + int64(plen)
	if h2 := p.read(off2, 27); len(h2) == 27 && string(h2[:4]) == "OggS" {
		n2 := int(h2[26])
		s2 := p.read(off2+27, n2)
		l2 := 0
		for _, s := range s2 {
			l2 += int(s)
		}
		pk2 := p.read(off2+27+int64(n2), l2)
		switch {
		case bytes.HasPrefix(pk2, []byte("\x03vorbis")):
			p.vorbisComments(pk2[7:])
		case bytes.HasPrefix(pk2, []byte("OpusTags")):
			p.vorbisComments(pk2[8:])
		}
	}
	// Duration: granule position of the last page.
	if rate > 0 {
		tailLen := int(min(p.size, 128*1024))
		tail := p.read(p.size-int64(tailLen), tailLen)
		if i := bytes.LastIndex(tail, []byte("OggS")); i >= 0 && i+14 <= len(tail) {
			g := int64(le64(tail[i+6:]))
			if g > 0 {
				p.m.Duration = time.Duration(float64(g) / float64(rate) * float64(time.Second))
			}
		}
	}
}

var mp3Bitrates = [2][3][16]int{
	{ // MPEG-1: layer I, II, III
		{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0},
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0},
		{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
	},
	{ // MPEG-2/2.5
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0},
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
	},
}

func (p *prober) mp3() {
	off := int64(0)
	if h := p.read(0, 10); len(h) == 10 && string(h[:3]) == "ID3" {
		sz := int64(h[6]&0x7F)<<21 | int64(h[7]&0x7F)<<14 | int64(h[8]&0x7F)<<7 | int64(h[9]&0x7F)
		p.id3v2(p.read(10, int(min(sz, 1<<20))), int(h[3]))
		off = 10 + sz
		if h[5]&0x10 != 0 {
			off += 10
		}
	}
	// Find the first frame header (skip padding).
	scan := p.read(off, 64*1024)
	i := 0
	for ; i+4 <= len(scan); i++ {
		if scan[i] == 0xFF && scan[i+1]&0xE0 == 0xE0 {
			if _, _, ok := mp3Header(scan[i:]); ok {
				break
			}
		}
	}
	if i+4 > len(scan) {
		return
	}
	fh := scan[i:]
	frameOff := off + int64(i)
	ver := fh[1] >> 3 & 3 // 3 = MPEG-1, 2 = MPEG-2, 0 = MPEG-2.5
	layer := 4 - int(fh[1]>>1&3)
	br, sr, _ := mp3Header(fh)
	p.m.SampleRate = sr
	p.m.Bitrate = br * 1000
	if fh[3]>>6 == 3 {
		p.m.Channels = 1
	} else {
		p.m.Channels = 2
	}
	p.m.addTag("Layer", "MPEG layer "+strings.Repeat("I", layer))
	// A Xing/Info or VBRI header gives the exact frame count for VBR files.
	samplesPerFrame := 1152
	if layer == 1 {
		samplesPerFrame = 384
	} else if layer == 3 && ver != 3 {
		samplesPerFrame = 576
	}
	sideInfo := 32
	if ver != 3 {
		sideInfo = 17
	}
	if p.m.Channels == 1 {
		if ver == 3 {
			sideInfo = 17
		} else {
			sideInfo = 9
		}
	}
	frames := int64(0)
	if x := p.read(frameOff+4+int64(sideInfo), 12); len(x) == 12 && (string(x[:4]) == "Xing" || string(x[:4]) == "Info") {
		if be32(x[4:])&1 != 0 {
			frames = int64(be32(x[8:]))
		}
	} else if v := p.read(frameOff+36, 18); len(v) == 18 && string(v[:4]) == "VBRI" {
		frames = int64(be32(v[14:]))
	}
	audioBytes := p.size - frameOff
	if t := p.read(p.size-128, 3); string(t) == "TAG" {
		audioBytes -= 128
		if len(p.m.Tags) <= 1 {
			p.id3v1(p.read(p.size-128, 128))
		}
	}
	if frames > 0 && sr > 0 {
		p.m.Duration = time.Duration(float64(frames*int64(samplesPerFrame)) / float64(sr) * float64(time.Second))
		if s := p.m.Duration.Seconds(); s > 0 {
			p.m.Bitrate = int(float64(audioBytes*8) / s)
		}
	} else if br > 0 {
		p.m.Duration = time.Duration(float64(audioBytes*8) / float64(br*1000) * float64(time.Second))
	}
}

// mp3Header validates an MPEG audio frame header and returns its bitrate
// (kbit/s) and sample rate (Hz).
func mp3Header(h []byte) (kbps, rate int, ok bool) {
	if len(h) < 4 || h[0] != 0xFF || h[1]&0xE0 != 0xE0 {
		return 0, 0, false
	}
	ver := h[1] >> 3 & 3
	layerBits := h[1] >> 1 & 3
	if ver == 1 || layerBits == 0 {
		return 0, 0, false
	}
	bi := h[2] >> 4
	si := h[2] >> 2 & 3
	if bi == 0 || bi == 15 || si == 3 {
		return 0, 0, false
	}
	rates := [4][3]int{{11025, 12000, 8000}, {}, {22050, 24000, 16000}, {44100, 48000, 32000}}
	vi := 1
	if ver == 3 {
		vi = 0
	}
	layer := 3 - int(layerBits) // 0 = layer I
	return mp3Bitrates[vi][layer][bi], rates[ver][si], true
}

func (p *prober) id3v2(d []byte, major int) {
	names := map[string]string{
		"TIT2": "Title", "TPE1": "Artist", "TALB": "Album", "TYER": "Year", "TDRC": "Year",
		"TCON": "Genre", "TRCK": "Track", "COMM": "Comment", "TPE2": "Album artist",
		"TT2": "Title", "TP1": "Artist", "TAL": "Album", "TYE": "Year", "TCO": "Genre", "TRK": "Track",
	}
	pos := 0
	for pos+6 <= len(d) {
		var id string
		var sz int
		if major == 2 {
			id = string(d[pos : pos+3])
			sz = int(d[pos+3])<<16 | int(d[pos+4])<<8 | int(d[pos+5])
			pos += 6
		} else {
			if pos+10 > len(d) {
				return
			}
			id = string(d[pos : pos+4])
			if major == 4 {
				sz = int(d[pos+4]&0x7F)<<21 | int(d[pos+5]&0x7F)<<14 | int(d[pos+6]&0x7F)<<7 | int(d[pos+7]&0x7F)
			} else {
				sz = int(be32(d[pos+4:]))
			}
			pos += 10
		}
		if id[0] == 0 || sz <= 0 || pos+sz > len(d) {
			return
		}
		body := d[pos : pos+sz]
		pos += sz
		name, ok := names[id]
		if !ok || len(body) < 2 {
			continue
		}
		enc := body[0]
		text := body[1:]
		if id == "COMM" {
			if len(text) < 3 {
				continue
			}
			text = text[3:] // language
			// skip the short description
			if enc == 1 || enc == 2 {
				for k := 0; k+1 < len(text); k += 2 {
					if text[k] == 0 && text[k+1] == 0 {
						text = text[k+2:]
						break
					}
				}
			} else if k := bytes.IndexByte(text, 0); k >= 0 {
				text = text[k+1:]
			}
		}
		p.m.addTag(name, id3Text(enc, text))
	}
}

func id3Text(enc byte, b []byte) string {
	switch enc {
	case 1: // UTF-16 with BOM
		if e, n := BOM(b); n == 2 {
			return DecodeString(e, b[2:])
		}
		return DecodeString(UTF16LE, b)
	case 2:
		return DecodeString(UTF16BE, b)
	case 3:
		return string(b)
	}
	return DecodeString(ISO8859_1, b)
}

func (p *prober) id3v1(d []byte) {
	if len(d) < 128 {
		return
	}
	field := func(a, b int) string { return DecodeString(ISO8859_1, bytes.TrimRight(d[a:b], "\x00 ")) }
	p.m.addTag("Title", field(3, 33))
	p.m.addTag("Artist", field(33, 63))
	p.m.addTag("Album", field(63, 93))
	p.m.addTag("Year", field(93, 97))
}

// --- video ----------------------------------------------------------------

func (p *prober) avi() {
	// RIFF AVI / LIST hdrl / avih
	off := int64(12)
	for i := 0; i < 64 && off+12 <= p.size; i++ {
		c := p.read(off, 12)
		if len(c) < 12 {
			return
		}
		ln := int64(le32(c[4:]))
		if string(c[:4]) == "LIST" && string(c[8:12]) == "hdrl" {
			h := p.read(off+12, 8+56)
			if len(h) >= 64 && string(h[:4]) == "avih" {
				usPerFrame := int64(le32(h[8:]))
				frames := int64(le32(h[8+16:]))
				p.m.Width = int(le32(h[8+32:]))
				p.m.Height = int(le32(h[8+36:]))
				p.m.Frames = int(frames)
				p.m.Duration = time.Duration(usPerFrame*frames) * time.Microsecond
				if usPerFrame > 0 {
					p.m.addTag("Frame rate", fmt.Sprintf("%.3g fps", 1e6/float64(usPerFrame)))
				}
			}
			return
		}
		off += 8 + ln + ln&1
	}
}

// isobmff walks the boxes of MP4, QuickTime, HEIC and AVIF files.
func (p *prober) isobmff() {
	var video, audio bool
	var walk func(off, end int64, depth int)
	walk = func(off, end int64, depth int) {
		for i := 0; i < 4096 && off+8 <= end; i++ {
			h := p.read(off, 16)
			if len(h) < 8 {
				return
			}
			size := int64(be32(h))
			typ := string(h[4:8])
			hdr := int64(8)
			if size == 1 && len(h) >= 16 {
				size = int64(be64(h[8:]))
				hdr = 16
			} else if size == 0 {
				size = end - off
			}
			if size < hdr || off+size > end {
				size = end - off
				if size < hdr {
					return
				}
			}
			body, bodyEnd := off+hdr, off+size
			switch typ {
			case "moov", "trak", "mdia", "minf", "stbl", "iprp", "ipco", "udta", "edts":
				if depth < 12 {
					walk(body, bodyEnd, depth+1)
				}
			case "meta":
				// A full box in ISO files (4 bytes version/flags), but a plain
				// container in QuickTime: detect by the next box's type.
				skip := int64(4)
				if t := p.read(body+4, 4); len(t) == 4 && (string(t) == "hdlr" || string(t) == "keys") {
					skip = 0
				}
				if depth < 12 {
					walk(body+skip, bodyEnd, depth+1)
				}
			case "mvhd":
				d := p.read(body, 32)
				if len(d) >= 20 {
					var scale, dur uint64
					if d[0] == 1 && len(d) >= 32 {
						scale, dur = uint64(be32(d[20:])), be64(d[24:])
					} else {
						scale, dur = uint64(be32(d[12:])), uint64(be32(d[16:]))
					}
					if scale > 0 {
						p.m.Duration = time.Duration(float64(dur) / float64(scale) * float64(time.Second))
					}
				}
			case "tkhd":
				d := p.read(body, int(min(size-hdr, 100)))
				if len(d) >= 84 {
					w := int(be32(d[len(d)-8:]) >> 16)
					hh := int(be32(d[len(d)-4:]) >> 16)
					if w > 0 && hh > 0 && w*hh > p.m.Width*p.m.Height {
						p.m.Width, p.m.Height = w, hh
					}
				}
			case "hdlr":
				d := p.read(body, 12)
				if len(d) >= 12 {
					switch string(d[8:12]) {
					case "vide":
						video = true
					case "soun":
						audio = true
					}
				}
			case "stsd":
				d := p.read(body, 8+8+28)
				if len(d) >= 16+20 && audio && p.m.SampleRate == 0 && !video {
					// Audio sample entry: channels at +24, rate (16.16) at +32 of the entry.
					if len(d) >= 8+8+28 {
						p.m.Channels = int(be16(d[8+8+16:]))
						p.m.SampleRate = int(be32(d[8+8+24:]) >> 16)
					}
				}
			case "ispe":
				d := p.read(body, 12)
				if len(d) == 12 {
					w, hh := int(be32(d[4:])), int(be32(d[8:]))
					if w*hh > p.m.Width*p.m.Height {
						p.m.Width, p.m.Height = w, hh
					}
				}
			case "pixi":
				d := p.read(body, 8)
				if len(d) >= 6 {
					n := int(d[4])
					bits := 0
					for k := 0; k < n && 5+k < len(d); k++ {
						bits += int(d[5+k])
					}
					p.m.BitDepth = bits
				}
			}
			off += size
		}
	}
	walk(0, p.size, 0)
	switch {
	case p.m.Kind == "image":
		p.m.Frames = 1
	case video:
		p.m.Kind = "video"
	case audio:
		p.m.Kind = "audio"
		if p.m.Format == "MP4" {
			p.m.Format, p.m.MIME = "M4A", "audio/mp4"
		}
	}
	if p.m.Duration > 0 {
		p.m.Bitrate = int(float64(p.size*8) / p.m.Duration.Seconds())
	}
}

// ebmlVint reads an EBML variable-length integer. mask=true strips the
// length marker (element sizes); false keeps it (element IDs).
func ebmlVint(b []byte, mask bool) (uint64, int) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0
	}
	n := 1
	for m := byte(0x80); b[0]&m == 0; m >>= 1 {
		n++
	}
	if n > 8 || n > len(b) {
		return 0, 0
	}
	v := uint64(b[0])
	if mask {
		v &= uint64(0xFF >> n)
	}
	for i := 1; i < n; i++ {
		v = v<<8 | uint64(b[i])
	}
	return v, n
}

func (p *prober) mkv() {
	scale := uint64(1000000)
	var dur float64
	var walk func(off, end int64, depth int)
	walk = func(off, end int64, depth int) {
		for i := 0; i < 4096 && off < end; i++ {
			h := p.read(off, 16)
			id, n1 := ebmlVint(h, false)
			if n1 == 0 {
				return
			}
			sz, n2 := ebmlVint(h[n1:], true)
			if n2 == 0 {
				return
			}
			body := off + int64(n1+n2)
			unknown := sz == (uint64(1)<<(7*n2))-1
			if unknown || body+int64(sz) > end {
				sz = uint64(end - body)
			}
			readUint := func() uint64 {
				var v uint64
				for _, c := range p.read(body, int(min(sz, 8))) {
					v = v<<8 | uint64(c)
				}
				return v
			}
			switch id {
			case 0x18538067, 0x1654AE6B, 0xAE, 0xE0, 0xE1: // Segment, Tracks, TrackEntry, Video, Audio
				if depth < 8 {
					walk(body, body+int64(sz), depth+1)
				}
			case 0x1549A966: // Info
				walk(body, body+int64(sz), depth+1)
			case 0x2AD7B1:
				scale = readUint()
			case 0x4489:
				d := p.read(body, int(sz))
				if len(d) == 4 {
					dur = float64(math.Float32frombits(be32(d)))
				} else if len(d) == 8 {
					dur = math.Float64frombits(be64(d))
				}
			case 0xB0:
				if w := int(readUint()); w > p.m.Width {
					p.m.Width = w
				}
			case 0xBA:
				if hh := int(readUint()); hh > p.m.Height {
					p.m.Height = hh
				}
			case 0xB5:
				d := p.read(body, int(sz))
				if len(d) == 4 {
					p.m.SampleRate = int(math.Float32frombits(be32(d)))
				} else if len(d) == 8 {
					p.m.SampleRate = int(math.Float64frombits(be64(d)))
				}
			case 0x9F:
				p.m.Channels = int(readUint())
			case 0x1F43B675, 0x1C53BB6B: // Cluster, Cues: media data, stop
				return
			}
			off = body + int64(sz)
		}
	}
	walk(0, p.size, 0)
	if dur > 0 {
		p.m.Duration = time.Duration(dur * float64(scale))
	}
	if p.m.Width == 0 && p.m.SampleRate > 0 {
		p.m.Kind = "audio"
		if p.m.Format == "WebM" {
			p.m.MIME = "audio/webm"
		} else {
			p.m.MIME = "audio/x-matroska"
		}
	}
}

// decodeUTF16Units is used by EXIF XP* tags (UTF-16LE).
func decodeUTF16Units(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		c := le16(b[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}
