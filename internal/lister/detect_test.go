// Tests for encoding detection, binary detection, the view suggestion and
// the locale-based default code pages.

package lister

import (
	"bytes"
	"testing"
)

func TestDetect(t *testing.T) {
	hu := "árvíztűrő tükörfúrógép, árvíztűrő tükörfúrógép\n"
	ru := "Съешь же ещё этих мягких французских булок, да выпей чаю. Это обычный текст.\n"
	tests := []struct {
		name   string
		data   []byte
		ansi   Encoding
		want   Encoding
		bom    int
		binary bool
	}{
		{"empty", nil, Win1252, UTF8, 0, false},
		{"ascii", []byte("hello world\n"), Win1250, UTF8, 0, false},
		{"utf8", []byte(hu), Win1250, UTF8, 0, false},
		{"utf8 bom", append([]byte{0xEF, 0xBB, 0xBF}, hu...), Win1250, UTF8, 3, false},
		{"utf16le bom", append([]byte{0xFF, 0xFE}, EncodeString(UTF16LE, hu)...), Win1250, UTF16LE, 2, false},
		{"utf16be bom", append([]byte{0xFE, 0xFF}, EncodeString(UTF16BE, hu)...), Win1250, UTF16BE, 2, false},
		{"utf16le no bom", EncodeString(UTF16LE, "plain ascii text in utf16\n"), Win1252, UTF16LE, 0, false},
		{"utf16be no bom", EncodeString(UTF16BE, "plain ascii text in utf16\n"), Win1252, UTF16BE, 0, false},
		{"ansi 1250", EncodeString(Win1250, hu), Win1250, Win1250, 0, false},
		{"utf8 cut at end", []byte("abc é")[:5], Win1252, UTF8, 0, false},
		{"binary nul", []byte("MZ\x90\x00\x03\x00\x00\x00"), Win1252, Win1252, 0, true},
		{"cyrillic 1251", EncodeString(Win1251, ru), Win1251, Win1251, 0, false},
		{"cyrillic koi8 on 1251 system", EncodeString(KOI8R, ru), Win1251, KOI8R, 0, false},
		{"cyrillic 866 on 1251 system", EncodeString(DOS866, ru), Win1251, DOS866, 0, false},
		{"invalid fallback", []byte{0xE9, 'a'}, "bogus", Win1252, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Detect(tt.data, tt.ansi)
			if d.Encoding != tt.want || d.BOMLen != tt.bom || d.Binary != tt.binary {
				t.Fatalf("got %+v want enc=%s bom=%d binary=%v", d, tt.want, tt.bom, tt.binary)
			}
		})
	}
}

func TestLooksBinary(t *testing.T) {
	tests := []struct {
		data []byte
		want bool
	}{
		{nil, false},
		{[]byte("text\twith\r\ntabs\f"), false},
		{[]byte("a\x00b"), true},
		{bytes.Repeat([]byte{1, 2, 3, 'a'}, 10), true},
		{append(bytes.Repeat([]byte("a"), 95), 1, 2, 3, 4, 5), false},
	}
	for i, tt := range tests {
		if got := LooksBinary(tt.data); got != tt.want {
			t.Errorf("%d: got %v", i, got)
		}
	}
}

func TestSuggestView(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want ViewKind
	}{
		{"a.txt", []byte("hello"), ViewText},
		{"a.png", []byte("\x89PNG\r\n\x1a\n...."), ViewMedia},
		{"noext", []byte("\xff\xd8\xff\xe0"), ViewMedia},
		{"page.html", []byte("<p>x"), ViewHTML},
		{"page.txt", []byte("<!DOCTYPE html><html>"), ViewHTML},
		{"doc.rtf", []byte(`{\rtf1 x}`), ViewRTF},
		{"doc.dat", []byte(`{\rtf1 x}`), ViewRTF},
		{"README.md", []byte("# x"), ViewMarkdown},
		{"prog.exe", []byte("MZ\x90\x00\x03\x00"), ViewBinary},
		{"u16.txt", EncodeString(UTF16LE, "hello there"), ViewText},
		{"pic.svg", []byte(`<?xml version="1.0"?><svg width="10" height="20"></svg>`), ViewMedia},
	}
	for _, tt := range tests {
		if got := SuggestView(tt.name, tt.data); got != tt.want {
			t.Errorf("%s: got %v want %v", tt.name, got, tt.want)
		}
	}
	if ViewHex.String() != "hex" || ModeBinary.String() != "binary" {
		t.Fatal("String() names")
	}
}

func TestCodepagesForLocale(t *testing.T) {
	tests := []struct {
		loc       string
		ansi, oem Encoding
	}{
		{"hu_HU.UTF-8", Win1250, DOS852},
		{"ru_RU.KOI8-R", Win1251, DOS866},
		{"en_US.UTF-8", Win1252, DOS437},
		{"", Win1252, DOS437},
		{"zh_TW.UTF-8", Big5, Big5},
		{"zh_CN", GBK, GBK},
		{"pt_BR.UTF-8", Win1252, DOS850},
		{"de_DE@euro", Win1252, DOS850},
		{"ja_JP.eucJP", ShiftJIS, ShiftJIS},
		{"xx_YY", Win1252, DOS437},
	}
	for _, tt := range tests {
		a, o := codepagesForLocale(tt.loc)
		if a != tt.ansi || o != tt.oem {
			t.Errorf("%q: got %s/%s want %s/%s", tt.loc, a, o, tt.ansi, tt.oem)
		}
	}
	if !DefaultANSI().Valid() || !DefaultOEM().Valid() {
		t.Fatal("defaults must be valid encodings")
	}
}

func TestRuneWidth(t *testing.T) {
	tests := []struct {
		r    rune
		want int
	}{
		{'a', 1}, {'ő', 1}, {'日', 2}, {'한', 2}, {'😀', 2}, {0x0301, 0}, {'Ж', 1}, {'Ａ', 2},
	}
	for _, tt := range tests {
		if got := RuneWidth(tt.r); got != tt.want {
			t.Errorf("%q: got %d want %d", tt.r, got, tt.want)
		}
	}
	if StringWidth("日本a") != 5 {
		t.Fatal("StringWidth")
	}
}

func TestEncodingForKey(t *testing.T) {
	for _, k := range []rune{'A', 'a', 'S', 'U', '8'} {
		e, ok := EncodingForKey(k)
		if !ok || !e.Valid() {
			t.Fatalf("key %q: %q %v", k, e, ok)
		}
	}
	if e, _ := EncodingForKey('U'); e != UTF16LE {
		t.Fatal("U must be UTF-16")
	}
	if _, ok := EncodingForKey('x'); ok {
		t.Fatal("x accepted")
	}
}
