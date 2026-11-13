// Tests for the image, EXIF and ID3 fields, using files generated inside a temp dir.
package listing

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// buildExifJPEG makes a minimal JPEG header with an EXIF block carrying DateTimeOriginal.
func buildExifJPEG(date string, bo binary.ByteOrder) []byte {
	var tiff bytes.Buffer
	if bo == binary.LittleEndian {
		tiff.WriteString("II")
	} else {
		tiff.WriteString("MM")
	}
	w := func(v any) { binary.Write(&tiff, bo, v) }
	w(uint16(42))
	w(uint32(8)) // IFD0 at 8
	// IFD0: one entry, ExifIFD pointer -> 26
	w(uint16(1))
	w(uint16(0x8769))
	w(uint16(4))
	w(uint32(1))
	w(uint32(26))
	w(uint32(0)) // next IFD
	// Exif IFD at 26: DateTimeOriginal ASCII 20 bytes at 44
	w(uint16(1))
	w(uint16(0x9003))
	w(uint16(2))
	w(uint32(20))
	w(uint32(44))
	w(uint32(0))
	tiff.WriteString(date)
	tiff.WriteByte(0)
	seg := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	var out bytes.Buffer
	out.Write([]byte{0xFF, 0xD8, 0xFF, 0xE1})
	binary.Write(&out, binary.BigEndian, uint16(len(seg)+2))
	out.Write(seg)
	out.Write([]byte{0xFF, 0xD9})
	return out.Bytes()
}

func TestExifDate(t *testing.T) {
	for _, bo := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		data := buildExifJPEG("2024:07:15 18:30:05", bo)
		got, ok := ExifDate(data)
		want := time.Date(2024, 7, 15, 18, 30, 5, 0, time.Local)
		if !ok || !got.Equal(want) {
			t.Errorf("%v: got %v %v", bo, got, ok)
		}
	}
	for _, bad := range [][]byte{nil, []byte("not a jpeg"), {0xFF, 0xD8, 0xFF, 0xE1, 0x00}} {
		if _, ok := ExifDate(bad); ok {
			t.Errorf("garbage %q parsed", bad)
		}
	}
}

func id3v23(frames map[string]string) []byte {
	var body bytes.Buffer
	for id, text := range frames {
		data := append([]byte{3}, []byte(text)...) // UTF-8
		body.WriteString(id)
		binary.Write(&body, binary.BigEndian, uint32(len(data)))
		body.Write([]byte{0, 0})
		body.Write(data)
	}
	n := body.Len()
	hdr := []byte{'I', 'D', '3', 3, 0, 0, byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
	return append(hdr, body.Bytes()...)
}

func TestReadID3(t *testing.T) {
	v2 := id3v23(map[string]string{"TIT2": "Sång", "TPE1": "Artist", "TALB": "Album", "TYER": "1999", "TRCK": "3/12", "TCON": "Rock"})
	got, ok := ReadID3(bytes.NewReader(append(v2, make([]byte, 200)...)))
	want := AudioTags{Title: "Sång", Artist: "Artist", Album: "Album", Year: "1999", Track: 3, Genre: "Rock"}
	if !ok || got != want {
		t.Errorf("v2: %+v %v", got, ok)
	}
	v1 := make([]byte, 128)
	copy(v1, "TAG")
	copy(v1[3:], "Old Title")
	copy(v1[33:], "Old Artist")
	copy(v1[93:], "1987")
	v1[126] = 7
	got, ok = ReadID3(bytes.NewReader(append(make([]byte, 500), v1...)))
	if !ok || got.Title != "Old Title" || got.Artist != "Old Artist" || got.Year != "1987" || got.Track != 7 {
		t.Errorf("v1: %+v %v", got, ok)
	}
	if _, ok := ReadID3(bytes.NewReader(make([]byte, 300))); ok {
		t.Error("no tags parsed as tags")
	}
	// UTF-16 with BOM.
	if s := decodeID3Text([]byte{1, 0xFF, 0xFE, 'H', 0, 'i', 0, 0, 0}); s != "Hi" {
		t.Errorf("utf16: %q", s)
	}
}

func TestMediaFields(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 12, 7))); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "p.png"), buf.Bytes(), 0o644)
	os.WriteFile(filepath.Join(dir, "x.jpg"), buildExifJPEG("2020:01:02 03:04:05", binary.BigEndian), 0o644)
	os.WriteFile(filepath.Join(dir, "s.mp3"), id3v23(map[string]string{"TIT2": "Song", "TRCK": "9"}), 0o644)

	set := ColumnSet{Columns: []Column{
		{Template: "[=img.width]x[=img.height]"}, {Template: "[=img.size]"},
		{Template: "[=img.exifdate.yyyy-MM-dd]"}, {Template: "[=audio.title] #[=audio.track]"},
	}}.MustCompile()
	f := &Format{}
	png := Item{Name: "p.png", Dir: dir, Size: 1}
	jpg := Item{Name: "x.jpg", Dir: dir, Size: 1}
	mp3 := Item{Name: "s.mp3", Dir: dir, Size: 1}
	if got := set.Render(&png, f); got[0] != "12x7" || got[1] != "12x7" || got[2] != "" {
		t.Errorf("png: %q", got)
	}
	if got := set.Render(&jpg, f); got[2] != "2020-01-02" {
		t.Errorf("jpg: %q", got)
	}
	if got := set.Render(&mp3, f); got[3] != "Song #9" {
		t.Errorf("mp3: %q", got)
	}
	if v := set.Columns[1].SortValue(&png, f); v.Num != 84 {
		t.Errorf("img.size sorts by pixels: %+v", v)
	}
	missing := Item{Name: "gone.png", Dir: dir}
	if got := set.Render(&missing, f); got[0] != "x" {
		t.Errorf("missing file: %q", got)
	}
}
