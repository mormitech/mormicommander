// Tests for the zip packer. Every test works in t.TempDir(). Archives are
// created by our own writer and, for interoperability, by Go's archive/zip
// and (when installed) Info-ZIP's zip/unzip and libarchive's bsdtar, so we
// know other tools read what we write and we read what they write. Binary
// edge cases (legacy code page names, Unicode path fields, data in front of
// the archive, Zip64 end records, damaged data) are crafted directly.
package ziparc

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	mrand "math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/archive"
)

var bg = context.Background()

func writeFile(t *testing.T, p string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

// compressible returns text-like data that deflates well.
func compressible(n int) []byte {
	r := mrand.New(mrand.NewSource(int64(n)))
	words := []string{"mormi", "commander", "total", "zip ", "árvíztűrő ", "tükörfúrógép\n"}
	var b bytes.Buffer
	for b.Len() < n {
		b.WriteString(words[r.Intn(len(words))])
	}
	return b.Bytes()[:n]
}

// makeTree builds a sample source folder and returns its path and the
// expected archive content (path -> data, folders with trailing slash).
func makeTree(t *testing.T) (string, map[string]string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "src")
	files := map[string][]byte{
		"readme.txt":                compressible(5000),
		"bin/random.dat":            randBytes(300 << 10),
		"bin/empty.dat":             {},
		"deep/a/b/c/d.txt":          []byte("deep"),
		"ünïcödé/árvíztűrő.txt":     []byte("hungarian"),
		"日本語/ファイル.txt":              []byte("japanese"),
		"spaces in name/file 1.txt": []byte("spaces"),
	}
	want := map[string]string{}
	for p, d := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(p)), d)
		want["src/"+p] = string(d)
	}
	if err := os.MkdirAll(filepath.Join(root, "emptydir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"", "bin/", "deep/", "deep/a/", "deep/a/b/", "deep/a/b/c/", "ünïcödé/", "日本語/", "spaces in name/", "emptydir/"} {
		want["src/"+d] = ""
	}
	return root, want
}

// contents opens an archive and returns path -> data for every entry.
func contents(t *testing.T, a archive.Archive) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, e := range a.Entries() {
		if e.IsDir {
			out[e.Path+"/"] = ""
			continue
		}
		rc, err := a.Open(e)
		if err != nil {
			t.Fatalf("open %s: %v", e.Path, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", e.Path, err)
		}
		out[e.Path] = string(b)
	}
	return out
}

func diffMaps(t *testing.T, label string, got, want map[string]string) {
	t.Helper()
	for k, v := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("%s: missing %q", label, k)
		} else if g != v {
			t.Errorf("%s: %q differs (%d vs %d bytes)", label, k, len(g), len(v))
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("%s: unexpected %q", label, k)
		}
	}
}

func pack(t *testing.T, zipPath string, ch archive.Changes, opts archive.PackOptions) archive.PackResult {
	t.Helper()
	res, err := Packer{}.Update(bg, zipPath, ch, opts)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	return res
}

func mustOpen(t *testing.T, p string, opts archive.OpenOptions) *Reader {
	t.Helper()
	r, err := Open(p, opts)
	if err != nil {
		t.Fatalf("Open %s: %v", p, err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func TestCreateRoundTrip(t *testing.T) {
	src, want := makeTree(t)
	for _, tc := range []struct {
		name string
		opts archive.PackOptions
	}{
		{"deflate default", archive.PackOptions{}},
		{"deflate 1", archive.PackOptions{Level: 1}},
		{"deflate 9", archive.PackOptions{Method: archive.MethodDeflate, Level: 9}},
		{"store", archive.PackOptions{Method: archive.MethodStore}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			zp := filepath.Join(t.TempDir(), "out.zip")
			res := pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: src}}}, tc.opts)
			if res.Added != 7 {
				t.Errorf("Added = %d", res.Added)
			}
			r := mustOpen(t, zp, archive.OpenOptions{})
			diffMaps(t, "ours", contents(t, r), want)
			rep, err := archive.Verify(bg, r, archive.VerifyOptions{})
			if err != nil || !rep.OK() {
				t.Errorf("verify: %v %+v", err, rep)
			}
			e, _ := r.Stat("src/readme.txt")
			if tc.opts.Method == archive.MethodStore {
				if e.Method != "Store" || e.PackedSize != e.Size {
					t.Errorf("store entry %+v", e)
				}
			} else if e.Method != "Deflate" || e.Ratio() >= 60 {
				t.Errorf("deflate entry method %s ratio %.1f", e.Method, e.Ratio())
			}
			if !e.HasCRC || e.CRC32 != crc32.ChecksumIEEE([]byte(want["src/readme.txt"])) {
				t.Errorf("crc %x", e.CRC32)
			}
			// The standard library must read it too.
			zr, err := zip.OpenReader(zp)
			if err != nil {
				t.Fatal(err)
			}
			defer zr.Close()
			for _, f := range zr.File {
				if strings.HasPrefix(f.Name, "src/ünïcödé/árvíztűrő") && f.Flags&flagUTF8 == 0 {
					t.Error("UTF-8 flag missing on non-ASCII name")
				}
				rc, err := f.Open()
				if err != nil {
					t.Fatal(err)
				}
				b, _ := io.ReadAll(rc)
				rc.Close()
				if !strings.HasSuffix(f.Name, "/") && string(b) != want[f.Name] {
					t.Errorf("stdlib read %s mismatch", f.Name)
				}
			}
			if unzip, err := exec.LookPath("unzip"); err == nil {
				out, err := exec.Command(unzip, "-t", zp).CombinedOutput()
				if err != nil || !bytes.Contains(out, []byte("No errors detected")) {
					t.Errorf("unzip -t: %v\n%s", err, out)
				}
			}
		})
	}
}

func TestLevelsDiffer(t *testing.T) {
	d := t.TempDir()
	writeFile(t, filepath.Join(d, "f.txt"), compressible(400<<10))
	size := func(level int) int64 {
		zp := filepath.Join(d, fmt.Sprintf("l%d.zip", level))
		pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: filepath.Join(d, "f.txt")}}}, archive.PackOptions{Level: level})
		st, _ := os.Stat(zp)
		return st.Size()
	}
	if s1, s9 := size(1), size(9); s9 >= s1 {
		t.Errorf("level 9 (%d) not smaller than level 1 (%d)", s9, s1)
	}
}

func TestMetadata(t *testing.T) {
	d := t.TempDir()
	mt := time.Date(2003, 4, 5, 6, 7, 9, 0, time.Local)
	p := filepath.Join(d, "tool.sh")
	writeFile(t, p, []byte("#!/bin/sh\n"))
	os.Chmod(p, 0o755)
	os.Chtimes(p, mt, mt)
	ro := filepath.Join(d, "ro.txt")
	writeFile(t, ro, []byte("r"))
	os.Chmod(ro, 0o444)
	defer os.Chmod(ro, 0o644)
	zp := filepath.Join(d, "m.zip")
	pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: p}, {Source: ro}}}, archive.PackOptions{})
	r := mustOpen(t, zp, archive.OpenOptions{})
	e, _ := r.Stat("tool.sh")
	if !e.ModTime.Equal(mt) {
		t.Errorf("mtime %v want %v", e.ModTime, mt)
	}
	if runtime.GOOS != "windows" && e.Mode.Perm() != 0o755 {
		t.Errorf("mode %v", e.Mode)
	}
	e2, _ := r.Stat("ro.txt")
	if e2.DOSAttrs&archive.AttrReadOnly == 0 {
		t.Errorf("read-only attr missing: %x", e2.DOSAttrs)
	}
	out := t.TempDir()
	if _, err := archive.Extract(bg, r, archive.ExtractOptions{Dest: out}); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(filepath.Join(out, "tool.sh"))
	if !st.ModTime().Equal(mt) || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o755) {
		t.Errorf("extracted %v %v", st.ModTime(), st.Mode())
	}
	os.Chmod(filepath.Join(out, "ro.txt"), 0o644)
}

func TestSymlinkStore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	d := t.TempDir()
	src := filepath.Join(d, "src")
	writeFile(t, filepath.Join(src, "target.txt"), []byte("t"))
	os.Symlink("target.txt", filepath.Join(src, "link"))
	zp := filepath.Join(d, "l.zip")
	pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: src}}}, archive.PackOptions{StoreSymlinks: true})
	r := mustOpen(t, zp, archive.OpenOptions{})
	e, _ := r.Stat("src/link")
	if !e.IsSymlink() {
		t.Fatalf("not a link: %+v", e)
	}
	out := t.TempDir()
	if _, err := archive.Extract(bg, r, archive.ExtractOptions{Dest: out}); err != nil {
		t.Fatal(err)
	}
	if tgt, err := os.Readlink(filepath.Join(out, "src", "link")); err != nil || tgt != "target.txt" {
		t.Errorf("link %q %v", tgt, err)
	}
	// Followed by default.
	zp2 := filepath.Join(d, "f.zip")
	pack(t, zp2, archive.Changes{Add: []archive.AddItem{{Source: src}}}, archive.PackOptions{})
	r2 := mustOpen(t, zp2, archive.OpenOptions{})
	if e, _ := r2.Stat("src/link"); e.IsSymlink() || e.Size != 1 {
		t.Errorf("followed link entry %+v", e)
	}
}

func TestUpdateOperations(t *testing.T) {
	src, _ := makeTree(t)
	zp := filepath.Join(t.TempDir(), "u.zip")
	pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: src}}}, archive.PackOptions{Password: "pw"})
	// Encrypted entries are copied raw: no password needed for the update.
	res := pack(t, zp, archive.Changes{
		Delete: []string{"src/bin", "src/readme.txt"},
		Rename: []archive.Rename{{From: "src/deep", To: "src/shallow"}, {From: "src/spaces in name/file 1.txt", To: "src/renamed.txt"}},
		Mkdir:  []string{"src/new/folder"},
		Add:    []archive.AddItem{{Dest: "src/added.txt", Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("added")), nil }, Size: 5}},
	}, archive.PackOptions{})
	if res.Deleted != 4 || res.Renamed != 6 || res.Added != 1 {
		t.Errorf("result %+v", res)
	}
	r := mustOpen(t, zp, archive.OpenOptions{Password: func(string, string, int) (string, error) { return "pw", nil }})
	got := contents(t, r)
	want := map[string]string{
		"src/": "", "src/emptydir/": "", "src/added.txt": "added",
		"src/shallow/": "", "src/shallow/a/": "", "src/shallow/a/b/": "", "src/shallow/a/b/c/": "", "src/shallow/a/b/c/d.txt": "deep",
		"src/ünïcödé/": "", "src/ünïcödé/árvíztűrő.txt": "hungarian", "src/日本語/": "", "src/日本語/ファイル.txt": "japanese",
		"src/spaces in name/": "", "src/renamed.txt": "spaces", "src/new/": "", "src/new/folder/": "",
	}
	diffMaps(t, "after update", got, want)
	e, _ := r.Stat("src/renamed.txt")
	if !e.Encrypted {
		t.Error("renamed entry lost encryption")
	}
	if e, _ := r.Stat("src/new"); !e.Implied {
		t.Error("parent of mkdir should be implied")
	}

	errTests := []struct {
		name string
		ch   archive.Changes
		err  error
	}{
		{"delete missing", archive.Changes{Delete: []string{"nope"}}, archive.ErrNotFound},
		{"rename missing", archive.Changes{Rename: []archive.Rename{{From: "nope", To: "x"}}}, archive.ErrNotFound},
		{"rename onto existing", archive.Changes{Rename: []archive.Rename{{From: "src/added.txt", To: "src/renamed.txt"}}}, archive.ErrExist},
		{"mkdir existing", archive.Changes{Mkdir: []string{"src/shallow"}}, archive.ErrExist},
		{"mkdir over implied", archive.Changes{Mkdir: []string{"src/new"}}, archive.ErrExist},
		{"rename into itself", archive.Changes{Rename: []archive.Rename{{From: "src", To: "src/x"}}}, nil},
	}
	before, _ := os.ReadFile(zp)
	for _, tt := range errTests {
		_, err := Packer{}.Update(bg, zp, tt.ch, archive.PackOptions{})
		if err == nil || (tt.err != nil && !errors.Is(err, tt.err)) {
			t.Errorf("%s: err = %v", tt.name, err)
		}
	}
	after, _ := os.ReadFile(zp)
	if !bytes.Equal(before, after) {
		t.Error("failed updates modified the archive")
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(zp), ".*tmp"))
	if len(leftovers) > 0 {
		t.Errorf("temp files left: %v", leftovers)
	}
}

func TestAddConflicts(t *testing.T) {
	d := t.TempDir()
	zp := filepath.Join(d, "c.zip")
	mem := func(name, data string) archive.AddItem {
		return archive.AddItem{Dest: name, Size: int64(len(data)), ModTime: time.Unix(1_600_000_000, 0),
			Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(data)), nil }}
	}
	pack(t, zp, archive.Changes{Add: []archive.AddItem{mem("a.txt", "old"), mem("dir/b.txt", "oldb")}}, archive.PackOptions{})
	tests := []struct {
		name   string
		policy archive.ConflictPolicy
		fn     archive.ConflictFunc
		want   map[string]string
		res    archive.PackResult
	}{
		{"skip", archive.PolicySkip, nil, map[string]string{"a.txt": "old"}, archive.PackResult{Skipped: 1}},
		{"overwrite", archive.PolicyOverwrite, nil, map[string]string{"a.txt": "new"}, archive.PackResult{Replaced: 1}},
		{"auto rename", archive.PolicyAutoRename, nil, map[string]string{"a.txt": "old", "a (2).txt": "new"}, archive.PackResult{Added: 1}},
		{"ask overwrite", archive.PolicyAsk, func(ci archive.ConflictInfo) archive.ConflictDecision {
			if ci.Path != "a.txt" || ci.Existing.Size != 3 {
				t.Errorf("conflict info %+v", ci)
			}
			return archive.ConflictDecision{Action: archive.ActOverwrite}
		}, map[string]string{"a.txt": "new"}, archive.PackResult{Replaced: 1}},
	}
	for _, tt := range tests {
		zc := filepath.Join(d, tt.name+".zip")
		b, _ := os.ReadFile(zp)
		os.WriteFile(zc, b, 0o644)
		res := pack(t, zc, archive.Changes{Add: []archive.AddItem{mem("a.txt", "new")}}, archive.PackOptions{Policy: tt.policy, OnConflict: tt.fn})
		if res.Added != tt.res.Added || res.Replaced != tt.res.Replaced || res.Skipped != tt.res.Skipped {
			t.Errorf("%s: res %+v", tt.name, res)
		}
		r := mustOpen(t, zc, archive.OpenOptions{})
		got := contents(t, r)
		delete(got, "dir/")
		delete(got, "dir/b.txt")
		diffMaps(t, tt.name, got, tt.want)
	}
	// Cancel leaves the archive untouched.
	before, _ := os.ReadFile(zp)
	_, err := Packer{}.Update(bg, zp, archive.Changes{Add: []archive.AddItem{mem("a.txt", "x")}}, archive.PackOptions{
		OnConflict: func(archive.ConflictInfo) archive.ConflictDecision {
			return archive.ConflictDecision{Action: archive.ActCancel}
		}})
	if !errors.Is(err, archive.ErrCanceled) {
		t.Errorf("cancel err %v", err)
	}
	after, _ := os.ReadFile(zp)
	if !bytes.Equal(before, after) {
		t.Error("cancelled update changed the archive")
	}
	// A file cannot replace a folder.
	if _, err := (Packer{}).Update(bg, zp, archive.Changes{Add: []archive.AddItem{mem("dir", "x")}}, archive.PackOptions{Policy: archive.PolicyOverwrite}); !errors.Is(err, archive.ErrExist) {
		t.Errorf("file over folder: %v", err)
	}
}

func TestComments(t *testing.T) {
	d := t.TempDir()
	zp := filepath.Join(d, "c.zip")
	writeFile(t, filepath.Join(d, "f.txt"), []byte("f"))
	c := "Archive comment – ékezetes"
	pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: filepath.Join(d, "f.txt")}}, Comment: &c,
		EntryComments: map[string]string{"f.txt": "file comment"}}, archive.PackOptions{})
	r := mustOpen(t, zp, archive.OpenOptions{})
	if r.Comment() != c {
		t.Errorf("comment %q", r.Comment())
	}
	if e, _ := r.Stat("f.txt"); e.Comment != "file comment" {
		t.Errorf("entry comment %q", e.Comment)
	}
	r.Close()
	// Comments survive an unrelated update and can be cleared.
	pack(t, zp, archive.Changes{Mkdir: []string{"x"}}, archive.PackOptions{})
	r2 := mustOpen(t, zp, archive.OpenOptions{})
	if r2.Comment() != c {
		t.Errorf("comment lost: %q", r2.Comment())
	}
	if e, _ := r2.Stat("f.txt"); e.Comment != "file comment" {
		t.Errorf("entry comment lost %q", e.Comment)
	}
	r2.Close()
	empty := ""
	pack(t, zp, archive.Changes{Comment: &empty, EntryComments: map[string]string{"f.txt": "új"}}, archive.PackOptions{})
	r3 := mustOpen(t, zp, archive.OpenOptions{})
	if r3.Comment() != "" {
		t.Errorf("comment not cleared: %q", r3.Comment())
	}
	if e, _ := r3.Stat("f.txt"); e.Comment != "új" {
		t.Errorf("entry comment %q", e.Comment)
	}
	long := strings.Repeat("x", 70000)
	if _, err := (Packer{}).Update(bg, zp, archive.Changes{Comment: &long}, archive.PackOptions{}); err == nil {
		t.Error("oversized comment accepted")
	}
}

func TestEncryption(t *testing.T) {
	d := t.TempDir()
	big := compressible(200 << 10)
	writeFile(t, filepath.Join(d, "src", "big.txt"), big)
	writeFile(t, filepath.Join(d, "src", "tiny.txt"), []byte("tiny"))
	writeFile(t, filepath.Join(d, "src", "rand.bin"), randBytes(100<<10))
	for _, tc := range []struct {
		enc  archive.Encryption
		name string
	}{
		{archive.EncryptNone, "AES-256"}, // a password without a scheme means AES-256
		{archive.EncryptAES128, "AES-128"},
		{archive.EncryptAES192, "AES-192"},
		{archive.EncryptAES256, "AES-256"},
		{archive.EncryptZipCrypto, "ZipCrypto"},
	} {
		t.Run(tc.enc.String(), func(t *testing.T) {
			zp := filepath.Join(t.TempDir(), "e.zip")
			pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: filepath.Join(d, "src")}}}, archive.PackOptions{Password: "s3cret ő", Encryption: tc.enc})

			// No password callback.
			r := mustOpen(t, zp, archive.OpenOptions{})
			e, _ := r.Stat("src/big.txt")
			if !e.Encrypted || e.Encryption != tc.name {
				t.Errorf("entry %+v", e)
			}
			if _, err := r.Open(e); !errors.Is(err, archive.ErrPasswordRequired) {
				t.Errorf("no password: %v", err)
			}
			// Wrong, wrong, right: the callback is asked until it works and
			// the good password is remembered for other entries.
			var attempts []int
			r2 := mustOpen(t, zp, archive.OpenOptions{Password: func(_, _ string, n int) (string, error) {
				attempts = append(attempts, n)
				if n < 3 {
					return "wrong" + fmt.Sprint(n), nil
				}
				return "s3cret ő", nil
			}})
			got := contents(t, r2)
			if got["src/big.txt"] != string(big) || got["src/tiny.txt"] != "tiny" {
				t.Error("decrypted content differs")
			}
			if tc.enc != archive.EncryptZipCrypto && fmt.Sprint(attempts) != "[1 2 3]" {
				// ZipCrypto has a 1/256 chance per wrong password of passing
				// the check byte, so only AES asserts the exact sequence.
				t.Errorf("attempts %v", attempts)
			}
			rep, err := archive.Verify(bg, r2, archive.VerifyOptions{})
			if err != nil || !rep.OK() {
				t.Errorf("verify %v %+v", err, rep)
			}
			// Cancel from the dialog stops the operation.
			r3 := mustOpen(t, zp, archive.OpenOptions{Password: func(string, string, int) (string, error) { return "", archive.ErrCanceled }})
			if _, err := archive.Verify(bg, r3, archive.VerifyOptions{}); !errors.Is(err, archive.ErrCanceled) {
				t.Errorf("cancel: %v", err)
			}
			if tc.enc == archive.EncryptZipCrypto {
				if unzip, err := exec.LookPath("unzip"); err == nil {
					out, err := exec.Command(unzip, "-t", "-P", "s3cret ő", zp).CombinedOutput()
					if err != nil || !bytes.Contains(out, []byte("No errors detected")) {
						t.Errorf("unzip -t -P: %v\n%s", err, out)
					}
				}
			} else if bsdtar, err := exec.LookPath("bsdtar"); err == nil {
				out := t.TempDir()
				cmd := exec.Command(bsdtar, "-x", "-f", zp, "-C", out, "--passphrase", "s3cret ő")
				if b, err := cmd.CombinedOutput(); err != nil {
					t.Logf("bsdtar cannot read AES here (%v): %s", err, b)
				} else if b, _ := os.ReadFile(filepath.Join(out, "src", "big.txt")); !bytes.Equal(b, big) {
					t.Error("bsdtar extracted different AES content")
				}
			}
		})
	}
}

func TestAESTamperDetected(t *testing.T) {
	d := t.TempDir()
	zp := filepath.Join(d, "t.zip")
	pack(t, zp, archive.Changes{Add: []archive.AddItem{{Dest: "f.txt", Size: 4000, Open: func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(compressible(4000))), nil
	}}}}, archive.PackOptions{Password: "pw", Method: archive.MethodStore})
	b, _ := os.ReadFile(zp)
	i := bytes.Index(b, []byte("f.txt"))
	b[i+5+11+16+2+100] ^= 0x55 // inside the ciphertext of the only entry
	os.WriteFile(zp, b, 0o644)
	r := mustOpen(t, zp, archive.OpenOptions{Password: func(string, string, int) (string, error) { return "pw", nil }})
	rep, err := archive.Verify(bg, r, archive.VerifyOptions{})
	if err != nil || rep.OK() || !errors.Is(rep.Failed[0].Err, archive.ErrChecksum) {
		t.Errorf("tamper not detected: %v %+v", err, rep)
	}
}

func TestReadInfoZipEncrypted(t *testing.T) {
	zipBin, err := exec.LookPath("zip")
	if err != nil {
		t.Skip("zip not installed")
	}
	d := t.TempDir()
	writeFile(t, filepath.Join(d, "in", "a.txt"), compressible(10000))
	zp := filepath.Join(d, "iz.zip")
	cmd := exec.Command(zipBin, "-q", "-r", "-P", "pass", zp, "in")
	cmd.Dir = d
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("zip: %v %s", err, out)
	}
	r := mustOpen(t, zp, archive.OpenOptions{Password: func(string, string, int) (string, error) { return "pass", nil }})
	if got := contents(t, r)["in/a.txt"]; got != string(compressible(10000)) {
		t.Error("Info-ZIP ZipCrypto content differs")
	}
	// Streaming to stdout makes Info-ZIP write data descriptors, so the
	// ZipCrypto check byte comes from the DOS time instead of the CRC.
	zs := filepath.Join(d, "stream.zip")
	out, err := os.Create(zs)
	if err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(zipBin, "-q", "-r", "-P", "pass", "-", "in")
	cmd.Dir, cmd.Stdout = d, out
	if err := cmd.Run(); err != nil {
		t.Fatalf("zip to stdout: %v", err)
	}
	out.Close()
	rs := mustOpen(t, zs, archive.OpenOptions{Password: func(string, string, int) (string, error) { return "pass", nil }})
	if rs.dir.records[len(rs.dir.records)-1].flags&flagDescriptor == 0 {
		t.Log("zip did not use a data descriptor when streaming")
	}
	if got := contents(t, rs)["in/a.txt"]; got != string(compressible(10000)) {
		t.Error("streamed ZipCrypto content differs")
	}
	// Info-ZIP may also store with bzip2.
	zb := filepath.Join(d, "bz.zip")
	cmd = exec.Command(zipBin, "-q", "-r", "-Z", "bzip2", zb, "in")
	cmd.Dir = d
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("zip -Z bzip2 unsupported: %s", out)
		return
	}
	rb := mustOpen(t, zb, archive.OpenOptions{})
	e, _ := rb.Stat("in/a.txt")
	if e.Method != "BZip2" {
		t.Logf("zip did not use bzip2 (%s)", e.Method)
	}
	if got := contents(t, rb)["in/a.txt"]; got != string(compressible(10000)) {
		t.Error("bzip2 content differs")
	}
}

// rawZip builds a zip with archive/zip, letting each entry set raw fields.
func rawZip(t *testing.T, p string, entries []zip.FileHeader, data []string, comment string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := range entries {
		fh := entries[i]
		w, err := zw.CreateHeader(&fh)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, data[i])
	}
	if comment != "" {
		zw.SetComment(comment)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, p, buf.Bytes())
}

func TestLegacyCodepageNames(t *testing.T) {
	d := t.TempDir()
	zp := filepath.Join(d, "old.zip")
	cp852 := string([]byte{0xa0, 'r', 'v', 0xa1, 'z', 't', 0xfb, 'r', 0x8b, '.', 't', 'x', 't'}) // árvíztűrő.txt
	cp852dir := string([]byte{'m', 0x82, 'l', 'y', '/'})                                         // mély/
	unicodeExtra := func(raw, real string) []byte {
		b := make([]byte, 9, 9+len(real))
		binary.LittleEndian.PutUint16(b, extraUnicodePath)
		binary.LittleEndian.PutUint16(b[2:], uint16(5+len(real)))
		b[4] = 1
		binary.LittleEndian.PutUint32(b[5:], crc32.ChecksumIEEE([]byte(raw)))
		return append(b, real...)
	}
	rawZip(t, zp, []zip.FileHeader{
		{Name: cp852, NonUTF8: true, CreatorVersion: hostFAT << 8},
		{Name: cp852dir + "x", NonUTF8: true},
		{Name: "\x81ber.txt", NonUTF8: true, Extra: unicodeExtra("\x81ber.txt", "Über-real.txt")},
		{Name: "utf8 ő.txt"},
	}, []string{"a", "b", "c", "d"}, "")
	tests := []struct {
		enc  string
		want []string
	}{
		{"cp852", []string{"árvíztűrő.txt", "mély", "Über-real.txt", "utf8 ő.txt"}},
		{"ibm852", []string{"árvíztűrő.txt", "mély", "Über-real.txt", "utf8 ő.txt"}},
		{"cp437", []string{"árvízt√rï.txt", "mély", "Über-real.txt", "utf8 ő.txt"}},
	}
	for _, tt := range tests {
		r := mustOpen(t, zp, archive.OpenOptions{Encoding: tt.enc})
		root, _ := r.ReadDir("")
		var names []string
		for _, e := range root {
			names = append(names, e.Name)
		}
		sort.Strings(names)
		w := append([]string(nil), tt.want...)
		sort.Strings(w)
		if fmt.Sprint(names) != fmt.Sprint(w) {
			t.Errorf("%s: %q want %q", tt.enc, names, w)
		}
	}
	if runtime.GOOS != "windows" {
		t.Setenv("LC_ALL", "hu_HU.UTF-8")
		r := mustOpen(t, zp, archive.OpenOptions{})
		if _, err := r.Stat("árvíztűrő.txt"); err != nil {
			t.Errorf("auto encoding with hu locale: %v", err)
		}
		// Renaming a legacy entry re-encodes it as flagged UTF-8 while
		// untouched legacy entries keep their bytes.
		r.Close()
		pack(t, zp, archive.Changes{Rename: []archive.Rename{{From: "árvíztűrő.txt", To: "tükör.txt"}}}, archive.PackOptions{})
		zr, _ := zip.OpenReader(zp)
		defer zr.Close()
		names := map[string]uint16{}
		for _, f := range zr.File {
			names[f.Name] = f.Flags
		}
		if fl, ok := names["tükör.txt"]; !ok || fl&flagUTF8 == 0 {
			t.Errorf("renamed entry not UTF-8: %v", names)
		}
		if _, ok := names[cp852dir+"x"]; !ok {
			t.Errorf("untouched legacy name changed: %v", names)
		}
	}
}

func TestMacUTF8WithoutFlag(t *testing.T) {
	// macOS Archive Utility writes UTF-8 names without the UTF-8 flag.
	zp := filepath.Join(t.TempDir(), "mac.zip")
	rawZip(t, zp, []zip.FileHeader{{Name: "ékezet.txt", NonUTF8: true}}, []string{"x"}, "")
	r := mustOpen(t, zp, archive.OpenOptions{})
	if _, err := r.Stat("ékezet.txt"); err != nil {
		t.Error(err)
	}
}

func TestSelfExtractingPrefix(t *testing.T) {
	d := t.TempDir()
	plain := filepath.Join(d, "plain.zip")
	rawZip(t, plain, []zip.FileHeader{{Name: "inside.txt", Method: zip.Deflate}}, []string{"payload"}, "c")
	b, _ := os.ReadFile(plain)
	exe := filepath.Join(d, "setup.exe")
	writeFile(t, exe, append(append([]byte("MZ"), bytes.Repeat([]byte{0x90}, 5000)...), b...))
	if f, ok := archive.Detect(exe); !ok || f.Name != "zip" {
		t.Fatal("SFX not detected by probe")
	}
	a, err := archive.Open(exe, archive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if got := contents(t, a)["inside.txt"]; got != "payload" {
		t.Errorf("payload %q", got)
	}
}

func TestDetectByContentAndExtension(t *testing.T) {
	d := t.TempDir()
	plain := filepath.Join(d, "x.zip")
	rawZip(t, plain, []zip.FileHeader{{Name: "a"}}, []string{"a"}, "")
	b, _ := os.ReadFile(plain)
	for _, n := range []string{"book.epub", "doc.docx", "lib.jar", "noext"} {
		p := filepath.Join(d, n)
		writeFile(t, p, b)
		if f, ok := archive.Detect(p); !ok || f.Name != "zip" {
			t.Errorf("%s not detected", n)
		}
	}
	if !archive.IsArchiveName("App.APK") || archive.IsArchiveName("notes.txt") {
		t.Error("IsArchiveName")
	}
	if !IsZip(filepath.Join(d, "noext")) {
		t.Error("IsZip")
	}
	writeFile(t, filepath.Join(d, "text.txt"), []byte("hello"))
	if _, ok := archive.Detect(filepath.Join(d, "text.txt")); ok {
		t.Error("text detected as archive")
	}
}

func TestZip64ManyEntries(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	// archive/zip writes Zip64 end records above 65535 entries.
	zp := filepath.Join(t.TempDir(), "many.zip")
	f, _ := os.Create(zp)
	zw := zip.NewWriter(f)
	const n = 70000
	for i := 0; i < n; i++ {
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("d%d/f%d", i%10, i), Method: zip.Store})
		fmt.Fprint(w, i)
	}
	zw.Close()
	f.Close()
	r := mustOpen(t, zp, archive.OpenOptions{})
	files := 0
	for _, e := range r.Entries() {
		if !e.IsDir {
			files++
		}
	}
	if files != n {
		t.Fatalf("files = %d", files)
	}
	if r.dirEndIsZip64() != true {
		t.Error("expected a Zip64 end record")
	}
	e, _ := r.Stat("d3/f69993")
	rc, _ := r.Open(e)
	b, _ := io.ReadAll(rc)
	if string(b) != "69993" {
		t.Errorf("content %q", b)
	}
	// Close before updating: Windows cannot replace a file that is open.
	rc.Close()
	r.Close()
	// Updating keeps all entries (Zip64 again on write).
	pack(t, zp, archive.Changes{Delete: []string{"d0"}}, archive.PackOptions{})
	r2 := mustOpen(t, zp, archive.OpenOptions{})
	if got := len(r2.dir.records); got != n-n/10 {
		t.Errorf("after delete %d records", got)
	}
}

func TestDamagedArchives(t *testing.T) {
	d := t.TempDir()
	zp := filepath.Join(d, "ok.zip")
	data := compressible(50000)
	pack(t, zp, archive.Changes{Add: []archive.AddItem{
		{Dest: "a.txt", Size: int64(len(data)), Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }},
		{Dest: "b.txt", Size: 3, Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("bbb")), nil }},
	}}, archive.PackOptions{})
	b, _ := os.ReadFile(zp)

	// Flip a byte in the compressed data of a.txt.
	bad := append([]byte(nil), b...)
	bad[200] ^= 0xff
	bp := filepath.Join(d, "bad.zip")
	writeFile(t, bp, bad)
	r := mustOpen(t, bp, archive.OpenOptions{})
	rep, err := archive.Verify(bg, r, archive.VerifyOptions{})
	if err != nil || rep.OK() || rep.Failed[0].Path != "a.txt" || !errors.Is(rep.Failed[0].Err, archive.ErrChecksum) || rep.Files != 1 {
		t.Errorf("damaged data: %v %+v", err, rep)
	}
	for name, content := range map[string][]byte{
		"truncated.zip": b[:len(b)/2],
		"empty.zip":     {},
		"garbage.zip":   randBytes(1000),
	} {
		p := filepath.Join(d, name)
		writeFile(t, p, content)
		if _, err := Open(p, archive.OpenOptions{}); !errors.Is(err, archive.ErrFormat) {
			t.Errorf("%s: err %v", name, err)
		}
	}
	// An empty but valid archive opens with no entries.
	ep := filepath.Join(d, "valid-empty.zip")
	pack(t, ep, archive.Changes{}, archive.PackOptions{})
	if r := mustOpen(t, ep, archive.OpenOptions{}); len(r.Entries()) != 0 {
		t.Error("empty archive has entries")
	}
}

func TestMultiVolume(t *testing.T) {
	src, want := makeTree(t)
	d := t.TempDir()
	zp := filepath.Join(d, "multi.zip")
	const vol = 100 << 10
	res := pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: src}}, Comment: ptr("split comment")}, archive.PackOptions{VolumeSize: vol, Level: 9})
	if len(res.Volumes) < 3 {
		t.Fatalf("volumes %v", res.Volumes)
	}
	for i, v := range res.Volumes {
		st, err := os.Stat(v)
		if err != nil || st.Size() > vol {
			t.Errorf("volume %s: %v size %d", v, err, st.Size())
		}
		wantName := fmt.Sprintf("multi.z%02d", i+1)
		if i == len(res.Volumes)-1 {
			wantName = "multi.zip"
		}
		if filepath.Base(v) != wantName {
			t.Errorf("volume %d named %s", i, filepath.Base(v))
		}
	}
	for _, open := range []string{zp, filepath.Join(d, "multi.z01"), filepath.Join(d, "multi.z02")} {
		a, err := archive.Open(open, archive.OpenOptions{})
		if err != nil {
			t.Fatalf("open %s: %v", filepath.Base(open), err)
		}
		r := a.(*Reader)
		if !r.Spanned() || len(r.Volumes()) != len(res.Volumes) || r.Comment() != "split comment" {
			t.Errorf("open %s: spanned=%v volumes=%d comment=%q", filepath.Base(open), r.Spanned(), len(r.Volumes()), r.Comment())
		}
		diffMaps(t, "multi via "+filepath.Base(open), contents(t, a), want)
		a.Close()
	}
	// Updating a spanned set is refused.
	if _, err := (Packer{}).Update(bg, zp, archive.Changes{Mkdir: []string{"x"}}, archive.PackOptions{}); !errors.Is(err, archive.ErrReadOnly) {
		t.Errorf("update spanned: %v", err)
	}
	// Info-ZIP can join it back into a normal archive that unzip accepts.
	if zipBin, err := exec.LookPath("zip"); err == nil {
		joined := filepath.Join(d, "joined.zip")
		if out, err := exec.Command(zipBin, "-q", "-s", "0", zp, "--out", joined).CombinedOutput(); err != nil {
			t.Errorf("zip -s 0: %v %s", err, out)
		} else if unzip, err := exec.LookPath("unzip"); err == nil {
			if out, err := exec.Command(unzip, "-t", joined).CombinedOutput(); err != nil || !bytes.Contains(out, []byte("No errors detected")) {
				t.Errorf("unzip -t joined: %v\n%s", err, out)
			}
		}
	}
	// A missing middle part is reported by name.
	os.Rename(filepath.Join(d, "multi.z02"), filepath.Join(d, "moved"))
	if _, err := Open(zp, archive.OpenOptions{}); !errors.Is(err, archive.ErrMissingVolume) {
		t.Errorf("missing part: %v", err)
	}
	// Too small volumes are rejected.
	if _, err := (Packer{}).Update(bg, filepath.Join(d, "tiny.zip"), archive.Changes{Add: []archive.AddItem{{Source: src}}}, archive.PackOptions{VolumeSize: 1000}); err == nil {
		t.Error("tiny volume size accepted")
	}
	// An archive that fits in one volume stays a single normal file.
	small := filepath.Join(d, "small.zip")
	res = pack(t, small, archive.Changes{Add: []archive.AddItem{{Source: filepath.Join(src, "readme.txt")}}}, archive.PackOptions{VolumeSize: vol})
	if len(res.Volumes) != 1 || res.Volumes[0] != small {
		t.Errorf("small volumes %v", res.Volumes)
	}
}

func ptr(s string) *string { return &s }

func TestReadInfoZipSplit(t *testing.T) {
	zipBin, err := exec.LookPath("zip")
	if err != nil {
		t.Skip("zip not installed")
	}
	d := t.TempDir()
	writeFile(t, filepath.Join(d, "in", "r.bin"), randBytes(400<<10))
	writeFile(t, filepath.Join(d, "in", "t.txt"), compressible(100<<10))
	cmd := exec.Command(zipBin, "-q", "-r", "-s", "128k", "iz.zip", "in")
	cmd.Dir = d
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("zip -s: %v %s", err, out)
	}
	r := mustOpen(t, filepath.Join(d, "iz.zip"), archive.OpenOptions{})
	if !r.Spanned() || len(r.Volumes()) < 3 {
		t.Fatalf("not read as split: %v", r.Volumes())
	}
	rep, err := archive.Verify(bg, r, archive.VerifyOptions{})
	if err != nil || !rep.OK() || rep.Files != 2 {
		t.Errorf("verify Info-ZIP split: %v %+v", err, rep)
	}
}

func TestNumberedParts(t *testing.T) {
	src, want := makeTree(t)
	d := t.TempDir()
	zp := filepath.Join(d, "whole.zip")
	pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: src}}}, archive.PackOptions{})
	b, _ := os.ReadFile(zp)
	part := 70 << 10
	n := 0
	for off := 0; off < len(b); off += part {
		n++
		end := min(off+part, len(b))
		writeFile(t, filepath.Join(d, fmt.Sprintf("cut.zip.%03d", n)), b[off:end])
	}
	for _, p := range []string{"cut.zip.001", fmt.Sprintf("cut.zip.%03d", n)} {
		a, err := Open(filepath.Join(d, p), archive.OpenOptions{})
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		diffMaps(t, p, contents(t, a), want)
		if len(a.Volumes()) != n {
			t.Errorf("%s: %d volumes", p, len(a.Volumes()))
		}
		a.Close()
	}
}

func TestCancelPack(t *testing.T) {
	d := t.TempDir()
	writeFile(t, filepath.Join(d, "big.bin"), randBytes(4<<20))
	zp := filepath.Join(d, "c.zip")
	pack(t, zp, archive.Changes{Mkdir: []string{"keep"}}, archive.PackOptions{})
	before, _ := os.ReadFile(zp)
	ctx, cancel := context.WithCancel(bg)
	calls := 0
	_, err := Packer{}.Update(ctx, zp, archive.Changes{Add: []archive.AddItem{{Source: filepath.Join(d, "big.bin")}}}, archive.PackOptions{
		Progress: func(p archive.Progress) {
			calls++
			if p.Done > 1<<20 {
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	after, _ := os.ReadFile(zp)
	if !bytes.Equal(before, after) {
		t.Error("cancelled pack changed the archive")
	}
	ents, _ := os.ReadDir(d)
	if len(ents) != 2 {
		t.Errorf("leftover files: %v", ents)
	}
}

func TestProgressTotals(t *testing.T) {
	src, _ := makeTree(t)
	zp := filepath.Join(t.TempDir(), "p.zip")
	var last archive.Progress
	pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: src}}}, archive.PackOptions{Progress: func(p archive.Progress) { last = p }})
	if last.Op != "pack" || last.Done != last.Total || last.Files != last.TotalFiles || last.TotalFiles != 7 {
		t.Errorf("progress %+v", last)
	}
}

func TestMoveToArchive(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "src")
	writeFile(t, filepath.Join(src, "a.txt"), []byte("a"))
	writeFile(t, filepath.Join(src, "sub", "b.txt"), []byte("b"))
	writeFile(t, filepath.Join(src, "keep.bak"), []byte("k"))
	zp := filepath.Join(d, "m.zip")
	res := pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: src}}}, archive.PackOptions{MoveSources: true, ExcludeMask: "*.bak", Password: "x"})
	if len(res.Moved) != 3 { // a.txt, sub/b.txt, sub; src stays because of keep.bak
		t.Errorf("moved %v", res.Moved)
	}
	if _, err := os.Stat(filepath.Join(src, "a.txt")); !os.IsNotExist(err) {
		t.Error("a.txt not moved")
	}
	if _, err := os.Stat(filepath.Join(src, "keep.bak")); err != nil {
		t.Error("excluded file removed")
	}
	r := mustOpen(t, zp, archive.OpenOptions{Password: func(string, string, int) (string, error) { return "x", nil }})
	if contents(t, r)["src/sub/b.txt"] != "b" {
		t.Error("moved file not in archive")
	}
}

func TestNestedArchives(t *testing.T) {
	d := t.TempDir()
	inner := filepath.Join(d, "inner.zip")
	pack(t, inner, archive.Changes{Add: []archive.AddItem{{Dest: "deep.txt", Size: 4, Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("deep")), nil }}}}, archive.PackOptions{})
	for _, m := range []archive.Method{archive.MethodStore, archive.MethodDeflate} {
		outer := filepath.Join(d, string(m)+"-outer.zip")
		pack(t, outer, archive.Changes{Add: []archive.AddItem{{Source: inner, Dest: "sub/inner.zip"}}}, archive.PackOptions{Method: m})
		o := mustOpen(t, outer, archive.OpenOptions{})
		e, _ := o.Stat("sub/inner.zip")
		_, _, direct := o.Section(e)
		if direct != (m == archive.MethodStore) {
			t.Errorf("%s: Section ok=%v", m, direct)
		}
		tmp := t.TempDir()
		n, err := archive.OpenNested(bg, o, e, archive.OpenOptions{}, tmp)
		if err != nil {
			t.Fatalf("%s: %v", m, err)
		}
		if n.Path() != outer+"/sub/inner.zip" {
			t.Errorf("nested path %q", n.Path())
		}
		if contents(t, n)["deep.txt"] != "deep" {
			t.Errorf("%s: nested content", m)
		}
		n.Close()
		if left, _ := os.ReadDir(tmp); len(left) != 0 {
			t.Errorf("%s: temp copy not removed: %v", m, left)
		}
	}
}

func TestEntryInfoFields(t *testing.T) {
	d := t.TempDir()
	zp := filepath.Join(d, "i.zip")
	data := compressible(10000)
	pack(t, zp, archive.Changes{Add: []archive.AddItem{
		{Dest: "x.txt", Size: int64(len(data)), Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }},
		{Dest: "folder", Mode: fs.ModeDir},
	}}, archive.PackOptions{})
	r := mustOpen(t, zp, archive.OpenOptions{})
	e, _ := r.Stat("x.txt")
	if e.Size != int64(len(data)) || e.PackedSize <= 0 || e.PackedSize >= e.Size || e.Method != "Deflate" || e.Encrypted {
		t.Errorf("info %+v", e)
	}
	f, _ := r.Stat("folder")
	if !f.IsDir || f.Implied || f.DOSAttrs&archive.AttrDir == 0 || !f.Mode.IsDir() {
		t.Errorf("folder %+v", f)
	}
	if methodName(14) != "LZMA" || methodName(200) != "Method 200" {
		t.Error("methodName")
	}
}

func TestBzip2Entry(t *testing.T) {
	bz, err := exec.LookPath("bzip2")
	if err != nil {
		t.Skip("bzip2 not installed")
	}
	data := compressible(30000)
	cmd := exec.Command(bz, "-c")
	cmd.Stdin = bytes.NewReader(data)
	packed, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	zp := filepath.Join(t.TempDir(), "bz.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.CreateRaw(&zip.FileHeader{Name: "b.txt", Method: methodBzip2, CRC32: crc32.ChecksumIEEE(data),
		CompressedSize64: uint64(len(packed)), UncompressedSize64: uint64(len(data))})
	w.Write(packed)
	zw.Close()
	writeFile(t, zp, buf.Bytes())
	r := mustOpen(t, zp, archive.OpenOptions{})
	if e, _ := r.Stat("b.txt"); e.Method != "BZip2" {
		t.Errorf("method %s", e.Method)
	}
	if contents(t, r)["b.txt"] != string(data) {
		t.Error("bzip2 content differs")
	}
}

func TestUnsupportedMethod(t *testing.T) {
	zp := filepath.Join(t.TempDir(), "lzma.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.CreateRaw(&zip.FileHeader{Name: "x", Method: methodLZMA, CompressedSize64: 3, UncompressedSize64: 10})
	w.Write([]byte{1, 2, 3})
	zw.Close()
	writeFile(t, zp, buf.Bytes())
	r := mustOpen(t, zp, archive.OpenOptions{})
	e, _ := r.Stat("x")
	if _, err := r.Open(e); !errors.Is(err, archive.ErrUnsupportedMethod) || !strings.Contains(err.Error(), "LZMA") {
		t.Errorf("err %v", err)
	}
}

func TestDOSTime(t *testing.T) {
	for _, tm := range []time.Time{
		time.Date(1980, 1, 1, 0, 0, 0, 0, time.Local),
		time.Date(2024, 2, 29, 23, 59, 58, 0, time.Local),
		time.Date(2107, 12, 31, 23, 59, 58, 0, time.Local),
	} {
		d, c := timeToDOS(tm)
		if got := dosToTime(d, c); !got.Equal(tm) {
			t.Errorf("round trip %v -> %v", tm, got)
		}
	}
	d, c := timeToDOS(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
	if got := dosToTime(d, c); got.Year() != 1980 {
		t.Errorf("pre-1980 clamped to %v", got)
	}
	if !dosToTime(0, 0).IsZero() {
		t.Error("zero DOS time should be zero")
	}
}

func TestZipCryptoKnownAnswer(t *testing.T) {
	// Encrypting then decrypting with the same password restores the data,
	// and the check byte rejects a different password most of the time.
	var hdr bytes.Buffer
	w, err := newZipCryptoWriter(&hdr, []byte("pw"), 0xAB000000)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("hello"))
	b := hdr.Bytes()
	var h [12]byte
	copy(h[:], b)
	k, ok := zipCryptoCheck(h, []byte("pw"), 0xAB)
	if !ok {
		t.Fatal("check byte mismatch")
	}
	body := append([]byte(nil), b[12:]...)
	k.decrypt(body)
	if string(body) != "hello" {
		t.Errorf("decrypted %q", body)
	}
}

func TestWinzipCTRCounter(t *testing.T) {
	// The first keystream block must be AES(key, 01 00 .. 00).
	key := bytes.Repeat([]byte{7}, 16)
	c, _ := newWinzipCTR(key)
	zero := make([]byte, 32)
	out := make([]byte, 32)
	c.xor(out, zero)
	c2, _ := newWinzipCTR(key)
	var ctr, ks [16]byte
	ctr[0] = 1
	c2.block.Encrypt(ks[:], ctr[:])
	if !bytes.Equal(out[:16], ks[:]) {
		t.Error("counter does not start at little-endian 1")
	}
	ctr[0] = 2
	c2.block.Encrypt(ks[:], ctr[:])
	if !bytes.Equal(out[16:], ks[:]) {
		t.Error("counter does not increment little-endian")
	}
}

func TestExtractFromZipWithSlip(t *testing.T) {
	// A real zip carrying "../" names extracts inside the target only.
	d := t.TempDir()
	zp := filepath.Join(d, "evil.zip")
	rawZip(t, zp, []zip.FileHeader{{Name: "../../evil.txt"}, {Name: "/abs/x.txt"}, {Name: "good\\win.txt"}}, []string{"e", "a", "w"}, "")
	r := mustOpen(t, zp, archive.OpenOptions{})
	dest := filepath.Join(d, "out")
	if _, err := archive.Extract(bg, r, archive.ExtractOptions{Dest: dest}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"evil.txt", "abs/x.txt", "good/win.txt"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(p))); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(d, "evil.txt")); err == nil {
		t.Error("zip slip!")
	}
	e, _ := r.Stat("evil.txt")
	if !e.Insecure {
		t.Error("insecure flag missing")
	}
}

func TestCopyBetweenArchivesAndConvert(t *testing.T) {
	src, want := makeTree(t)
	d := t.TempDir()
	a := filepath.Join(d, "a.zip")
	pack(t, a, archive.Changes{Add: []archive.AddItem{{Source: src}}, Comment: ptr("from a")}, archive.PackOptions{Password: "one"})
	ra := mustOpen(t, a, archive.OpenOptions{Password: func(string, string, int) (string, error) { return "one", nil }})

	// Copy the "src/deep" folder, seen from inside "src", into "into/" of b.
	b := filepath.Join(d, "b.zip")
	items := archive.ItemsFrom(ra, archive.Selection(ra, []string{"src/deep"}, "src"), "src", "into")
	pack(t, b, archive.Changes{Add: items}, archive.PackOptions{})
	rb := mustOpen(t, b, archive.OpenOptions{})
	diffMaps(t, "copied", contents(t, rb), map[string]string{
		"into/": "", "into/deep/": "", "into/deep/a/": "", "into/deep/a/b/": "", "into/deep/a/b/c/": "", "into/deep/a/b/c/d.txt": "deep",
	})

	// Convert the whole archive, re-encrypting with another password.
	c := filepath.Join(d, "c.zip")
	if _, err := archive.Convert(bg, ra, "zip", c, archive.PackOptions{Password: "two", Encryption: archive.EncryptAES128}); err != nil {
		t.Fatal(err)
	}
	rc := mustOpen(t, c, archive.OpenOptions{Password: func(string, string, int) (string, error) { return "two", nil }})
	diffMaps(t, "converted", contents(t, rc), want)
	if rc.Comment() != "from a" {
		t.Errorf("comment %q", rc.Comment())
	}
	if _, err := archive.Convert(bg, ra, "nope", c, archive.PackOptions{}); !errors.Is(err, archive.ErrReadOnly) {
		t.Errorf("unknown format: %v", err)
	}
}

func TestFormatRegistered(t *testing.T) {
	f, ok := archive.Lookup("zip")
	if !ok || f.Packer == nil || !f.Caps.Write || !f.Caps.Volumes || len(f.Caps.Encryptions) != 4 {
		t.Errorf("format %+v", f)
	}
	if _, err := archive.Open(filepath.Join(t.TempDir(), "missing.zip"), archive.OpenOptions{}); err == nil {
		t.Error("opening a missing file succeeded")
	}
}
