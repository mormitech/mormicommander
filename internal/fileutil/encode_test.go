// Tests for encoding and decoding files: byte-identical round trips for every
// format and many sizes, known encodings from the classic tools, split
// output, MIME mails with several attachments, BinHex with run-length edge
// cases, and the safety rules for names taken from encoded data.

package fileutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func randomData(n int, seed int64) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(b)
	return b
}

func TestLineCodecKnown(t *testing.T) {
	if got := string(uuCodec.encodeLine(nil, []byte("Cat"))); got != "#0V%T" {
		t.Errorf("UUE(Cat) = %q", got)
	}
	if got := string(xxCodec.encodeLine(nil, []byte("Cat"))); got != "1Eq3o" {
		t.Errorf("XXE(Cat) = %q", got)
	}
	for _, c := range []*lineCodec{uuCodec, xxCodec} {
		for n := 0; n <= 45; n++ {
			in := randomData(n, int64(n))
			line := string(c.encodeLine(nil, in))
			out, k, ok := c.decodeLine(nil, line)
			if !ok || k != n || !bytes.Equal(out, in) {
				t.Errorf("codec %q len %d: round trip failed", c.table[:2], n)
			}
		}
	}
	// Old encoders used a space for zero and some transports strip trailing
	// spaces; both must still decode.
	if out, _, ok := uuCodec.decodeLine(nil, "#0V%T"); !ok || string(out) != "Cat" {
		t.Error("plain UUE line not decoded")
	}
	line := string(uuCodec.encodeLine(nil, []byte{'C', 0, 0}))
	short := strings.TrimRight(strings.ReplaceAll(line, "`", " "), " ")
	if out, _, ok := uuCodec.decodeLine(nil, short); !ok || !bytes.Equal(out, []byte{'C', 0, 0}) {
		t.Errorf("stripped UUE line %q not decoded", short)
	}
	if _, _, ok := xxCodec.decodeLine(nil, "hello world!"); ok {
		t.Error("junk accepted as XXE")
	}
}

func encodeDecode(t *testing.T, enc Encoding, data []byte, lines int) ([]string, *DecodeResult) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "data.bin")
	os.WriteFile(src, data, 0o644)
	er, err := EncodeFile(context.Background(), EncodeOptions{Source: src, Encoding: enc, LinesPerPart: lines, Format: FormatUnix})
	if err != nil {
		t.Fatalf("%v encode: %v", enc, err)
	}
	out := t.TempDir()
	dr, err := DecodeFiles(context.Background(), DecodeOptions{Sources: er.Files[:1], TargetDir: out})
	if err != nil {
		t.Fatalf("%v decode: %v", enc, err)
	}
	if len(dr.Files) != 1 {
		t.Fatalf("%v: decoded %d files", enc, len(dr.Files))
	}
	got, _ := os.ReadFile(dr.Files[0].Path)
	if !bytes.Equal(got, data) {
		t.Fatalf("%v len %d: decoded data differs", enc, len(data))
	}
	if filepath.Base(dr.Files[0].Path) != "data.bin" || dr.Files[0].Size != int64(len(data)) {
		t.Errorf("%v: decoded file %+v", enc, dr.Files[0])
	}
	return er.Files, dr
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	sizes := []int{0, 1, 2, 3, 44, 45, 46, 57, 76, 1000, 70000}
	for _, enc := range []Encoding{EncodingMIME, EncodingUUE, EncodingXXE, EncodingBinHex, EncodingBase64} {
		for _, n := range sizes {
			if enc == EncodingBase64 && n == 0 {
				continue // bare base64 of nothing is an empty file: nothing to find
			}
			t.Run(fmt.Sprintf("%v_%d", enc, n), func(t *testing.T) {
				_, dr := encodeDecode(t, enc, randomData(n, int64(n)), 0)
				want := enc
				if enc == EncodingUUE && n == 0 {
					// An empty block has no data line to tell UUE from XXE.
					want = EncodingUUE
				}
				if dr.Files[0].Encoding != want {
					t.Errorf("detected %v, want %v", dr.Files[0].Encoding, want)
				}
				if len(dr.Warnings) != 0 {
					t.Errorf("warnings: %v", dr.Warnings)
				}
			})
		}
	}
}

func TestEncodedLayout(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "cat.txt")
	os.WriteFile(src, []byte("Cat"), 0o640)
	for _, tt := range []struct {
		enc  Encoding
		want string
	}{
		{EncodingUUE, "begin 640 cat.txt\n#0V%T\n`\nend\n"},
		{EncodingXXE, "begin 640 cat.txt\n1Eq3o\n+\nend\n"},
		{EncodingMIME, "MIME-Version: 1.0\nContent-Type: application/octet-stream; name=cat.txt\nContent-Transfer-Encoding: base64\nContent-Disposition: attachment; filename=cat.txt\n\nQ2F0\n"},
	} {
		res, err := EncodeFile(context.Background(), EncodeOptions{Source: src, Encoding: tt.enc, Format: FormatUnix})
		if err != nil {
			t.Fatal(err)
		}
		want := tt.want
		if os.PathSeparator == '\\' {
			want = strings.Replace(want, "640", "644", 1)
		}
		if got := readFile(t, res.Files[0]); got != want {
			t.Errorf("%v:\n%q\nwant\n%q", tt.enc, got, want)
		}
		if res.Files[0] != src+tt.enc.Extension() {
			t.Errorf("%v: default name %s", tt.enc, res.Files[0])
		}
	}
	// Windows layout uses CRLF.
	res, err := EncodeFile(context.Background(), EncodeOptions{Source: src, Encoding: EncodingUUE, Format: FormatWindows, Target: filepath.Join(dir, "w.uue")})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, res.Files[0]); !strings.HasSuffix(got, "#0V%T\r\n`\r\nend\r\n") {
		t.Errorf("CRLF layout: %q", got)
	}
	// And decodes fine.
	dr, err := DecodeFiles(context.Background(), DecodeOptions{Sources: res.Files, TargetDir: t.TempDir()})
	if err != nil || readFile(t, dr.Files[0].Path) != "Cat" {
		t.Errorf("CRLF decode: %v", err)
	}
}

func TestEncodeSplitParts(t *testing.T) {
	data := randomData(45*25+7, 9)
	for _, enc := range []Encoding{EncodingUUE, EncodingXXE} {
		files, _ := encodeDecode(t, enc, data, 10)
		if len(files) != 3 {
			t.Fatalf("%v: parts = %v", enc, files)
		}
		if !strings.HasSuffix(files[0], "data.bin.001"+enc.Extension()) || !strings.HasSuffix(files[2], "data.bin.003"+enc.Extension()) {
			t.Errorf("part names %v", files)
		}
		if !strings.HasPrefix(readFile(t, files[0]), "begin ") || !strings.HasSuffix(readFile(t, files[2]), "end\n") {
			t.Error("begin/end not in first/last part")
		}
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "x")
	os.WriteFile(src, data, 0o644)
	if _, err := EncodeFile(context.Background(), EncodeOptions{Source: src, Encoding: EncodingMIME, LinesPerPart: 5}); err == nil {
		t.Error("MIME split accepted")
	}
	if _, err := EncodeFile(context.Background(), EncodeOptions{Source: src, Encoding: EncodingUUE, LinesPerPart: -1}); err == nil {
		t.Error("negative lines accepted")
	}
	if _, err := EncodeFile(context.Background(), EncodeOptions{Source: src, Encoding: Encoding(42)}); err == nil {
		t.Error("unknown encoding accepted")
	}
	if _, err := EncodeFile(context.Background(), EncodeOptions{Source: src, Encoding: EncodingUUE, Target: src}); err == nil {
		t.Error("overwriting the source accepted")
	}
	EncodeFile(context.Background(), EncodeOptions{Source: src, Encoding: EncodingUUE})
	if _, err := EncodeFile(context.Background(), EncodeOptions{Source: src, Encoding: EncodingUUE}); !errors.Is(err, os.ErrExist) {
		t.Errorf("second encode err = %v", err)
	}
}

func TestDecodePartsWithJunkAndExplicitOrder(t *testing.T) {
	data := randomData(45*30, 3)
	dir := t.TempDir()
	src := filepath.Join(dir, "pic.jpg")
	os.WriteFile(src, data, 0o644)
	er, err := EncodeFile(context.Background(), EncodeOptions{Source: src, Encoding: EncodingUUE, LinesPerPart: 12, Format: FormatUnix})
	if err != nil {
		t.Fatal(err)
	}
	// Wrap every part in news headers and a signature, as posted parts are.
	var renamed []string
	for i, f := range er.Files {
		body := readFile(t, f)
		wrapped := fmt.Sprintf("From: someone\nSubject: pic.jpg (%d/%d)\n\n%s\n-- \nsig line\n", i+1, len(er.Files), body)
		p := filepath.Join(dir, fmt.Sprintf("post%d.txt", i+1))
		os.WriteFile(p, []byte(wrapped), 0o644)
		renamed = append(renamed, p)
	}
	out := t.TempDir()
	dr, err := DecodeFiles(context.Background(), DecodeOptions{Sources: renamed, TargetDir: out})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(out, "pic.jpg")); !bytes.Equal(got, data) {
		t.Fatal("decoded data differs")
	}
	if len(dr.Warnings) == 0 || !strings.Contains(dr.Warnings[0], "skipped") {
		t.Errorf("expected a skipped-lines warning, got %v", dr.Warnings)
	}

	// Leaving out the last part gives a clear warning.
	out2 := t.TempDir()
	dr, err = DecodeFiles(context.Background(), DecodeOptions{Sources: er.Files[:2], TargetDir: out2})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range dr.Warnings {
		found = found || strings.Contains(w, "without an \"end\" line")
	}
	if !found {
		t.Errorf("warnings = %v", dr.Warnings)
	}
}

func TestDecodeMultipleBlocksAndSafeNames(t *testing.T) {
	a := string(uuCodec.encodeLine(nil, []byte("one")))
	b := string(xxCodec.encodeLine(nil, []byte("two")))
	text := "Some text first\n" +
		"begin 644 ../../escape.txt\n" + a + "\n`\nend\n" +
		"between\n" +
		"begin 644 /etc/same.txt\n" + b + "\n+\nend\n" +
		"begin 600 C:\\x\\same.txt\n" + a + "\n`\nend\n"
	dir := t.TempDir()
	p := filepath.Join(dir, "mail.txt")
	os.WriteFile(p, []byte(text), 0o644)
	out := t.TempDir()
	dr, err := DecodeFiles(context.Background(), DecodeOptions{Sources: []string{p}, TargetDir: out})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name, content string
		enc           Encoding
	}{{"escape.txt", "one", EncodingUUE}, {"same.txt", "two", EncodingXXE}, {"same (2).txt", "one", EncodingUUE}}
	if len(dr.Files) != len(want) {
		t.Fatalf("files = %+v", dr.Files)
	}
	for i, w := range want {
		f := dr.Files[i]
		if f.Path != filepath.Join(out, w.name) || readFile(t, f.Path) != w.content || f.Encoding != w.enc {
			t.Errorf("file %d = %+v, want %v", i, f, w)
		}
	}
}

func TestDecodeMIME(t *testing.T) {
	mail := "From: a@example.com\r\n" +
		"To: b@example.com\r\n" +
		"Subject: files\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"XYZ\"\r\n" +
		"\r\n" +
		"preamble\r\n" +
		"--XYZ\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Hello, the files are attached.\r\n" +
		"--XYZ\r\n" +
		"Content-Type: multipart/alternative; boundary=\"inner\"\r\n" +
		"\r\n" +
		"--inner\r\n" +
		"Content-Type: application/octet-stream; name=\"nested.bin\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"AAEC\r\n" +
		"--inner--\r\n" +
		"--XYZ\r\n" +
		"Content-Type: application/octet-stream\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: attachment; filename*=UTF-8''%C3%A1rv%C3%ADz.txt\r\n" +
		"\r\n" +
		"SGVs bG8s\r\n" +
		"IHdvcmxk\r\n" +
		"--XYZ\r\n" +
		"Content-Type: text/plain; name=\"=?UTF-8?B?bm90ZS50eHQ=?=\"\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"Content-Disposition: attachment\r\n" +
		"\r\n" +
		"caf=C3=A9 =3D ok=\r\n" +
		"!\r\n" +
		"--XYZ\r\n" +
		"Content-Type: application/octet-stream; name=\"old.bin\"\r\n" +
		"Content-Transfer-Encoding: x-uuencode\r\n" +
		"\r\n" +
		"begin 644 old.bin\r\n#0V%T\r\n`\r\nend\r\n" +
		"--XYZ--\r\n"
	dir := t.TempDir()
	p := filepath.Join(dir, "mail.eml")
	os.WriteFile(p, []byte(mail), 0o644)
	out := t.TempDir()
	dr, err := DecodeFiles(context.Background(), DecodeOptions{Sources: []string{p}, TargetDir: out})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"nested.bin": "\x00\x01\x02", "árvíz.txt": "Hello, world", "note.txt": "café = ok!", "old.bin": "Cat"}
	if len(dr.Files) != len(want) {
		t.Fatalf("files = %+v", dr.Files)
	}
	for name, content := range want {
		if got := readFile(t, filepath.Join(out, name)); got != content {
			t.Errorf("%s = %q, want %q", name, got, content)
		}
	}
}

func TestDecodeMIMEWithoutAttachmentFallsBackToUUE(t *testing.T) {
	mail := "Subject: old style\nContent-Type: text/plain\n\nHere it is:\nbegin 644 cat.txt\n#0V%T\n`\nend\n"
	dir := t.TempDir()
	p := filepath.Join(dir, "m.eml")
	os.WriteFile(p, []byte(mail), 0o644)
	dr, err := DecodeFiles(context.Background(), DecodeOptions{Sources: []string{p}})
	if err != nil || len(dr.Files) != 1 || readFile(t, dr.Files[0].Path) != "Cat" {
		t.Fatalf("%v %+v", err, dr)
	}
}

func TestDecodeBase64Variants(t *testing.T) {
	dir := t.TempDir()
	// uuencode -m style block.
	p := filepath.Join(dir, "m.txt")
	os.WriteFile(p, []byte("begin-base64 644 hello.txt\nSGVsbG8s\nIHdvcmxk\n====\n"), 0o644)
	dr, err := DecodeFiles(context.Background(), DecodeOptions{Sources: []string{p}})
	if err != nil || readFile(t, filepath.Join(dir, "hello.txt")) != "Hello, world" || dr.Files[0].Encoding != EncodingBase64 {
		t.Fatalf("begin-base64: %v %+v", err, dr)
	}
	// Bare base64 text: the name comes from the encoded file's name.
	q := filepath.Join(dir, "greeting.b64")
	os.WriteFile(q, []byte("SGVsbG8sIHdv\ncmxk\n"), 0o644)
	dr, err = DecodeFiles(context.Background(), DecodeOptions{Sources: []string{q}})
	if err != nil || dr.Files[0].Path != filepath.Join(dir, "greeting") || readFile(t, dr.Files[0].Path) != "Hello, world" {
		t.Fatalf("bare base64: %v %+v", err, dr)
	}
	// Bare base64 whose implied name equals the source gets ".decoded".
	r := filepath.Join(dir, "plain")
	os.WriteFile(r, []byte("SGVsbG8="), 0o644)
	dr, err = DecodeFiles(context.Background(), DecodeOptions{Sources: []string{r}})
	if err != nil || dr.Files[0].Path != r+".decoded" {
		t.Fatalf("same-name base64: %v %+v", err, dr)
	}
	// Damaged base64.
	s := filepath.Join(dir, "bad.b64")
	os.WriteFile(s, []byte("SGVsbG8=SGVs\n===="), 0o644)
	if _, err := DecodeFiles(context.Background(), DecodeOptions{Sources: []string{s}}); err == nil {
		t.Error("damaged base64 accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "bad")); !os.IsNotExist(err) {
		t.Error("output of a failed decode was left behind")
	}
}

func TestDecodeNothingAndErrors(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "plain.txt")
	os.WriteFile(p, []byte("just some text, nothing encoded here!\n"), 0o644)
	if _, err := DecodeFiles(context.Background(), DecodeOptions{Sources: []string{p}}); !errors.Is(err, ErrNothingToDecode) {
		t.Errorf("err = %v", err)
	}
	if _, err := DecodeFiles(context.Background(), DecodeOptions{}); err == nil {
		t.Error("no sources accepted")
	}
	if _, err := DecodeFiles(context.Background(), DecodeOptions{Sources: []string{filepath.Join(dir, "missing")}}); err == nil {
		t.Error("missing source accepted")
	}
	// Existing output is not overwritten unless asked.
	q := filepath.Join(dir, "e.uue")
	os.WriteFile(q, []byte("begin 644 plain.txt\n#0V%T\n`\nend\n"), 0o644)
	if _, err := DecodeFiles(context.Background(), DecodeOptions{Sources: []string{q}}); !errors.Is(err, os.ErrExist) {
		t.Errorf("err = %v", err)
	}
	if _, err := DecodeFiles(context.Background(), DecodeOptions{Sources: []string{q}, Overwrite: true}); err != nil {
		t.Errorf("overwrite: %v", err)
	}
	if readFile(t, p) != "Cat" {
		t.Error("overwrite did not happen")
	}
	// Cancel.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DecodeFiles(ctx, DecodeOptions{Sources: []string{q}, Overwrite: true}); !errors.Is(err, ErrCanceled) {
		t.Errorf("cancel err = %v", err)
	}
}

func TestBinHex(t *testing.T) {
	// Run-length edge cases: runs of the marker byte itself, long runs,
	// runs exactly at 255 and around it.
	cases := [][]byte{
		bytes.Repeat([]byte{0x90}, 1),
		bytes.Repeat([]byte{0x90}, 2),
		bytes.Repeat([]byte{0x90}, 300),
		bytes.Repeat([]byte{'a'}, 255),
		bytes.Repeat([]byte{'a'}, 256),
		append(bytes.Repeat([]byte{0}, 1000), 0x90, 0x90, 0, 0x90),
		[]byte("ab\x90\x00\x90c"),
		randomData(5000, 7),
	}
	for i, data := range cases {
		var buf bytes.Buffer
		if err := encodeBinHex(&buf, "file.bin", data, "\n"); err != nil {
			t.Fatal(err)
		}
		f, err := decodeBinHex("junk before\n" + buf.String())
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if f.Name != "file.bin" || !bytes.Equal(f.Data, data) {
			t.Errorf("case %d: round trip failed (name %q, %d bytes)", i, f.Name, len(f.Data))
		}
		for _, line := range strings.Split(buf.String(), "\n") {
			if len(line) > 64 {
				t.Errorf("case %d: line longer than 64 characters", i)
				break
			}
		}
	}
	var buf bytes.Buffer
	encodeBinHex(&buf, "x", []byte("hello hello hello"), "\n")
	s := buf.String()
	// Flip a data character: a CRC must catch it.
	idx := strings.Index(s, ":") + 30
	c := s[idx]
	repl := byte('A')
	if c == 'A' {
		repl = 'B'
	}
	if _, err := decodeBinHex(s[:idx] + string(repl) + s[idx+1:]); err == nil {
		t.Error("corrupted BinHex accepted")
	}
	for _, bad := range []string{"nothing", hqxHeader + "\nno colon", hqxHeader + "\n:abc", hqxHeader + "\n:!!:", hqxHeader + "\n:~~~:"} {
		if _, err := decodeBinHex(bad); err == nil {
			t.Errorf("decodeBinHex(%q) accepted", bad)
		}
	}
	if crc16XModem(0, []byte("123456789")) != 0x31C3 {
		t.Error("CRC-16/XMODEM check value wrong")
	}
}

func TestDetectEncoding(t *testing.T) {
	tests := []struct {
		in   string
		want Encoding
	}{
		{"MIME-Version: 1.0\nContent-Type: text/plain\n\nx", EncodingMIME},
		{"\nFrom: x\nSubject: y\nContent-Type: multipart/mixed; boundary=a\n\n", EncodingMIME},
		{"From: x\nSubject: y\n\nbegin 644 a\n#0V%T\n`\nend\n", EncodingUUE},
		{"hello\nbegin 644 a.txt\n", EncodingUUE},
		{"begin-base64 644 a.txt\n", EncodingUUE},
		{"x\n" + hqxHeader + "\n:", EncodingBinHex},
		{"SGVsbG8=\n", EncodingBase64},
		{"hello world", 0},
		{"", 0},
	}
	for _, tt := range tests {
		if got := detectEncoding([]byte(tt.in)); got != tt.want {
			t.Errorf("detectEncoding(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
