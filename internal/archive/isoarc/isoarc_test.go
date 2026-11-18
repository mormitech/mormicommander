package isoarc

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"unicode/utf16"

	"mormicommander/internal/archive"
)

// isoFile is one file of a test image: its ISO name, Joliet name and data
// split into extents (more than one makes a multi-extent file).
type isoFile struct {
	iso, joliet string
	parts       []string
}

// buildISO writes a small image by hand: PVD at 16, Joliet SVD at 17,
// terminator at 18, roots at 20 (ISO) and 22 (Joliet), the "DOCS" folder at
// 21 and 23, file data from 24 on.
func buildISO(t *testing.T, withJoliet bool, root, docs []isoFile) []byte {
	t.Helper()
	img := make([]byte, 64*sector)
	next := int64(24)
	type placed struct {
		f    isoFile
		exts []extent
	}
	place := func(fs []isoFile) []placed {
		var out []placed
		for _, f := range fs {
			p := placed{f: f}
			for _, part := range f.parts {
				copy(img[next*sector:], part)
				p.exts = append(p.exts, extent{next, int64(len(part))})
				next += int64(len(part)/sector + 1)
			}
			out = append(out, p)
		}
		return out
	}
	pr, pd := place(root), place(docs)
	rec := func(name []byte, lba, size int64, flags byte) []byte {
		l := 33 + len(name)
		if l%2 == 1 {
			l++
		}
		r := make([]byte, l)
		r[0] = byte(l)
		binary.LittleEndian.PutUint32(r[2:], uint32(lba))
		binary.BigEndian.PutUint32(r[6:], uint32(lba))
		binary.LittleEndian.PutUint32(r[10:], uint32(size))
		binary.BigEndian.PutUint32(r[14:], uint32(size))
		copy(r[18:], []byte{124, 5, 17, 10, 30, 0, 0}) // 2024-05-17 10:30:00 UTC
		r[25] = flags
		r[32] = byte(len(name))
		copy(r[33:], name)
		return r
	}
	enc := func(s string, joliet bool) []byte {
		if !joliet {
			return []byte(s)
		}
		var b []byte
		for _, u := range utf16.Encode([]rune(s)) {
			b = binary.BigEndian.AppendUint16(b, u)
		}
		return b
	}
	dir := func(at, self, parent int64, fs []placed, sub string, subAt int64, joliet bool) {
		var b []byte
		b = append(b, rec([]byte{0}, self, sector, 2)...)
		b = append(b, rec([]byte{1}, parent, sector, 2)...)
		if sub != "" {
			b = append(b, rec(enc(sub, joliet), subAt, sector, 2)...)
		}
		for _, p := range fs {
			name := p.f.iso + ";1"
			if joliet {
				name = p.f.joliet
			}
			for i, x := range p.exts {
				var fl byte
				if i < len(p.exts)-1 {
					fl = 0x80
				}
				b = append(b, rec(enc(name, joliet), x.off, x.size, fl)...)
			}
		}
		copy(img[at*sector:], b)
	}
	dir(20, 20, 20, pr, "DOCS", 21, false)
	dir(21, 21, 20, pd, "", 0, false)
	vd := func(at int64, typ byte, rootAt int64, joliet bool) {
		d := img[at*sector:]
		d[0], d[6] = typ, 1
		copy(d[1:], "CD001")
		pad := []byte(" ")
		if joliet {
			pad = []byte("\x00 ")
		}
		copy(d[40:72], bytes.Repeat(pad, 32))
		copy(d[40:], enc("TESTDISC", joliet))
		binary.LittleEndian.PutUint16(d[128:], sector)
		if joliet {
			copy(d[88:], "%/E")
		}
		copy(d[156:], rec([]byte{0}, rootAt, sector, 2))
	}
	vd(16, 1, 20, false)
	term := int64(17)
	if withJoliet {
		dir(22, 22, 22, pr, "Docs", 23, true)
		dir(23, 23, 22, pd, "", 0, true)
		vd(17, 2, 22, true)
		term = 18
	}
	img[term*sector] = 255
	copy(img[term*sector+1:], "CD001")
	return img[:next*sector]
}

func writeTemp(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "disc.iso")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readAll(t *testing.T, z *Reader, p string) string {
	t.Helper()
	e, err := z.Stat(p)
	if err != nil {
		t.Fatalf("stat %s: %v", p, err)
	}
	rc, err := z.Open(e)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func testFiles() ([]isoFile, []isoFile) {
	big := []string{string(bytes.Repeat([]byte("a"), sector)), "tail"}
	return []isoFile{{"README.TXT", "readme.txt", []string{"hello"}}, {"BIG.BIN", "big.bin", big}},
		[]isoFile{{"ARVIZ.TXT", "árvíztűrő tükörfúrógép.txt", []string{"unicode"}}}
}

func TestJolietNamesAndMultiExtent(t *testing.T) {
	root, docs := testFiles()
	p := writeTemp(t, buildISO(t, true, root, docs))
	if !Detect(p) {
		t.Fatal("not detected")
	}
	z, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if z.Format() != "iso" || z.Label() != "TESTDISC" {
		t.Fatalf("format %q label %q", z.Format(), z.Label())
	}
	es, err := z.ReadDir("")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name)
	}
	if len(names) != 3 || names[0] != "Docs" || names[1] != "big.bin" || names[2] != "readme.txt" {
		t.Fatalf("root = %v", names)
	}
	if got := readAll(t, z, "readme.txt"); got != "hello" {
		t.Fatalf("readme = %q", got)
	}
	if got := readAll(t, z, "Docs/árvíztűrő tükörfúrógép.txt"); got != "unicode" {
		t.Fatalf("unicode file = %q", got)
	}
	big := readAll(t, z, "big.bin")
	if len(big) != sector+4 || big[sector:] != "tail" {
		t.Fatalf("multi-extent file has %d bytes", len(big))
	}
	e, _ := z.Stat("big.bin")
	if e.Size != sector+4 || e.ModTime.Year() != 2024 || e.Method != "ISO 9660 (Joliet)" {
		t.Fatalf("big.bin entry = %+v", e)
	}
}

func TestPlainISONames(t *testing.T) {
	root, docs := testFiles()
	p := writeTemp(t, buildISO(t, false, root, docs))
	z, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if got := readAll(t, z, "DOCS/ARVIZ.TXT"); got != "unicode" {
		t.Fatalf("plain name file = %q", got)
	}
	if _, err := z.Stat("README.TXT"); err != nil {
		t.Fatal("the ;1 version suffix was not removed")
	}
}

func TestExtractAndVerify(t *testing.T) {
	root, docs := testFiles()
	z, err := Open(writeTemp(t, buildISO(t, true, root, docs)))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	rep, err := archive.Verify(context.Background(), z, archive.VerifyOptions{})
	if err != nil || !rep.OK() || rep.Files != 3 {
		t.Fatalf("verify: %+v %v", rep, err)
	}
	dest := t.TempDir()
	if _, err := archive.Extract(context.Background(), z, archive.ExtractOptions{Dest: dest}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "Docs", "árvíztűrő tükörfúrógép.txt"))
	if err != nil || string(b) != "unicode" {
		t.Fatalf("extracted %q %v", b, err)
	}
}

func TestNotISO(t *testing.T) {
	p := writeTemp(t, make([]byte, 40*sector))
	if Detect(p) {
		t.Fatal("zeros detected as ISO")
	}
	if _, err := Open(p); err == nil {
		t.Fatal("zeros opened")
	}
	// a damaged image whose root points past the end must fail cleanly, not panic
	root, docs := testFiles()
	img := buildISO(t, false, root, docs)
	binary.LittleEndian.PutUint32(img[16*sector+156+2:], 100000)
	if _, err := Open(writeTemp(t, img)); err == nil {
		t.Fatal("damaged image opened")
	}
}

// An image made by macOS's own hdiutil (no network; skipped elsewhere).
func TestHdiutilImage(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("hdiutil is macOS only")
	}
	hd, err := exec.LookPath("hdiutil")
	if err != nil {
		t.Skip("no hdiutil")
	}
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub folder"), 0o755)
	os.WriteFile(filepath.Join(src, "sub folder", "Hosszú név ő.txt"), []byte("ok"), 0o644)
	out := filepath.Join(t.TempDir(), "made.iso")
	if b, err := exec.Command(hd, "makehybrid", "-iso", "-joliet", "-o", out, src).CombinedOutput(); err != nil {
		t.Skipf("hdiutil failed: %v %s", err, b)
	}
	z, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if got := readAll(t, z, "sub folder/Hosszú név ő.txt"); got != "ok" {
		t.Fatalf("got %q", got)
	}
}
