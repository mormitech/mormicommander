// Tests for encoding detection, decoding and encoding, including Hungarian
// text in the code pages Total Commander users actually meet.
package compare

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func utf16le(s string) []byte {
	var b []byte
	for _, r := range s {
		b = append(b, byte(r), byte(r>>8))
	}
	return b
}

func utf16be(s string) []byte {
	var b []byte
	for _, r := range s {
		b = append(b, byte(r>>8), byte(r))
	}
	return b
}

func TestDetect(t *testing.T) {
	hu := "árvíztűrő tükörfúrógép"
	hu1250, _ := Encode(hu, EncWindows1250, false)
	tests := []struct {
		name string
		data []byte
		opts DetectOptions
		want Detection
	}{
		{"empty", nil, DetectOptions{}, Detection{Encoding: EncUTF8, ASCII: true}},
		{"ascii", []byte("hello\nworld\n"), DetectOptions{}, Detection{Encoding: EncUTF8, ASCII: true}},
		{"utf8", []byte(hu), DetectOptions{}, Detection{Encoding: EncUTF8}},
		{"utf8 bom", append([]byte{0xEF, 0xBB, 0xBF}, hu...), DetectOptions{}, Detection{Encoding: EncUTF8, BOM: true}},
		{"utf16le bom", append([]byte{0xFF, 0xFE}, utf16le("hi")...), DetectOptions{}, Detection{Encoding: EncUTF16LE, BOM: true}},
		{"utf16be bom", append([]byte{0xFE, 0xFF}, utf16be("hi")...), DetectOptions{}, Detection{Encoding: EncUTF16BE, BOM: true}},
		{"utf32le bom", []byte{0xFF, 0xFE, 0, 0, 'a', 0, 0, 0}, DetectOptions{}, Detection{Encoding: EncUTF32LE, BOM: true}},
		{"utf32be bom", []byte{0, 0, 0xFE, 0xFF, 0, 0, 0, 'a'}, DetectOptions{}, Detection{Encoding: EncUTF32BE, BOM: true}},
		{"utf16le no bom", utf16le("plain text file\r\n"), DetectOptions{}, Detection{Encoding: EncUTF16LE}},
		{"utf16be no bom", utf16be("plain text file\r\n"), DetectOptions{}, Detection{Encoding: EncUTF16BE}},
		{"binary nul", []byte{'M', 'Z', 0x90, 0, 3, 0, 0, 0, 4, 0, 0, 0, 0xFF, 0xFF, 0, 0}, DetectOptions{}, Detection{Encoding: EncWindows1252, Binary: true}},
		{"binary ctrl", bytes.Repeat([]byte{1, 2, 3, 'a'}, 10), DetectOptions{}, Detection{Encoding: EncWindows1252, Binary: true}},
		{"ansi default", []byte("caf\xe9"), DetectOptions{}, Detection{Encoding: EncWindows1252}},
		{"ansi fallback option", hu1250, DetectOptions{ANSI: EncWindows1250}, Detection{Encoding: EncWindows1250}},
		{"1250 by undefined byte", []byte("\x8dest"), DetectOptions{}, Detection{Encoding: EncWindows1250}},
		{"utf8 cut at end", []byte(hu)[:len(hu)-1], DetectOptions{}, Detection{Encoding: EncUTF8}},
		{"tabs and esc are text", []byte("a\tb\x1b[0m\n"), DetectOptions{}, Detection{Encoding: EncUTF8, ASCII: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.data, tc.opts); got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
	if !IsBinary([]byte{0, 1, 2}) || IsBinary([]byte("text")) {
		t.Fatal("IsBinary")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	hu := "Árvíztűrő tükörfúrógép ő ű\r\n"
	for _, e := range []Encoding{EncUTF8, EncUTF16LE, EncUTF16BE, EncUTF32LE, EncUTF32BE, EncWindows1250, EncISO8859_2, EncCP852} {
		for _, bom := range []bool{false, true} {
			if bom && !e.IsUnicode() {
				continue
			}
			data, err := Encode(hu, e, bom)
			if err != nil {
				t.Fatalf("%v: %v", e, err)
			}
			if bom && !bytes.HasPrefix(data, e.BOM()) {
				t.Fatalf("%v: BOM missing", e)
			}
			got, err := Decode(data, e)
			if err != nil || got != hu {
				t.Fatalf("%v bom=%v: got %q, %v", e, bom, got, err)
			}
		}
	}
	// Windows-1252 has no ő: encoding must fail with ErrUnencodable.
	if _, err := Encode("ő", EncWindows1252, false); !errors.Is(err, ErrUnencodable) {
		t.Fatalf("expected ErrUnencodable, got %v", err)
	}
	// Known byte values.
	b, _ := Encode("őű", EncWindows1250, false)
	if !bytes.Equal(b, []byte{0xF5, 0xFB}) {
		t.Fatalf("1250 bytes %x", b)
	}
	b, _ = Encode("─", EncCP437, false)
	if !bytes.Equal(b, []byte{0xC4}) {
		t.Fatalf("437 bytes %x", b)
	}
	// Invalid UTF-8 survives a UTF-8 round trip byte for byte.
	raw := []byte{'a', 0xFF, 'b'}
	s, _ := Decode(raw, EncUTF8)
	back, _ := Encode(s, EncUTF8, false)
	if !bytes.Equal(back, raw) {
		t.Fatal("invalid UTF-8 not preserved")
	}
	// Auto decode.
	s, err := Decode(append([]byte{0xFF, 0xFE}, utf16le("ok")...), EncAuto)
	if err != nil || s != "ok" {
		t.Fatalf("auto decode %q %v", s, err)
	}
}

func TestParseEncoding(t *testing.T) {
	tests := []struct {
		in   string
		want Encoding
		ok   bool
	}{
		{"UTF-8", EncUTF8, true},
		{"utf8", EncUTF8, true},
		{"windows-1250", EncWindows1250, true},
		{"CP1250", EncWindows1250, true},
		{"latin2", EncISO8859_2, true},
		{"iso_8859_2", EncISO8859_2, true},
		{"oem", EncCP437, true},
		{"utf-16be", EncUTF16BE, true},
		{"klingon", EncAuto, false},
	}
	for _, tc := range tests {
		got, err := ParseEncoding(tc.in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("ParseEncoding(%q) = %v, %v", tc.in, got, err)
		}
	}
	for _, e := range Encodings() {
		back, err := ParseEncoding(e.String())
		if err != nil || back != e {
			t.Errorf("round trip %v: %v %v", e, back, err)
		}
	}
	if Encoding(999).String() != "Encoding(999)" {
		t.Error("unknown String")
	}
}

// TestUndefinedBytesRoundTrip: bytes a code page leaves undefined (0x81 in
// Windows-1250/1252) must survive decode, edit and save unchanged.
func TestUndefinedBytesRoundTrip(t *testing.T) {
	var all []byte
	for c := 0x20; c <= 0xFF; c++ {
		all = append(all, byte(c))
	}
	for _, e := range []Encoding{EncWindows1250, EncWindows1252, EncWindows1251} {
		text, err := Decode(all, e)
		if err != nil {
			t.Fatal(err)
		}
		back, err := Encode(text, e, false)
		if err != nil {
			t.Fatalf("%v: %v", e, err)
		}
		if !bytes.Equal(back, all) {
			t.Errorf("%v: round trip changed the bytes", e)
		}
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	data := append([]byte("first line\n"), all...)
	os.WriteFile(p, data, 0o644)
	other := filepath.Join(dir, "b.txt")
	os.WriteFile(other, []byte("x\n"), 0o644)
	s, err := OpenFiles(p, other, Options{}, DetectOptions{ANSI: EncWindows1250})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetLine(Left, 0, "edited"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Left, ""); err != nil {
		t.Fatalf("save after edit: %v", err)
	}
	got, _ := os.ReadFile(p)
	if want := append([]byte("edited\n"), all...); !bytes.Equal(got, want) {
		t.Errorf("saved bytes differ:\n got %q\nwant %q", got, want)
	}
	if _, err := Encode("ő", EncWindows1252, false); !errors.Is(err, ErrUnencodable) {
		t.Errorf("real unencodable text must still fail: %v", err)
	}
}
