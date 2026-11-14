// This file reads the tags of FLAC and Ogg (Vorbis, Opus, FLAC in Ogg) audio files, which carry Vorbis comments
// ("ARTIST=...", "TITLE=...") instead of ID3 frames, so the Music columns and the Multi-Rename tool's [=audio.*]
// fields work for them as they do for MP3 files. Both readers are small and read only the start of the file: a FLAC
// file keeps its tags in a metadata block before the audio, an Ogg file in its second packet. The Ogg page checksums
// are not verified: a damaged file gives wrong or no tags, never an error, which is all a column needs.

package listing

import (
	"bytes"
	"encoding/binary"
	"io"
	"strconv"
	"strings"
)

// maxCommentBlock is the largest tag block read. Tags with embedded cover art can be big, but not bigger than this.
const maxCommentBlock = 16 << 20

// ReadAudioTags reads the tags of an audio file from r: the Vorbis comments of a FLAC or Ogg file, else ID3 (v2 at
// the start, v1 at the end). A FLAC file with an ID3v2 block in front, which some taggers write, is read either way.
func ReadAudioTags(r io.ReadSeeker) (AudioTags, bool) {
	head := make([]byte, 10)
	n, _ := io.ReadFull(r, head)
	head = head[:n]
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return AudioTags{}, false
	}
	switch {
	case bytes.HasPrefix(head, []byte("fLaC")):
		return readFLACTags(r)
	case bytes.HasPrefix(head, []byte("OggS")):
		return readOggTags(r)
	}
	if t, ok := ReadID3(r); ok {
		return t, true
	}
	if len(head) == 10 && string(head[:3]) == "ID3" {
		skip := int64(10 + syncsafe(head[6:10]))
		if head[5]&0x10 != 0 { // a footer follows the tag
			skip += 10
		}
		if _, err := r.Seek(skip, io.SeekStart); err == nil {
			magic := make([]byte, 4)
			if _, err := io.ReadFull(r, magic); err == nil && string(magic) == "fLaC" {
				if _, err := r.Seek(skip, io.SeekStart); err == nil {
					return readFLACTags(r)
				}
			}
		}
	}
	return AudioTags{}, false
}

// readFLACTags reads the VORBIS_COMMENT metadata block of a FLAC stream that starts at r's position.
func readFLACTags(r io.ReadSeeker) (AudioTags, bool) {
	var t AudioTags
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != "fLaC" {
		return t, false
	}
	hdr := make([]byte, 4)
	for i := 0; i < 128; i++ { // (a real file has a handful of blocks)
		if _, err := io.ReadFull(r, hdr); err != nil {
			return t, false
		}
		last, typ := hdr[0]&0x80 != 0, hdr[0]&0x7f
		size := int64(hdr[1])<<16 | int64(hdr[2])<<8 | int64(hdr[3])
		if typ == 4 { // VORBIS_COMMENT
			if size > maxCommentBlock {
				return t, false
			}
			body := make([]byte, size)
			if _, err := io.ReadFull(r, body); err != nil {
				return t, false
			}
			ok := parseVorbisComment(body, &t)
			return t, ok
		}
		if typ == 127 || last { // invalid, or no comment block before the audio
			return t, false
		}
		if _, err := r.Seek(size, io.SeekCurrent); err != nil {
			return t, false
		}
	}
	return t, false
}

// readOggTags reads the comment packet of the first logical stream of an Ogg file: the second packet, after the
// identification packet that says which codec the stream holds.
func readOggTags(r io.Reader) (AudioTags, bool) {
	var t AudioTags
	var packets [][]byte
	var cur []byte
	var serial uint32
	total := 0
	hdr := make([]byte, 27)
	for page := 0; len(packets) < 2 && page < 1024; page++ {
		if _, err := io.ReadFull(r, hdr); err != nil || string(hdr[:4]) != "OggS" {
			return t, false
		}
		s := binary.LittleEndian.Uint32(hdr[14:18])
		if page == 0 {
			serial = s
		}
		lacing := make([]byte, hdr[26])
		if _, err := io.ReadFull(r, lacing); err != nil {
			return t, false
		}
		size := 0
		for _, l := range lacing {
			size += int(l)
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(r, data); err != nil {
			return t, false
		}
		if s != serial {
			continue // a page of another stream of a multiplexed file
		}
		total += size
		if total > maxCommentBlock {
			return t, false
		}
		// a packet ends at the first lacing value under 255; 255 means it goes on (also on the next page)
		p := 0
		for _, l := range lacing {
			cur = append(cur, data[p:p+int(l)]...)
			p += int(l)
			if l < 255 {
				packets = append(packets, cur)
				cur = nil
				if len(packets) == 2 {
					break
				}
			}
		}
	}
	if len(packets) < 2 {
		return t, false
	}
	id, c := packets[0], packets[1]
	switch {
	case bytes.HasPrefix(id, []byte("\x01vorbis")) && bytes.HasPrefix(c, []byte("\x03vorbis")):
		c = c[7:]
	case bytes.HasPrefix(id, []byte("OpusHead")) && bytes.HasPrefix(c, []byte("OpusTags")):
		c = c[8:]
	case bytes.HasPrefix(id, []byte("\x7fFLAC")) && len(c) >= 4 && c[0]&0x7f == 4:
		c = c[4:] // FLAC in Ogg: the packet is a metadata block with its 4-byte header
	default:
		return t, false
	}
	ok := parseVorbisComment(c, &t)
	return t, ok
}

// parseVorbisComment reads a Vorbis comment block (little-endian lengths: the vendor string, then "KEY=value" pairs)
// into t. Keys are case-insensitive; the first value of a key wins. It says whether a tag was found.
func parseVorbisComment(b []byte, t *AudioTags) bool {
	if len(b) < 8 {
		return false
	}
	vendor := int(binary.LittleEndian.Uint32(b))
	if vendor < 0 || 4+vendor+4 > len(b) {
		return false
	}
	p := 4 + vendor
	count := int(binary.LittleEndian.Uint32(b[p:]))
	p += 4
	var albumArtist string
	found := false
	for i := 0; i < count && p+4 <= len(b); i++ {
		n := int(binary.LittleEndian.Uint32(b[p:]))
		p += 4
		if n < 0 || p+n > len(b) {
			break
		}
		key, val, ok := strings.Cut(string(b[p:p+n]), "=")
		p += n
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		set := func(dst *string) {
			if *dst == "" {
				*dst = val
				found = true
			}
		}
		switch strings.ToUpper(key) {
		case "TITLE":
			set(&t.Title)
		case "ARTIST":
			set(&t.Artist)
		case "ALBUMARTIST", "ALBUM ARTIST":
			if albumArtist == "" {
				albumArtist = val
			}
		case "ALBUM":
			set(&t.Album)
		case "DATE", "YEAR":
			if len(val) > 4 {
				val = val[:4]
			}
			set(&t.Year)
		case "TRACKNUMBER":
			if t.Track == 0 {
				n, _, _ := strings.Cut(val, "/")
				if v, err := strconv.Atoi(strings.TrimSpace(n)); err == nil && v > 0 {
					t.Track = v
					found = true
				}
			}
		case "GENRE":
			set(&t.Genre)
		}
	}
	if t.Artist == "" && albumArtist != "" {
		t.Artist = albumArtist
		found = true
	}
	return found
}
