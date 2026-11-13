// These tests feed the detector one sample of every format it knows, built in
// memory or written with the standard library, plus the tricky cases: files
// renamed to the wrong extension, self-extracting archives, executables that
// merely contain the 7z signature bytes, compressed tar streams, ISO and UDF
// images, RAR volume flags, and later volumes of split sets that carry no
// signature at all.

package sevenrar

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func tarBytes(t *testing.T, format tar.Format) []byte {
	t.Helper()
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	if err := tw.WriteHeader(&tar.Header{Name: "hello.txt", Mode: 0o644, Size: 5, Format: format, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("hello"))
	tw.Close()
	return b.Bytes()
}

// v7Tar turns a ustar header into an old v7 header (no magic) with a fixed
// checksum.
func v7Tar(t *testing.T) []byte {
	b := tarBytes(t, tar.FormatUSTAR)
	h := b[:512]
	for i := 257; i < 512; i++ {
		h[i] = 0
	}
	for i := 148; i < 156; i++ {
		h[i] = ' '
	}
	var sum int
	for _, c := range h {
		sum += int(c)
	}
	copy(h[148:], []byte(sprintfOctal(sum)))
	return b
}

func sprintfOctal(n int) string {
	s := ""
	for n > 0 {
		s = string(rune('0'+n%8)) + s
		n /= 8
	}
	for len(s) < 6 {
		s = "0" + s
	}
	return s + "\x00 "
}

func gz(t *testing.T, b []byte) []byte {
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	w.Write(b)
	w.Close()
	return out.Bytes()
}

func xzBytes(t *testing.T, b []byte) []byte {
	var out bytes.Buffer
	w, err := xz.NewWriter(&out)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(b)
	w.Close()
	return out.Bytes()
}

func zstdBytes(t *testing.T, b []byte) []byte {
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	return enc.EncodeAll(b, nil)
}

func zipBytes(t *testing.T) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.Create("a.txt")
	w.Write([]byte("zip"))
	zw.Close()
	return b.Bytes()
}

func isoBytes(id string) []byte {
	b := make([]byte, 0x8000+3*2048)
	copy(b[0x8001:], id)
	return b
}

func detectBytes(t *testing.T, b []byte) Detection {
	t.Helper()
	d, err := Detect(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDetectFormats(t *testing.T) {
	dir := t.TempDir()
	p7 := filepath.Join(dir, "a.7z")
	write7z(t, p7, sampleFiles())
	b7, _ := os.ReadFile(p7)
	r5, _ := os.ReadFile(writeRar5(t, filepath.Join(dir, "r5"), sampleFiles(), rarOpts{})[0])
	r4, _ := os.ReadFile(writeRar4(t, filepath.Join(dir, "r4"), sampleFiles(), 0)[0])
	tarU := tarBytes(t, tar.FormatUSTAR)
	tarG := tarBytes(t, tar.FormatGNU)

	cases := []struct {
		name    string
		data    []byte
		format  Format
		variant string
		inner   Format
		rarVer  int
	}{
		{"7z", b7, Format7z, "", 0, 0},
		{"rar5", r5, FormatRar, "", 0, 5},
		{"rar4", r4, FormatRar, "", 0, 4},
		{"zip", zipBytes(t), FormatZip, "", 0, 0},
		{"empty zip", []byte("PK\x05\x06" + string(make([]byte, 18))), FormatZip, "", 0, 0},
		{"spanned zip", append([]byte("PK\x07\x08PK\x03\x04"), make([]byte, 30)...), FormatZip, "spanned", 0, 0},
		{"ustar", tarU, FormatTar, "ustar", 0, 0},
		{"gnu tar", tarG, FormatTar, "gnu", 0, 0},
		{"v7 tar", v7Tar(t), FormatTar, "v7", 0, 0},
		{"gzip", gz(t, []byte("just text")), FormatGzip, "", FormatUnknown, 0},
		{"tgz", gz(t, tarU), FormatGzip, "", FormatTar, 0},
		{"xz", xzBytes(t, []byte("text")), FormatXz, "", FormatUnknown, 0},
		{"txz", xzBytes(t, tarU), FormatXz, "", FormatTar, 0},
		{"tzst", zstdBytes(t, tarU), FormatZstd, "", FormatTar, 0},
		{"bzip2", []byte("BZh91AY&SY\x00\x00"), FormatBzip2, "", FormatUnknown, 0},
		{"lz4", []byte{0x04, 0x22, 0x4D, 0x18, 0x64, 0x40, 0xA7}, FormatLz4, "", 0, 0},
		{"compress", []byte{0x1F, 0x9D, 0x90, 'a'}, FormatCompress, "", 0, 0},
		{"cab", append([]byte("MSCF\x00\x00\x00\x00"), make([]byte, 40)...), FormatCab, "", 0, 0},
		{"iso", isoBytes("CD001"), FormatISO, "", 0, 0},
		{"udf", isoBytes("BEA01"), FormatISO, "udf", 0, 0},
		{"ar", []byte("!<arch>\nfoo.o/          "), FormatAr, "", 0, 0},
		{"deb", []byte("!<arch>\ndebian-binary   1342943816  0     0     100644  4         `\n2.0\n"), FormatAr, "deb", 0, 0},
		{"cpio newc", []byte("070701000000"), FormatCpio, "newc", 0, 0},
		{"cpio odc", []byte("070707000000"), FormatCpio, "odc", 0, 0},
		{"cpio binary", []byte{0xC7, 0x71, 0, 0}, FormatCpio, "binary", 0, 0},
		{"arj", []byte{0x60, 0xEA, 0x22, 0x00, 0x1E}, FormatArj, "", 0, 0},
		{"lzh", []byte("\x1f\x00-lh5-\x00\x00"), FormatLzh, "", 0, 0},
		{"text", []byte("hello world, not an archive"), FormatUnknown, "", 0, 0},
		{"empty", nil, FormatUnknown, "", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := detectBytes(t, c.data)
			if d.Format != c.format || d.Variant != c.variant || d.Inner != c.inner || d.RarVersion != c.rarVer {
				t.Errorf("Detect = %+v, want format %s variant %q inner %s rar %d", d, c.format, c.variant, c.inner, c.rarVer)
			}
			if d.SFX || d.Offset != 0 {
				t.Errorf("unexpected SFX: %+v", d)
			}
			if d.Supported() != (c.format == Format7z || c.format == FormatRar) {
				t.Errorf("Supported = %v", d.Supported())
			}
		})
	}
}

func TestDetectSFX(t *testing.T) {
	dir := t.TempDir()
	p7 := filepath.Join(dir, "a.7z")
	write7z(t, p7, sampleFiles())
	b7, _ := os.ReadFile(p7)
	r5, _ := os.ReadFile(writeRar5(t, filepath.Join(dir, "r5"), sampleFiles(), rarOpts{solid: true})[0])

	stub := append([]byte("MZ\x90\x00"), bytes.Repeat([]byte{0xCC}, 70000)...)
	// The stub contains bare signature bytes (like 7-Zip's own code does);
	// they must not be taken for an archive because no valid header follows.
	stub = append(stub, sig7z...)
	stub = append(stub, bytes.Repeat([]byte{0}, 100)...)
	stub = append(stub, sigRar5...)
	stub = append(stub, bytes.Repeat([]byte{0x11}, 100)...)

	d := detectBytes(t, append(append([]byte(nil), stub...), b7...))
	if d.Format != Format7z || !d.SFX || d.Offset != int64(len(stub)) {
		t.Errorf("7z sfx: %+v", d)
	}
	d = detectBytes(t, append(append([]byte(nil), stub...), r5...))
	if d.Format != FormatRar || !d.SFX || d.Offset != int64(len(stub)) || !d.Solid {
		t.Errorf("rar sfx: %+v", d)
	}
	d = detectBytes(t, append(append([]byte(nil), stub...), zipBytes(t)...))
	if d.Format != FormatZip || !d.SFX {
		t.Errorf("zip sfx: %+v", d)
	}
	d = detectBytes(t, stub)
	if d.Format != FormatUnknown {
		t.Errorf("plain exe: %+v", d)
	}

	// An SFX RAR opens like a normal archive.
	sfx := filepath.Join(dir, "setup.exe")
	writeFile(t, sfx, append(append([]byte(nil), stub...), r5...))
	a, err := Open(sfx, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if got := string(readAll(t, a, "docs/readme.txt")); got != "hello from the archive\n" {
		t.Errorf("readme = %q", got)
	}
	if !a.Info().SFX {
		t.Error("Info.SFX false")
	}

	sfx7 := filepath.Join(dir, "setup7.exe")
	writeFile(t, sfx7, append(append([]byte(nil), stub...), b7...))
	a7, err := Open(sfx7, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer a7.Close()
	if res, err := a7.Test(context.Background(), nil); err != nil || res.Files != 5 {
		t.Errorf("7z sfx Test = %+v, %v", res, err)
	}
}

func TestDetectRenamedArchive(t *testing.T) {
	dir := t.TempDir()
	// A RAR named .zip and a 7z named .docx are recognised by content.
	paths := writeRar5(t, filepath.Join(dir, "x"), sampleFiles(), rarOpts{})
	renamed := filepath.Join(dir, "x.zip")
	os.Rename(paths[0], renamed)
	d, err := DetectFile(renamed)
	if err != nil || d.Format != FormatRar {
		t.Errorf("rar as zip: %+v %v", d, err)
	}
	p := filepath.Join(dir, "report.docx")
	write7z(t, p, sampleFiles())
	if a, err := Open(p, Options{}); err != nil {
		t.Errorf("open 7z as docx: %v", err)
	} else {
		a.Close()
	}
	if _, err := DetectFile(dir); err == nil {
		t.Error("DetectFile(dir) did not fail")
	}
	if _, err := DetectFile(filepath.Join(dir, "missing")); err == nil {
		t.Error("DetectFile(missing) did not fail")
	}
}

func TestDetectVolumes(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("v"), 4000)
	files := []tFile{{name: "d.bin", data: data}}

	newVols := writeRar5(t, filepath.Join(dir, "n"), files, rarOpts{volumeSize: 1500})
	oldVols := writeRar4(t, filepath.Join(dir, "o"), files, 1500)
	for i, p := range newVols {
		d, err := DetectFile(p)
		if err != nil {
			t.Fatal(err)
		}
		want := VolumeInfo{Multi: true, Number: i, First: i == 0}
		if d.Volume != want {
			t.Errorf("%s: volume = %+v, want %+v", filepath.Base(p), d.Volume, want)
		}
	}
	for i, p := range oldVols {
		d, err := DetectFile(p)
		if err != nil {
			t.Fatal(err)
		}
		want := VolumeInfo{Multi: true, Number: i, First: i == 0}
		if d.Volume != want {
			t.Errorf("%s: volume = %+v, want %+v", filepath.Base(p), d.Volume, want)
		}
	}
	// A single archive is not a volume.
	single := writeRar5(t, filepath.Join(dir, "s"), files, rarOpts{})[0]
	if d, _ := DetectFile(single); d.Volume.Multi {
		t.Errorf("single: %+v", d.Volume)
	}

	// Split 7z: the later parts have no signature but are recognised.
	for i := 1; i <= 6; i++ {
		src := filepath.Join("testdata", "7z", "multi.7z.00"+string(rune('0'+i)))
		b, _ := os.ReadFile(src)
		writeFile(t, filepath.Join(dir, "m.7z.00"+string(rune('0'+i))), b)
	}
	d, err := DetectFile(filepath.Join(dir, "m.7z.001"))
	if err != nil || d.Format != Format7z || d.Volume != (VolumeInfo{Multi: true, Number: 0, First: true}) {
		t.Errorf("m.7z.001: %+v %v", d, err)
	}
	d, err = DetectFile(filepath.Join(dir, "m.7z.003"))
	if err != nil || d.Format != Format7z || d.Volume != (VolumeInfo{Multi: true, Number: 2, First: false}) {
		t.Errorf("m.7z.003: %+v %v", d, err)
	}
	// A lone .001 file is not reported as multi-volume.
	b7, _ := os.ReadFile(filepath.Join(dir, "m.7z.001"))
	writeFile(t, filepath.Join(dir, "lone.7z.001"), b7)
	if d, _ := DetectFile(filepath.Join(dir, "lone.7z.001")); d.Volume.Multi {
		t.Errorf("lone: %+v", d.Volume)
	}
	// Spanned zip parts.
	writeFile(t, filepath.Join(dir, "z.z01"), append([]byte("PK\x07\x08PK\x03\x04"), make([]byte, 30)...))
	if d, _ := DetectFile(filepath.Join(dir, "z.z01")); d.Format != FormatZip || !d.Volume.First {
		t.Errorf("z.z01: %+v", d)
	}
}

func TestReadVint(t *testing.T) {
	cases := []struct {
		in   []byte
		v    uint64
		size int
	}{
		{[]byte{0x05}, 5, 1},
		{[]byte{0x80, 0x01}, 128, 2},
		{[]byte{0xFF, 0x7F}, 16383, 2},
		{[]byte{0x80}, 0, 0},
		{nil, 0, 0},
	}
	for _, c := range cases {
		v, n := readVint(c.in)
		if v != c.v || n != c.size {
			t.Errorf("readVint(%x) = %d,%d want %d,%d", c.in, v, n, c.v, c.size)
		}
	}
}

func TestFormatString(t *testing.T) {
	if Format7z.String() != "7z" || FormatRar.String() != "rar" || Format(999).String() != "unknown" {
		t.Error("String")
	}
	if !FormatGzip.Compressor() || FormatTar.Compressor() || FormatZip.Compressor() {
		t.Error("Compressor")
	}
}
