package listing

// Tests of the FLAC and Ogg tag readers on tiny files made here: a FLAC file (also with an ID3v2 block in front), Ogg
// Vorbis with its comment packet spread over two pages, Ogg Opus, and files that are damaged or have no tags.

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// vorbisComment builds a Vorbis comment block.
func vorbisComment(pairs ...string) []byte {
	var b bytes.Buffer
	le := func(n int) { binary.Write(&b, binary.LittleEndian, uint32(n)) }
	le(len("test vendor"))
	b.WriteString("test vendor")
	le(len(pairs))
	for _, p := range pairs {
		le(len(p))
		b.WriteString(p)
	}
	return b.Bytes()
}

// flacFile builds a FLAC file: STREAMINFO, a padding block, then the comment block (the last block), then "audio".
func flacFile(comment []byte) []byte {
	var b bytes.Buffer
	b.WriteString("fLaC")
	block := func(typ byte, last bool, body []byte) {
		if last {
			typ |= 0x80
		}
		n := len(body)
		b.Write([]byte{typ, byte(n >> 16), byte(n >> 8), byte(n)})
		b.Write(body)
	}
	block(0, false, make([]byte, 34))
	block(1, false, make([]byte, 10))
	block(4, true, comment)
	b.Write(make([]byte, 64))
	return b.Bytes()
}

// oggPage builds one Ogg page holding segs (each segment at most 255 bytes); the checksum is left zero.
func oggPage(serial uint32, seq uint32, segs [][]byte) []byte {
	var b bytes.Buffer
	b.WriteString("OggS")
	b.Write([]byte{0, 0})
	b.Write(make([]byte, 8)) // granule position
	binary.Write(&b, binary.LittleEndian, serial)
	binary.Write(&b, binary.LittleEndian, seq)
	b.Write(make([]byte, 4)) // checksum
	b.WriteByte(byte(len(segs)))
	for _, s := range segs {
		b.WriteByte(byte(len(s)))
	}
	for _, s := range segs {
		b.Write(s)
	}
	return b.Bytes()
}

// lace cuts a packet into segments: 255-byte pieces and a last shorter one (an empty one when the length divides).
func lace(p []byte) [][]byte {
	var out [][]byte
	for len(p) >= 255 {
		out = append(out, p[:255])
		p = p[255:]
	}
	return append(out, p)
}

func TestReadFLACTags(t *testing.T) {
	c := vorbisComment("title=Sång", "ARTIST=Band", "ALBUM=Record", "DATE=2003-05-01", "TRACKNUMBER=4/11", "GENRE=Jazz", "ARTIST=Second")
	want := AudioTags{Title: "Sång", Artist: "Band", Album: "Record", Year: "2003", Track: 4, Genre: "Jazz"}
	got, ok := ReadAudioTags(bytes.NewReader(flacFile(c)))
	if !ok || got != want {
		t.Errorf("FLAC: %+v %v", got, ok)
	}
	// an empty ID3v2 tag in front of the FLAC stream
	id3 := []byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0}
	got, ok = ReadAudioTags(bytes.NewReader(append(id3, flacFile(c)...)))
	if !ok || got != want {
		t.Errorf("FLAC after ID3: %+v %v", got, ok)
	}
	// album artist stands in for a missing artist
	got, _ = ReadAudioTags(bytes.NewReader(flacFile(vorbisComment("ALBUMARTIST=Various", "TITLE=x"))))
	if got.Artist != "Various" {
		t.Errorf("album artist: %+v", got)
	}
	if _, ok := ReadAudioTags(bytes.NewReader(flacFile(vorbisComment()))); ok {
		t.Error("a comment block without tags counts as tags")
	}
	broken := flacFile(c)
	if _, ok := ReadAudioTags(bytes.NewReader(broken[:60])); ok {
		t.Error("a cut file gave tags")
	}
}

func TestReadOggTags(t *testing.T) {
	// Vorbis: the identification packet on page 0, the comment packet (with a long tag) spread over pages 1 and 2
	long := string(bytes.Repeat([]byte("x"), 600))
	ident := append([]byte("\x01vorbis"), make([]byte, 23)...)
	comment := append(append([]byte("\x03vorbis"), vorbisComment("TITLE=Song", "ARTIST=Singer", "COMMENT="+long, "TRACKNUMBER=2")...), 1)
	segs := lace(comment)
	var f []byte
	f = append(f, oggPage(7, 0, [][]byte{ident})...)
	f = append(f, oggPage(99, 0, [][]byte{[]byte("another stream")})...) // a page of another stream in between
	f = append(f, oggPage(7, 1, segs[:2])...)
	f = append(f, oggPage(7, 2, segs[2:])...)
	got, ok := ReadAudioTags(bytes.NewReader(f))
	if !ok || got.Title != "Song" || got.Artist != "Singer" || got.Track != 2 {
		t.Errorf("Ogg Vorbis: %+v %v", got, ok)
	}
	// Opus
	opus := append(oggPage(1, 0, [][]byte{append([]byte("OpusHead"), make([]byte, 11)...)}),
		oggPage(1, 1, lace(append([]byte("OpusTags"), vorbisComment("album=Live", "date=1977")...)))...)
	got, ok = ReadAudioTags(bytes.NewReader(opus))
	if !ok || got.Album != "Live" || got.Year != "1977" {
		t.Errorf("Ogg Opus: %+v %v", got, ok)
	}
	// an unknown codec, and a cut file
	other := append(oggPage(1, 0, [][]byte{[]byte("\x80theora")}), oggPage(1, 1, [][]byte{[]byte("whatever")})...)
	if _, ok := ReadAudioTags(bytes.NewReader(other)); ok {
		t.Error("an unknown codec gave tags")
	}
	if _, ok := ReadAudioTags(bytes.NewReader(f[:40])); ok {
		t.Error("a cut file gave tags")
	}
}

func TestLoadAudioFactsFLAC(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.flac")
	if err := os.WriteFile(p, flacFile(vorbisComment("ARTIST=Band", "TITLE=Song", "TRACKNUMBER=9")), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	it := FromFileInfo(dir, fi)
	LoadAudioFacts(&it)
	if it.Extra[ExtraArtist] != "Band" || it.Extra[ExtraTitle] != "Song" || it.Extra[ExtraTrack] != int64(9) {
		t.Errorf("facts: %+v", it.Extra)
	}
}
