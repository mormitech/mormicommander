package listing

// Tests of the EXIF date in image formats beyond JPEG (issue #7), on tiny files made here: PNG with an eXIf chunk,
// an extended WebP with an EXIF chunk, a bare TIFF file, and a minimal HEIF file with an Exif item and an image size
// property, plus damaged and unrelated data.

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// exifTIFFBlock is the TIFF block of buildExifJPEG (the JPEG markers and the "Exif\0\0" header cut off).
func exifTIFFBlock(date string, bo binary.ByteOrder) []byte {
	j := buildExifJPEG(date, bo)
	return j[12 : len(j)-2]
}

func pngChunk(typ string, data []byte) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, uint32(len(data)))
	b.WriteString(typ)
	b.Write(data)
	binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
	return b.Bytes()
}

func pngWithExif(tiff []byte) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr, 4)
	binary.BigEndian.PutUint32(ihdr[4:], 3)
	ihdr[8], ihdr[9] = 8, 2
	b.Write(pngChunk("IHDR", ihdr))
	b.Write(pngChunk("eXIf", tiff))
	b.Write(pngChunk("IEND", nil))
	return b.Bytes()
}

func webpWithExif(tiff []byte) []byte {
	chunk := func(typ string, data []byte) []byte {
		var b bytes.Buffer
		b.WriteString(typ)
		binary.Write(&b, binary.LittleEndian, uint32(len(data)))
		b.Write(data)
		if len(data)%2 == 1 {
			b.WriteByte(0)
		}
		return b.Bytes()
	}
	body := append([]byte("WEBP"), chunk("VP8X", make([]byte, 10))...)
	body = append(body, chunk("EXIF", append([]byte("Exif\x00\x00"), tiff...))...)
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(len(body)))
	b.Write(body)
	return b.Bytes()
}

// heifWithExif builds the smallest HEIF file the reader accepts: ftyp, a meta box (handler, primary item, item
// locations, two items: the image and its Exif, an image size property) and an mdat holding the Exif item.
func heifWithExif(tiff []byte, w, h uint32) []byte {
	be := func(v ...any) []byte {
		var b bytes.Buffer
		for _, x := range v {
			binary.Write(&b, binary.BigEndian, x)
		}
		return b.Bytes()
	}
	box := func(typ string, parts ...[]byte) []byte {
		body := bytes.Join(parts, nil)
		return append(append(be(uint32(8+len(body))), typ...), body...)
	}
	full := func(typ string, ver byte, parts ...[]byte) []byte {
		return box(typ, append([][]byte{{ver, 0, 0, 0}}, parts...)...)
	}
	exif := append(append(be(uint32(6)), "Exif\x00\x00"...), tiff...)
	ftyp := box("ftyp", []byte("heic"), be(uint32(0)), []byte("mif1heic"))
	meta := func(offset uint32) []byte {
		return full("meta", 0,
			full("hdlr", 0, be(uint32(0)), []byte("pict"), make([]byte, 12), []byte{0}),
			full("pitm", 0, be(uint16(1))),
			full("iloc", 0, []byte{0x44, 0x00}, be(uint16(1), uint16(2), uint16(0), uint16(1), offset, uint32(len(exif)))),
			full("iinf", 0, be(uint16(2)),
				full("infe", 2, be(uint16(1), uint16(0)), []byte("hvc1\x00")),
				full("infe", 2, be(uint16(2), uint16(0)), []byte("Exif\x00"))),
			box("iprp",
				box("ipco", full("ispe", 0, be(w, h))),
				full("ipma", 0, be(uint32(1), uint16(1)), []byte{1, 0x81})))
	}
	offset := uint32(len(ftyp) + len(meta(0)) + 8)
	return bytes.Join([][]byte{ftyp, meta(offset), box("mdat", exif)}, nil)
}

func TestExifDateMoreFormats(t *testing.T) {
	want := time.Date(2019, 3, 4, 5, 6, 7, 0, time.Local)
	for _, bo := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		tiff := exifTIFFBlock("2019:03:04 05:06:07", bo)
		for name, data := range map[string][]byte{
			"png":  pngWithExif(tiff),
			"webp": webpWithExif(tiff),
			"tiff": append(append([]byte(nil), tiff...), make([]byte, 100)...),
		} {
			if got, ok := ExifDate(data); !ok || !got.Equal(want) {
				t.Errorf("%s %v: %v %v", name, bo, got, ok)
			}
		}
	}
	for _, bad := range [][]byte{
		pngWithExif(nil), []byte("RIFF\x10\x00\x00\x00WEBPVP8X"), []byte("II*\x00\xff\xff\xff\xff"), []byte("MM\x00\x2b\x00\x00\x00\x08"),
	} {
		if _, ok := ExifDate(bad); ok {
			t.Errorf("garbage %q gave a date", bad)
		}
	}
}

func TestHEIFFacts(t *testing.T) {
	data := heifWithExif(exifTIFFBlock("2022:12:24 18:00:01", binary.BigEndian), 4032, 3024)
	if !isHEIF(data) {
		t.Fatal("not taken for HEIF")
	}
	w, h, exif := heifFacts(bytes.NewReader(data))
	if w != 4032 || h != 3024 || exif == nil {
		t.Fatalf("heifFacts: %d x %d, exif %d bytes", w, h, len(exif))
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "IMG_1.HEIC"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, "IMG_1.HEIC"))
	if err != nil {
		t.Fatal(err)
	}
	it := FromFileInfo(dir, fi)
	LoadImageFacts(&it)
	if d, ok := it.Extra[ExtraExifDate].(time.Time); !ok || !d.Equal(time.Date(2022, 12, 24, 18, 0, 1, 0, time.Local)) {
		t.Errorf("exif date: %v", it.Extra)
	}
	if it.Extra[ExtraImgWidth] != int64(4032) || it.Extra[ExtraImgHeight] != int64(3024) {
		t.Errorf("size: %v", it.Extra)
	}
	// damaged HEIF files give nothing and do not crash
	for _, cut := range []int{12, 40, len(data) / 2, len(data) - 5} {
		heifFacts(bytes.NewReader(data[:cut]))
	}
	if isHEIF([]byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isomiso2")) {
		t.Error("an MP4 video is taken for HEIF")
	}
}
