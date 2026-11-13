// Tests for "Find text": every option against in-memory data, every encoding, and
// matches that straddle the chunk boundary (the chunk size is shrunk for that).

package find

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/text/encoding/charmap"
)

func utf16le(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

func utf16be(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u>>8), byte(u))
	}
	return b
}

func enc(cm *charmap.Charmap, s string) []byte {
	b, err := cm.NewEncoder().Bytes([]byte(s))
	if err != nil {
		panic(err)
	}
	return b
}

func TestContentMatch(t *testing.T) {
	cases := []struct {
		name string
		opts ContentOptions
		data []byte
		want bool
	}{
		{"plain", ContentOptions{Text: "hello"}, []byte("say hello world"), true},
		{"absent", ContentOptions{Text: "bye"}, []byte("say hello world"), false},
		{"case-insensitive", ContentOptions{Text: "HELLO"}, []byte("say hello"), true},
		{"case-sensitive miss", ContentOptions{Text: "HELLO", CaseSensitive: true}, []byte("say hello"), false},
		{"case-sensitive hit", ContentOptions{Text: "HELLO", CaseSensitive: true}, []byte("say HELLO"), true},
		{"unicode fold", ContentOptions{Text: "ÁRVÍZTŰRŐ"}, []byte("árvíztűrő tükörfúrógép"), true},
		{"whole word hit", ContentOptions{Text: "cat", WholeWords: true}, []byte("the cat sat"), true},
		{"whole word miss", ContentOptions{Text: "cat", WholeWords: true}, []byte("concatenate"), false},
		{"whole word later hit", ContentOptions{Text: "cat", WholeWords: true}, []byte("concat cat."), true},
		{"whole word at start", ContentOptions{Text: "cat", WholeWords: true}, []byte("cat"), true},
		{"whole word underscore", ContentOptions{Text: "cat", WholeWords: true}, []byte("my_cat"), false},
		{"whole word unicode", ContentOptions{Text: "fa", WholeWords: true}, []byte("faé"), false},
		{"whole word punct needle", ContentOptions{Text: "a.", WholeWords: true}, []byte("xa.b"), false},
		{"whole word punct needle ok", ContentOptions{Text: "a.", WholeWords: true}, []byte("x a.b"), true},
		{"regex", ContentOptions{Text: `fo+ba?r`, Regex: true}, []byte("xx foooobr yy"), true},
		{"regex case", ContentOptions{Text: `FOO`, Regex: true}, []byte("foo"), true},
		{"regex case-sensitive", ContentOptions{Text: `FOO`, Regex: true, CaseSensitive: true}, []byte("foo"), false},
		{"regex line anchor", ContentOptions{Text: `^func main`, Regex: true}, []byte("package x\nfunc main() {}"), true},
		{"regex whole word", ContentOptions{Text: `c[a-z]t`, Regex: true, WholeWords: true}, []byte("scatter cut"), true},
		{"regex whole word miss", ContentOptions{Text: `c[a-z]t`, Regex: true, WholeWords: true}, []byte("scatter"), false},
		{"hex", ContentOptions{Text: "4D 5A", Hex: true}, []byte{0, 1, 'M', 'Z', 0x90}, true},
		{"hex miss", ContentOptions{Text: "4D5B", Hex: true}, []byte{0, 1, 'M', 'Z', 0x90}, false},
		{"hex is case-sensitive", ContentOptions{Text: "6d", Hex: true}, []byte("M"), false},
		{"utf16le", ContentOptions{Text: "Grüße", Encodings: EncUTF16LE}, append([]byte{0xFF, 0xFE}, utf16le("Viele Grüße!")...), true},
		{"utf16le odd offset", ContentOptions{Text: "secret", Encodings: EncUTF16LE}, append([]byte{7}, utf16le("a secret")...), true},
		{"utf16be", ContentOptions{Text: "grüße", Encodings: EncUTF16BE}, utf16be("Viele GRÜSSE grüße"), true},
		{"utf16le not selected", ContentOptions{Text: "secret", Encodings: EncUTF8}, utf16le("a secret"), false},
		{"utf16 surrogate", ContentOptions{Text: "😀x", Encodings: EncUTF16LE}, utf16le("hi 😀x"), true},
		{"ansi 1252", ContentOptions{Text: "café", Encodings: EncANSI}, enc(charmap.Windows1252, "un café noir"), true},
		{"ansi 1250", ContentOptions{Text: "tűz", Encodings: EncANSI, ANSICodepage: "windows-1250"}, enc(charmap.Windows1250, "a tűz ég"), true},
		{"ansi 1252 not utf8", ContentOptions{Text: "café", Encodings: EncUTF8}, enc(charmap.Windows1252, "un café noir"), false},
		{"oem 437", ContentOptions{Text: "Größe", Encodings: EncOEM}, enc(charmap.CodePage437, "Die Größe"), true},
		{"oem 852", ContentOptions{Text: "ő", Encodings: EncOEM, OEMCodepage: "852"}, enc(charmap.CodePage852, "nő"), true},
		{"utf8 default", ContentOptions{Text: "ünïcödé"}, []byte("some ünïcödé text"), true},
		{"empty file", ContentOptions{Text: "x"}, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := compileContent(c.opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := m.matchBytes(c.data); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestContentCompileErrors(t *testing.T) {
	for _, o := range []ContentOptions{
		{Text: "zz", Hex: true},
		{Text: "4D", Hex: true, Regex: true},
		{Text: "(", Regex: true},
		{Text: "x", Encodings: EncANSI, ANSICodepage: "klingon"},
		{Text: " ", Hex: true},
	} {
		if _, err := compileContent(o); err == nil {
			t.Errorf("%+v: want error", o)
		}
	}
}

// TestChunkBoundaries places the needle at every offset around a tiny chunk size, so
// every split of the needle, of a multi-byte character and of a UTF-16 unit is tried.
func TestChunkBoundaries(t *testing.T) {
	old, oldRe := chunkSize, regexOverlap
	chunkSize, regexOverlap = 16, 12
	defer func() { chunkSize, regexOverlap = old, oldRe }()

	type tc struct {
		name   string
		opts   ContentOptions
		build  func(prefix int) []byte
		expect bool
	}
	pad := func(n int) string { return strings.Repeat(" ", n) }
	cases := []tc{
		{"literal", ContentOptions{Text: "needle"}, func(p int) []byte { return []byte(pad(p) + "needle" + pad(20)) }, true},
		{"utf8 multibyte", ContentOptions{Text: "ŐŰé", Encodings: EncUTF8, CaseSensitive: true}, func(p int) []byte { return []byte(pad(p) + "ŐŰé" + pad(20)) }, true},
		{"fold multibyte", ContentOptions{Text: "őűé", Encodings: EncUTF8}, func(p int) []byte { return []byte(pad(p) + "ŐŰÉ" + pad(3)) }, true},
		{"whole word true", ContentOptions{Text: "word", WholeWords: true}, func(p int) []byte { return []byte(pad(p) + "word" + pad(5)) }, true},
		{"whole word false left", ContentOptions{Text: "word", WholeWords: true}, func(p int) []byte { return []byte(strings.Repeat("x", p+1) + "word" + pad(5)) }, false},
		{"whole word false right", ContentOptions{Text: "word", WholeWords: true}, func(p int) []byte { return []byte(pad(p) + "wordy" + pad(5)) }, false},
		{"whole word at eof", ContentOptions{Text: "word", WholeWords: true}, func(p int) []byte { return []byte(pad(p) + "word") }, true},
		{"utf16", ContentOptions{Text: "needle", Encodings: EncUTF16LE}, func(p int) []byte { return append([]byte(pad(p)), utf16le("needle   ")...) }, true},
		{"utf16be", ContentOptions{Text: "needle", Encodings: EncUTF16BE}, func(p int) []byte { return append([]byte(pad(p)), utf16be("needle   ")...) }, true},
		{"hex", ContentOptions{Text: "DE AD BE EF", Hex: true}, func(p int) []byte { return append(append([]byte(pad(p)), 0xDE, 0xAD, 0xBE, 0xEF), pad(3)...) }, true},
		{"regex", ContentOptions{Text: `ne+dle`, Regex: true}, func(p int) []byte { return []byte(pad(p) + "needle" + pad(2)) }, true},
		{"regex whole word false", ContentOptions{Text: `ne+dle`, Regex: true, WholeWords: true}, func(p int) []byte { return []byte(pad(p) + "needles" + pad(2)) }, false},
	}
	for _, c := range cases {
		m, err := compileContent(c.opts)
		if err != nil {
			t.Fatal(err)
		}
		for p := 0; p < 40; p++ {
			if got := m.matchBytes(c.build(p)); got != c.expect {
				t.Errorf("%s prefix %d: got %v want %v", c.name, p, got, c.expect)
			}
		}
	}
}

func TestContentCancel(t *testing.T) {
	m, _ := compileContent(ContentOptions{Text: "x"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.matchReader(ctx, bytes.NewReader([]byte("x"))); err == nil {
		t.Error("want context error")
	}
}

func TestParseHex(t *testing.T) {
	cases := map[string][]byte{
		"4D 5A":       {0x4d, 0x5a},
		"4d5a":        {0x4d, 0x5a},
		"0x4D,0x5A":   {0x4d, 0x5a},
		"A":           {0x0a},
		"de-ad:be ef": {0xde, 0xad, 0xbe, 0xef},
	}
	for in, want := range cases {
		got, err := ParseHex(in)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("ParseHex(%q) = %x, %v; want %x", in, got, err, want)
		}
	}
}

func TestDecoders(t *testing.T) {
	// UTF-8 split in the middle of a character, byte by byte.
	d := &utf8Decoder{}
	in := []byte("aé😀b")
	var out strings.Builder
	for i := range in {
		out.WriteString(d.decode(in[i:i+1], i == len(in)-1))
	}
	if out.String() != "aé😀b" {
		t.Errorf("utf8 decoder: %q", out.String())
	}
	// UTF-16 split everywhere, including between surrogates.
	u := &utf16Decoder{}
	raw := utf16le("x😀y")
	out.Reset()
	for i := range raw {
		out.WriteString(u.decode(raw[i:i+1], i == len(raw)-1))
	}
	if out.String() != "x😀y" {
		t.Errorf("utf16 decoder: %q", out.String())
	}
	// Invalid UTF-8 becomes U+FFFD.
	if got := (&utf8Decoder{}).decode([]byte{'a', 0xff, 'b'}, true); got != "a�b" {
		t.Errorf("invalid utf8: %q", got)
	}
}

func TestCodepageNames(t *testing.T) {
	for in, want := range map[string]string{
		"1252": "windows-1252", "CP1250": "windows-1250", "ibm437": "cp437", "852": "cp852",
		"ISO8859-2": "iso-8859-2", " KOI8_R ": "koi8-r", "windows-1251": "windows-1251",
	} {
		if got := normCodepage(in); got != want {
			t.Errorf("normCodepage(%q) = %q, want %q", in, got, want)
		}
	}
	names := Codepages()
	if len(names) < 30 || names[0] > names[len(names)-1] {
		t.Errorf("Codepages: %v", names)
	}
}
