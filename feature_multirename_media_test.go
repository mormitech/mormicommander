package main

// Tests of the Multi-Rename tool's media fields (board issue #7): EXIF dates, image sizes and ID3 tags of files in a
// temporary folder end up in the preview, chained F5 steps still read the real files, folders and files without the
// facts give empty text, and a changed file is read again.

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mormicommander/internal/multirename"
)

// mrExifJPEG makes a minimal JPEG header whose EXIF block carries DateTimeOriginal.
func mrExifJPEG(date string) []byte {
	var tiff bytes.Buffer
	bo := binary.BigEndian
	w := func(v any) { binary.Write(&tiff, bo, v) }
	tiff.WriteString("MM")
	w(uint16(42))
	w(uint32(8))
	w(uint16(1)) // IFD0: one entry, the Exif IFD pointer to offset 26
	w(uint16(0x8769))
	w(uint16(4))
	w(uint32(1))
	w(uint32(26))
	w(uint32(0))
	w(uint16(1)) // Exif IFD: DateTimeOriginal, 20 ASCII bytes at offset 44
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

// mrID3 makes an ID3v2.3 header with UTF-8 text frames.
func mrID3(frames [][2]string) []byte {
	var body bytes.Buffer
	for _, f := range frames {
		data := append([]byte{3}, []byte(f[1])...)
		body.WriteString(f[0])
		binary.Write(&body, binary.BigEndian, uint32(len(data)))
		body.Write([]byte{0, 0})
		body.Write(data)
	}
	n := body.Len()
	hdr := []byte{'I', 'D', '3', 3, 0, 0, byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
	return append(hdr, body.Bytes()...)
}

func TestMultiRenameMediaFields(t *testing.T) {
	m, dir := mrSetup(t)
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var pb bytes.Buffer
	if err := png.Encode(&pb, image.NewRGBA(image.Rect(0, 0, 12, 7))); err != nil {
		t.Fatal(err)
	}
	write("a.jpg", mrExifJPEG("2021:05:06 07:08:09"))
	write("b.png", pb.Bytes())
	write("c.mp3", mrID3([][2]string{{"TPE1", "Band"}, {"TIT2", "Song"}, {"TRCK", "3/12"}}))
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := m.Start(dir, []string{"a.jpg", "b.png", "c.mp3", "d"})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Media) == 0 || st.Media[0] != "img.exifdate" {
		t.Fatalf("media field names: %v", st.Media)
	}
	for _, n := range st.Media {
		if _, ok := mrMediaFields[n]; !ok {
			t.Errorf("menu field %q has no implementation", n) // every name in the menu must be answered
		}
	}
	s := multirename.DefaultSettings()
	check := func(mask string, want ...string) {
		t.Helper()
		s.NameMask = mask
		p := m.Preview(s)
		if p.FieldError != "" {
			t.Fatalf("%s: %s", mask, p.FieldError)
		}
		got := mrNews(p)
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: got %q, want %q", mask, got, want)
				return
			}
		}
	}
	check("[=img.exifyear][=img.exifmonth][=img.exifday]_[N]", "20210506_a.jpg", "_b.png", "_c.mp3", "_d")
	check("[=img.exifdate]", "2021-05-06 07.08.09.jpg")
	check("[N]_[=img.size]", "a_.jpg", "b_12x7.png", "c_.mp3", "d_")
	check("[N]_[=img.width]", "a_.jpg", "b_12.png")
	check("[=audio.track] [=audio.artist] - [=id3.title]", "  - .jpg", "  - .png", "03 Band - Song.mp3", "  - ")

	// After a chained F5 step the facts still come from the real files.
	s.NameMask = "x[N]"
	if err := m.NextStep(s); err != nil {
		t.Fatal(err)
	}
	check("[N]-[=exif.datetimeoriginal]", "xa-2021-05-06 07.08.09.jpg")

	// A changed file is read again.
	write("c.mp3", mrID3([][2]string{{"TPE1", "Other band"}}))
	future := time.Now().Add(time.Hour)
	os.Chtimes(filepath.Join(dir, "c.mp3"), future, future)
	check("[=audio.artist]", ".jpg", ".png", "Other band.mp3", "")

	// Start! renames with the media fields.
	m.Reset()
	s.NameMask = "[=img.exifyear]-[N]"
	s.Search, s.Replace = "^-", ""
	s.RegEx = true
	p := m.Preview(s)
	if p.Errors != 0 {
		t.Fatalf("preview errors: %+v", p.Rows)
	}
	if _, err := m.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2021-a.jpg")); err != nil {
		t.Errorf("not renamed: %v", mrList(t, dir))
	}
}
