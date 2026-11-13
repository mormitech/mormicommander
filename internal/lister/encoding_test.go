// Tests for the encoding table and the per-character decoders: every
// encoding must round-trip the text it can represent, report the right byte
// size per character, and survive broken input without stalling.

package lister

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

func TestEncodingRoundTrip(t *testing.T) {
	tests := []struct {
		enc  Encoding
		text string
	}{
		{UTF8, "árvíztűrő tükörfúrógép ÁRVÍZTŰRŐ 😀"},
		{UTF16LE, "árvíztűrő tükörfúrógép 😀"},
		{UTF16BE, "árvíztűrő tükörfúrógép 😀"},
		{Win1250, "árvíztűrő tükörfúrógép ÁRVÍZTŰRŐ"},
		{ISO8859_2, "árvíztűrő tükörfúrógép"},
		{DOS852, "árvíztűrő tükörfúrógép"},
		{DOS437, "Hello ÇüéâäàåçêëèïîìÄÅ ░▒▓│┤"},
		{Win1251, "Съешь же ещё этих мягких французских булок"},
		{KOI8R, "Съешь же ещё этих мягких французских булок"},
		{DOS866, "Съешь же ещё этих мягких французских булок"},
		{Win1252, "naïve café façade €"},
		{Win1253, "Ξεσκεπάζω την ψυχοφθόρα βδελυγμία"},
		{MacRoman, "café ƒ π"},
		{ShiftJIS, "日本語のテキスト ｶﾀｶﾅ abc"},
		{EUCJP, "日本語のテキスト abc"},
		{GBK, "中文文本 abc"},
		{GB18030, "中文文本 abc €"},
		{Big5, "中文文本 abc"},
		{EUCKR, "한국어 텍스트 abc"},
	}
	for _, tt := range tests {
		t.Run(string(tt.enc), func(t *testing.T) {
			raw := EncodeString(tt.enc, tt.text)
			if got := DecodeString(tt.enc, raw); got != tt.text {
				t.Fatalf("round trip: got %q want %q", got, tt.text)
			}
			// Decoding character by character must use up exactly the bytes.
			d := newRuneDecoder(tt.enc)
			var runes []rune
			for b := raw; len(b) > 0; {
				r, n := d.decode(b)
				if n <= 0 {
					t.Fatal("decoder made no progress")
				}
				runes = append(runes, r)
				b = b[n:]
			}
			if string(runes) != tt.text {
				t.Fatalf("per-rune decode: got %q", string(runes))
			}
		})
	}
}

func TestDecodersOnGarbage(t *testing.T) {
	garbage := []byte{0xFF, 0xFE, 0x80, 0x00, 0xC3, 0xD8, 0x3D, 0x81, 0x30, 0xE0}
	for _, info := range Encodings() {
		d := newRuneDecoder(info.ID)
		b := garbage
		steps := 0
		for len(b) > 0 {
			_, n := d.decode(b)
			if n <= 0 || n > len(b) {
				t.Fatalf("%s: bad size %d", info.ID, n)
			}
			b = b[n:]
			steps++
		}
		if steps == 0 {
			t.Fatalf("%s: no steps", info.ID)
		}
	}
}

func TestUTF16Surrogates(t *testing.T) {
	d := newRuneDecoder(UTF16LE)
	r, n := d.decode([]byte{0x3D, 0xD8, 0x00, 0xDE}) // U+1F600
	if r != 0x1F600 || n != 4 {
		t.Fatalf("got %U %d", r, n)
	}
	r, n = d.decode([]byte{0x3D, 0xD8, 0x41, 0x00}) // lone high surrogate
	if r != utf8.RuneError || n != 2 {
		t.Fatalf("lone surrogate: got %U %d", r, n)
	}
	r, n = d.decode([]byte{0x41})
	if r != utf8.RuneError || n != 1 {
		t.Fatalf("odd byte: got %U %d", r, n)
	}
}

func TestLookupEncoding(t *testing.T) {
	tests := []struct {
		in   string
		want Encoding
		ok   bool
	}{
		{"UTF-8", UTF8, true},
		{" windows-1250 ", Win1250, true},
		{"cp852", DOS852, true},
		{"1251", Win1251, true},
		{"437", DOS437, true},
		{"Shift_JIS", ShiftJIS, true},
		{"latin2", ISO8859_2, true},
		{"koi8-r", KOI8R, true},
		{"65001", UTF8, true},
		{"klingon", "", false},
		{"99999", "", false},
	}
	for _, tt := range tests {
		got, ok := LookupEncoding(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("LookupEncoding(%q) = %q,%v want %q,%v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestEncodingsTable(t *testing.T) {
	seen := map[Encoding]bool{}
	for _, e := range Encodings() {
		if seen[e.ID] {
			t.Fatalf("duplicate %s", e.ID)
		}
		seen[e.ID] = true
		if e.Name == "" || e.Group == "" {
			t.Fatalf("%s lacks name/group", e.ID)
		}
		if !e.ID.Valid() || e.ID.Info().ID != e.ID {
			t.Fatalf("%s not valid", e.ID)
		}
		if e.SingleByte && tableCache[e.ID] == nil {
			t.Fatalf("%s has no table", e.ID)
		}
	}
	if Encoding("nope").Valid() {
		t.Fatal("unknown encoding reported valid")
	}
}

func TestEncodeUnsupported(t *testing.T) {
	got := EncodeString(Win1252, "a€ő")
	if !bytes.Equal(got, []byte{'a', 0x80, '?'}) {
		t.Fatalf("got %x", got)
	}
}
