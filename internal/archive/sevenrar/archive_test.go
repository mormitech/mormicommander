// These tests cover opening, browsing and reading 7z and RAR archives: the
// folder view (ReadDir with implied folders), Stat, single-file Open with
// checksum verification, unsafe entries, multi-volume sets opened from any
// volume, and the password flows for encrypted file lists and encrypted
// contents. Archives are written by writers_test.go or taken from the
// sevenzip library's test data.

package sevenrar

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleFiles is a small tree with a nested folder, an implied folder, an
// empty file, a Unicode name and a file whose folder is only implied.
func sampleFiles() []tFile {
	return []tFile{
		{name: "docs", dir: true},
		{name: "docs/readme.txt", data: []byte("hello from the archive\n")},
		{name: "docs/empty.txt"},
		{name: "src/main/app.go", data: bytes.Repeat([]byte("package main\n"), 50), mode: 0o755},
		{name: "árvíztűrő tükörfúrógép.txt", data: []byte("unicode ok")},
		{name: "top.bin", data: bytes.Repeat([]byte{0, 1, 2, 3, 250}, 1000)},
	}
}

// makeArchive writes sampleFiles in the given format and returns its path.
func makeArchive(t *testing.T, format string, files []tFile) string {
	t.Helper()
	dir := t.TempDir()
	switch format {
	case "7z":
		p := filepath.Join(dir, "sample.7z")
		write7z(t, p, files)
		return p
	case "rar5":
		return writeRar5(t, filepath.Join(dir, "sample"), files, rarOpts{})[0]
	case "rar4":
		return writeRar4(t, filepath.Join(dir, "sample"), files, 0)[0]
	}
	t.Fatalf("unknown format %s", format)
	return ""
}

var formats = []string{"7z", "rar5", "rar4"}

func readAll(t *testing.T, a *Archive, p string) []byte {
	t.Helper()
	rc, err := a.Open(p)
	if err != nil {
		t.Fatalf("Open(%q): %v", p, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read %q: %v", p, err)
	}
	return b
}

func TestBrowseAndRead(t *testing.T) {
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			files := sampleFiles()
			a, err := Open(makeArchive(t, format, files), Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()

			root, err := a.ReadDir("")
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, e := range root {
				names = append(names, e.Name)
			}
			want := "docs,src,top.bin,árvíztűrő tükörfúrógép.txt"
			if got := strings.Join(names, ","); got != want {
				t.Errorf("root = %s, want %s", got, want)
			}

			src, err := a.Stat("src")
			if err != nil || !src.IsDir || !src.Synthetic {
				t.Errorf("src: %+v %v, want synthetic folder", src, err)
			}
			docs, _ := a.Stat("docs/")
			if !docs.IsDir || docs.Synthetic {
				t.Errorf("docs: %+v, want stored folder", docs)
			}
			sub, err := a.ReadDir("src/main")
			if err != nil || len(sub) != 1 || sub[0].Name != "app.go" {
				t.Fatalf("ReadDir(src/main) = %+v, %v", sub, err)
			}
			if format != "rar4" && sub[0].Mode.Perm() != 0o755 {
				t.Errorf("app.go mode = %v", sub[0].Mode)
			}
			if !sub[0].ModTime.Equal(testTime) {
				t.Errorf("app.go time = %v, want %v", sub[0].ModTime, testTime)
			}

			for _, f := range files {
				if f.dir {
					continue
				}
				if got := readAll(t, a, f.name); !bytes.Equal(got, f.data) {
					t.Errorf("%s: got %d bytes, want %d", f.name, len(got), len(f.data))
				}
				e, err := a.Stat(f.name)
				if err != nil || e.Size != int64(len(f.data)) {
					t.Errorf("Stat(%s) = %+v, %v", f.name, e, err)
				}
			}

			if _, err := a.Stat("nope"); !errors.Is(err, ErrNotFound) {
				t.Errorf("Stat(nope) err = %v", err)
			}
			if _, err := a.ReadDir("top.bin"); !errors.Is(err, ErrNotDir) {
				t.Errorf("ReadDir(file) err = %v", err)
			}
			if _, err := a.Open("docs"); !errors.Is(err, ErrIsDir) {
				t.Errorf("Open(dir) err = %v", err)
			}

			info := a.Info()
			if info.Files != 5 || info.Dirs != 1 || info.Encrypted || info.HeadersEncrypted {
				t.Errorf("Info = %+v", info)
			}
			var walked int
			if err := a.Walk("", func(Entry) error { walked++; return nil }); err != nil || walked != 8 {
				t.Errorf("Walk visited %d, err %v; want 8", walked, err)
			}
		})
	}
}

func TestUnsafeEntriesHiddenFromBrowse(t *testing.T) {
	files := []tFile{
		{name: "ok.txt", data: []byte("ok")},
		{name: "../evil.txt", data: []byte("evil")},
		{name: "a/../../evil2.txt", data: []byte("evil")},
		{name: "/abs/file.txt", data: []byte("abs")},
	}
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			a, err := Open(makeArchive(t, format, files), Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			unsafe := 0
			for _, e := range a.Entries() {
				if e.Unsafe {
					unsafe++
				}
			}
			if unsafe != 2 {
				t.Errorf("unsafe entries = %d, want 2", unsafe)
			}
			root, _ := a.ReadDir("")
			var names []string
			for _, e := range root {
				names = append(names, e.Name)
			}
			if got := strings.Join(names, ","); got != "abs,ok.txt" {
				t.Errorf("root = %s", got)
			}
			if a.Info().Unsafe != 2 {
				t.Errorf("Info.Unsafe = %d", a.Info().Unsafe)
			}
		})
	}
}

func TestOpenRejectsOtherFormats(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.zip")
	writeFile(t, p, append([]byte("PK\x03\x04"), make([]byte, 100)...))
	if _, err := Open(p, Options{}); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("zip: err = %v", err)
	}
	if _, err := Open(filepath.Join(dir, "missing.7z"), Options{}); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing: err = %v", err)
	}
}

func TestLibrary7zFixtures(t *testing.T) {
	for _, name := range []string{"t0.7z", "t1.7z", "lzma2.7z", "bzip2.7z", "empty.7z"} {
		t.Run(name, func(t *testing.T) {
			a, err := Open(filepath.Join("testdata", "7z", name), Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if a.Encrypted() {
				t.Error("reported encrypted")
			}
			res, err := a.Test(context.Background(), nil)
			if err != nil {
				t.Fatalf("Test: %v (%+v)", err, res)
			}
			if name != "empty.7z" && res.Files == 0 {
				t.Error("no files tested")
			}
		})
	}
}

func TestSevenZipMultiVolumeFromAnyPart(t *testing.T) {
	for _, start := range []string{"multi.7z.001", "multi.7z.004"} {
		t.Run(start, func(t *testing.T) {
			a, err := Open(filepath.Join("testdata", "7z", start), Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if got := len(a.Volumes()); got != 6 {
				t.Errorf("volumes = %d, want 6", got)
			}
			if filepath.Base(a.Path()) != "multi.7z.001" {
				t.Errorf("Path = %s", a.Path())
			}
			if _, err := a.Test(context.Background(), nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSevenZipEncryptedHeaders(t *testing.T) {
	p := filepath.Join("testdata", "7z", "t2.7z")
	if _, err := Open(p, Options{}); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("no password: err = %v", err)
	}
	if _, err := Open(p, Options{Password: "nope"}); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("wrong password: err = %v", err)
	}
	var reqs []PasswordRequest
	a, err := Open(p, Options{PasswordFunc: func(r PasswordRequest) (string, bool) {
		reqs = append(reqs, r)
		if r.Attempt == 1 {
			return "wrong", true
		}
		return "password", true
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if len(reqs) != 2 || !reqs[0].HeadersEncrypted || reqs[1].Attempt != 2 {
		t.Errorf("requests = %+v", reqs)
	}
	if !a.HeadersEncrypted() || !a.Encrypted() {
		t.Errorf("HeadersEncrypted=%v Encrypted=%v", a.HeadersEncrypted(), a.Encrypted())
	}
	if _, err := a.Test(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	_, err = Open(p, Options{PasswordFunc: func(PasswordRequest) (string, bool) { return "", false }})
	if !errors.Is(err, ErrPasswordCancelled) {
		t.Errorf("cancel: err = %v", err)
	}
	calls := 0
	_, err = Open(p, Options{MaxPasswordAttempts: 2, PasswordFunc: func(PasswordRequest) (string, bool) { calls++; return "bad", true }})
	if !errors.Is(err, ErrWrongPassword) || calls != 2 {
		t.Errorf("attempts: err = %v, calls = %d", err, calls)
	}
}

func TestSevenZipEncryptedContents(t *testing.T) {
	for _, name := range []string{"t4.7z", "t5.7z"} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join("testdata", "7z", name)
			// The file list is readable without a password.
			a, err := Open(p, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if a.HeadersEncrypted() || !a.Encrypted() {
				t.Errorf("HeadersEncrypted=%v Encrypted=%v", a.HeadersEncrypted(), a.Encrypted())
			}
			if _, err := a.Test(context.Background(), nil); !errors.Is(err, ErrPasswordRequired) {
				t.Errorf("Test without password: err = %v", err)
			}
			if err := a.SetPassword("wrong"); !errors.Is(err, ErrWrongPassword) {
				t.Errorf("SetPassword(wrong) = %v", err)
			}
			if err := a.SetPassword("password"); err != nil {
				t.Errorf("SetPassword(right) = %v", err)
			}
			if _, err := a.Test(context.Background(), nil); err != nil {
				t.Errorf("Test with password: %v", err)
			}
			a.Close()

			// Asked lazily on first read, with the entry name.
			var req PasswordRequest
			a, err = Open(p, Options{PasswordFunc: func(r PasswordRequest) (string, bool) { req = r; return "password", true }})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if req.Attempt != 0 {
				t.Error("password asked before it was needed")
			}
			res, err := a.Extract(context.Background(), t.TempDir(), ExtractOptions{})
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if req.Entry == "" || req.HeadersEncrypted || res.Files == 0 {
				t.Errorf("req = %+v, res = %+v", req, res)
			}
		})
	}
}

func TestRarMultiVolume(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 300) // 4800 bytes
	files := []tFile{
		{name: "a.txt", data: []byte("first file")},
		{name: "big/data.bin", data: data},
		{name: "z.txt", data: []byte("last file")},
	}
	cases := []struct {
		name  string
		write func(dir string) []string
	}{
		{"rar5 new naming", func(dir string) []string {
			return writeRar5(t, filepath.Join(dir, "set"), files, rarOpts{volumeSize: 1500})
		}},
		{"rar5 old naming", func(dir string) []string {
			return writeRar5(t, filepath.Join(dir, "set"), files, rarOpts{volumeSize: 1500, oldNaming: true})
		}},
		{"rar4 old naming", func(dir string) []string {
			return writeRar4(t, filepath.Join(dir, "set"), files, 1500)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			vols := c.write(dir)
			if len(vols) < 3 {
				t.Fatalf("only %d volumes written", len(vols))
			}
			for _, start := range []string{vols[0], vols[len(vols)-1]} {
				a, err := Open(start, Options{})
				if err != nil {
					t.Fatalf("Open(%s): %v", filepath.Base(start), err)
				}
				if got := a.Volumes(); len(got) != len(vols) {
					t.Errorf("Volumes = %v, want %v", got, vols)
				}
				if !bytes.Equal(readAll(t, a, "big/data.bin"), data) {
					t.Error("spanned file differs")
				}
				if string(readAll(t, a, "z.txt")) != "last file" {
					t.Error("z.txt differs")
				}
				if res, err := a.Test(context.Background(), nil); err != nil || res.Files != 3 {
					t.Errorf("Test = %+v, %v", res, err)
				}
				a.Close()
			}
			// A missing middle volume is named in the error.
			if err := os.Remove(vols[1]); err != nil {
				t.Fatal(err)
			}
			a, err := Open(vols[0], Options{})
			if err == nil {
				_, err = a.Test(context.Background(), nil)
				a.Close()
			}
			var mv *MissingVolumeError
			if !errors.As(err, &mv) || !errors.Is(err, ErrMissingVolume) {
				t.Fatalf("missing volume: err = %v", err)
			}
			if mv.Path != "" && filepath.Base(mv.Path) != filepath.Base(vols[1]) {
				t.Errorf("missing volume path = %s, want %s", mv.Path, vols[1])
			}
		})
	}
}

func TestRarEncryptedContents(t *testing.T) {
	files := sampleFiles()
	p := writeRar5(t, filepath.Join(t.TempDir(), "enc"), files, rarOpts{password: "secret"})[0]
	a, err := Open(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.HeadersEncrypted() || !a.Encrypted() {
		t.Fatalf("HeadersEncrypted=%v Encrypted=%v", a.HeadersEncrypted(), a.Encrypted())
	}
	e, _ := a.Stat("docs/readme.txt")
	if !e.Encrypted {
		t.Error("entry not marked encrypted")
	}
	if _, err := a.Open("docs/readme.txt"); !errors.Is(err, ErrPasswordRequired) {
		t.Errorf("Open without password: %v", err)
	}
	if err := a.SetPassword("wrong"); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("SetPassword(wrong) = %v", err)
	}
	if err := a.SetPassword("secret"); err != nil {
		t.Fatalf("SetPassword(secret) = %v", err)
	}
	for _, f := range files {
		if !f.dir && !bytes.Equal(readAll(t, a, f.name), f.data) {
			t.Errorf("%s differs", f.name)
		}
	}
}

func TestRarEncryptedHeaders(t *testing.T) {
	files := sampleFiles()
	p := writeRar5(t, filepath.Join(t.TempDir(), "hdr"), files, rarOpts{password: "secret", encHeaders: true})[0]
	d, err := DetectFile(p)
	if err != nil || !d.HeadersEncrypted || d.RarVersion != 5 {
		t.Errorf("DetectFile = %+v, %v", d, err)
	}
	if _, err := Open(p, Options{}); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("no password: %v", err)
	}
	if _, err := Open(p, Options{Password: "bad"}); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("bad password: %v", err)
	}
	attempts := 0
	a, err := Open(p, Options{Password: "bad", PasswordFunc: func(r PasswordRequest) (string, bool) {
		attempts++
		if r.Attempt != 2 || !r.HeadersEncrypted {
			t.Errorf("request = %+v", r)
		}
		return "secret", true
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if attempts != 1 || !a.HeadersEncrypted() {
		t.Errorf("attempts = %d, HeadersEncrypted = %v", attempts, a.HeadersEncrypted())
	}
	if got := string(readAll(t, a, "docs/readme.txt")); got != "hello from the archive\n" {
		t.Errorf("readme = %q", got)
	}
	if res, err := a.Test(context.Background(), nil); err != nil || res.Files != 5 {
		t.Errorf("Test = %+v, %v", res, err)
	}
}

func TestRar4EncryptedHeadersNeedPassword(t *testing.T) {
	p := filepath.Join(t.TempDir(), "old.rar")
	writeFile(t, p, append(append([]byte(nil), sigRar4...), rar4Main(rar4MainPassword)...))
	d, err := DetectFile(p)
	if err != nil || !d.HeadersEncrypted || d.RarVersion != 4 {
		t.Errorf("DetectFile = %+v, %v", d, err)
	}
	if _, err := Open(p, Options{}); !errors.Is(err, ErrPasswordRequired) {
		t.Errorf("err = %v", err)
	}
}

func TestRarSolidSingleFileOpen(t *testing.T) {
	files := sampleFiles()
	p := writeRar5(t, filepath.Join(t.TempDir(), "solid"), files, rarOpts{solid: true})[0]
	a, err := Open(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if !a.Info().Solid || !a.Detection().Solid {
		t.Error("not reported solid")
	}
	// The last file needs the streaming fallback.
	if got := readAll(t, a, "top.bin"); !bytes.Equal(got, files[5].data) {
		t.Error("top.bin differs")
	}
}

func TestCorruptDataGivesCRCError(t *testing.T) {
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			p := makeArchive(t, format, []tFile{{name: "f.txt", data: []byte("some precious data")}})
			b, _ := os.ReadFile(p)
			i := bytes.Index(b, []byte("precious"))
			b[i] = 'P'
			writeFile(t, p, b)
			a, err := Open(p, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			rc, err := a.Open("f.txt")
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.ReadAll(rc)
			rc.Close()
			if !errors.Is(err, ErrCRC) {
				t.Errorf("read err = %v, want ErrCRC", err)
			}
			res, err := a.Test(context.Background(), nil)
			if !errors.Is(err, ErrCRC) || res.OK() || len(res.Errors) != 1 || res.Errors[0].Path != "f.txt" {
				t.Errorf("Test = %+v, %v", res, err)
			}
		})
	}
}

func TestClosedArchive(t *testing.T) {
	a, err := Open(makeArchive(t, "7z", sampleFiles()), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := a.Open("top.bin"); !errors.Is(err, ErrClosed) {
		t.Errorf("Open after Close: %v", err)
	}
	if _, err := a.Extract(context.Background(), t.TempDir(), ExtractOptions{}); !errors.Is(err, ErrClosed) {
		t.Errorf("Extract after Close: %v", err)
	}
}

func TestEntryRatio(t *testing.T) {
	cases := []struct {
		e    Entry
		want int
	}{
		{Entry{Size: 100, PackedSize: 50}, 50},
		{Entry{Size: 3, PackedSize: 1}, 33},
		{Entry{Size: 100, PackedSize: -1}, -1},
		{Entry{Size: 0, PackedSize: 0}, -1},
	}
	for _, c := range cases {
		if got := c.e.Ratio(); got != c.want {
			t.Errorf("Ratio(%+v) = %d, want %d", c.e, got, c.want)
		}
	}
}
