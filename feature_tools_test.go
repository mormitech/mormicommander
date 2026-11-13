package main

// Tests of the Files-menu tools' bound methods (feature_tools*.go), in temporary folders only: they check the wiring
// to the engines (requests in, results and plain-sentence errors out), not the engines themselves.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func newTestTools() *Tools { return &Tools{app: &App{}, ops: map[string]context.CancelFunc{}} }

func toolsWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestToolsRefusesRemoteAndDemo(t *testing.T) {
	tl := newTestTools()
	if _, err := tl.Properties("ftp://host/x"); err == nil {
		t.Fatal("a server path should be refused")
	}
	if _, err := tl.Properties(filepath.Join(t.TempDir(), "missing.zip", "inner.txt")); err == nil {
		t.Fatal("a path that is not there should be refused")
	}
	demo := &Tools{app: NewApp(startOpts{Demo: true}), ops: map[string]context.CancelFunc{}}
	if _, err := demo.Space("x", []string{t.TempDir()}); err == nil || !strings.Contains(err.Error(), "demo") {
		t.Fatalf("the demo should be refused, got %v", err)
	}
}

func TestToolsSpaceAndProperties(t *testing.T) {
	dir := t.TempDir()
	toolsWrite(t, filepath.Join(dir, "a", "one.bin"), make([]byte, 1000))
	toolsWrite(t, filepath.Join(dir, "a", "sub", "two.bin"), make([]byte, 234))
	tl := newTestTools()
	r, err := tl.Space("s1", []string{filepath.Join(dir, "a")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Total.Bytes != 1234 || r.Items[0].Files != 2 || r.Items[0].Dirs != 1 || !r.Items[0].Complete || r.Canceled {
		t.Fatalf("unexpected sizes: %+v", r)
	}
	p, err := tl.Properties(filepath.Join(dir, "a", "one.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "one.bin" || p.Size != 1000 || p.IsDir || p.Modified == 0 {
		t.Fatalf("unexpected properties: %+v", p)
	}
	sums, err := tl.Checksums("c1", filepath.Join(dir, "a", "one.bin"), []string{"MD5", "SHA256"})
	if err != nil || len(sums["MD5"]) != 32 || len(sums["SHA256"]) != 64 {
		t.Fatalf("checksums: %v %v", sums, err)
	}
	m, err := tl.MatchChecksum("m1", filepath.Join(dir, "a", "one.bin"), sums["SHA256"])
	if err != nil || m["ok"] != true {
		t.Fatalf("match: %v %v", m, err)
	}
}

func TestToolsChecksumsCreateVerify(t *testing.T) {
	dir := t.TempDir()
	toolsWrite(t, filepath.Join(dir, "x.txt"), []byte("hello\n"))
	toolsWrite(t, filepath.Join(dir, "d", "y.txt"), []byte("world\n"))
	tl := newTestTools()
	req := ToolsCRCCreate{BaseDir: dir, Paths: []string{filepath.Join(dir, "x.txt"), filepath.Join(dir, "d")}, Algorithm: "MD5",
		Output: filepath.Join(dir, "all.md5"), Unix: true}
	res, err := tl.CreateChecksums("k1", req)
	if err != nil || len(res.Written) != 1 || res.Hashed != 2 {
		t.Fatalf("create: %+v %v", res, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "all.md5"))
	if !strings.Contains(string(data), "b1946ac92492d2347c6235b4d2611184 *x.txt") || !strings.Contains(string(data), "d/y.txt") {
		t.Fatalf("checksum file: %q", data)
	}
	again, err := tl.CreateChecksums("k2", req)
	if err != nil || again.Exists == "" {
		t.Fatalf("an existing checksum file should be reported, got %+v %v", again, err)
	}
	toolsWrite(t, filepath.Join(dir, "d", "y.txt"), []byte("changed\n"))
	reps, err := tl.VerifyChecksums("v1", []string{filepath.Join(dir, "all.md5"), filepath.Join(dir, "x.txt")})
	if err != nil || len(reps) != 1 || reps[0].OK != 1 || reps[0].Failed != 1 {
		t.Fatalf("verify: %+v %v", reps, err)
	}
	if _, err := tl.VerifyChecksums("v2", []string{filepath.Join(dir, "x.txt")}); err == nil {
		t.Fatal("verify without a checksum file should say so")
	}
	text, err := tl.ChecksumText("t1", []string{filepath.Join(dir, "x.txt")}, "MD5", false)
	if err != nil || strings.TrimSpace(text) != "b1946ac92492d2347c6235b4d2611184" {
		t.Fatalf("clipboard text: %q %v", text, err)
	}
}

func TestToolsSplitCombine(t *testing.T) {
	src, out := t.TempDir(), t.TempDir()
	data := bytes.Repeat([]byte("0123456789"), 3000) // 30,000 bytes
	toolsWrite(t, filepath.Join(src, "big.iso"), data)
	tl := newTestTools()
	if _, err := tl.ParseSize("nonsense"); err == nil {
		t.Fatal("a bad size should be refused")
	}
	sr, err := tl.Split("p1", ToolsSplit{Source: filepath.Join(src, "big.iso"), TargetDir: out, Size: "10000"})
	if err != nil || len(sr.Parts) != 3 || sr.CRCFile == "" {
		t.Fatalf("split: %+v %v", sr, err)
	}
	info, err := tl.CombineInfo([]string{filepath.Join(out, "big.002")})
	if err != nil || len(info.Parts) != 3 || info.Name != "big.iso" || info.CRC == "" || !info.Auto {
		t.Fatalf("combine info: %+v %v", info, err)
	}
	target := filepath.Join(src, "back.iso")
	cr, err := tl.Combine("c1", ToolsCombine{Parts: info.Parts, Auto: true, Target: target})
	if err != nil || !cr.Checked || cr.Size != int64(len(data)) {
		t.Fatalf("combine: %+v %v", cr, err)
	}
	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, data) {
		t.Fatal("the combined file differs from the original")
	}
	if again, err := tl.Combine("c2", ToolsCombine{Parts: info.Parts, Auto: true, Target: target}); err != nil || again.Exists == "" {
		t.Fatalf("an existing target should be reported, got %+v %v", again, err)
	}
	// a damaged part: the check against the .crc file fails with a sentence
	toolsWrite(t, filepath.Join(out, "big.002"), bytes.Repeat([]byte("x"), 10000))
	if _, err := tl.Combine("c3", ToolsCombine{Parts: info.Parts, Auto: true, Target: filepath.Join(src, "bad.iso"), Overwrite: true}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("a damaged part should be reported, got %v", err)
	}
}

func TestToolsEncodeDecode(t *testing.T) {
	src, out, back := t.TempDir(), t.TempDir(), t.TempDir()
	data := []byte{0, 1, 2, 3, 250, 251, 252, 253, 254, 255, 'h', 'i'}
	toolsWrite(t, filepath.Join(src, "bin.dat"), data)
	tl := newTestTools()
	for _, enc := range []string{"mime", "uue", "xxe", "binhex"} {
		er, err := tl.Encode("e-"+enc, ToolsEncode{Sources: []string{filepath.Join(src, "bin.dat")}, Encoding: enc, TargetDir: out, Overwrite: true})
		if err != nil || len(er.Files) != 1 {
			t.Fatalf("%s encode: %+v %v", enc, er, err)
		}
		dr, err := tl.Decode("d-"+enc, ToolsDecode{Sources: er.Files, TargetDir: back, Overwrite: true})
		if err != nil || len(dr.Files) != 1 {
			t.Fatalf("%s decode: %+v %v", enc, dr, err)
		}
		got, _ := os.ReadFile(dr.Files[0])
		if !bytes.Equal(got, data) {
			t.Fatalf("%s: the decoded file differs", enc)
		}
	}
	if _, err := tl.Decode("d-none", ToolsDecode{Sources: []string{filepath.Join(src, "bin.dat")}, TargetDir: back}); err == nil {
		t.Fatal("a file without encoded data should say so")
	}
}

func TestToolsLinks(t *testing.T) {
	dir := t.TempDir()
	toolsWrite(t, filepath.Join(dir, "f.txt"), []byte("x"))
	tl := newTestTools()
	if len(tl.LinkKinds()) == 0 {
		t.Fatal("no link kinds")
	}
	link := filepath.Join(dir, "sub", "f-link")
	os.MkdirAll(filepath.Dir(link), 0o755)
	r, err := tl.CreateLink(ToolsLink{Kind: "symlink", Target: filepath.Join(dir, "f.txt"), Link: link, Relative: true})
	if runtime.GOOS == "windows" && r.Error != "" {
		t.Skip("symbolic links need Developer Mode here")
	}
	if err != nil || r.Error != "" || r.Exists {
		t.Fatalf("symlink: %+v %v", r, err)
	}
	if s, _ := os.Readlink(link); s != filepath.Join("..", "f.txt") {
		t.Fatalf("the relative target is %q", s)
	}
	if r, _ := tl.CreateLink(ToolsLink{Kind: "symlink", Target: filepath.Join(dir, "f.txt"), Link: link}); !r.Exists {
		t.Fatal("an existing link name should be reported")
	}
	res, err := tl.CreateLinks("l1", ToolsLinks{Kind: "hardlink", Sources: []string{filepath.Join(dir, "f.txt")}, DestDir: filepath.Join(dir, "sub")})
	if err != nil || len(res) != 1 || res[0].Error != "" {
		t.Fatalf("hard link: %+v %v", res, err)
	}
	if r, _ := tl.CreateLink(ToolsLink{Kind: "hardlink", Target: dir, Link: filepath.Join(dir, "dirlink")}); r.Error == "" {
		t.Fatal("a hard link to a folder should be refused")
	}
}

func TestToolsSetAttrs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions are Unix")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "d", "s.sh")
	toolsWrite(t, f, []byte("#!/bin/sh\n"))
	tl := newTestTools()
	st, err := tl.AttrInfo([]string{f})
	if err != nil || !st.UnixPerms || st.Octal != "0644" || len(st.Perm) != 12 {
		t.Fatalf("attr info: %+v %v", st, err)
	}
	when := time.Date(2020, 5, 6, 7, 8, 9, 0, time.Local)
	res, err := tl.SetAttrs("a1", ToolsAttrRequest{Paths: []string{filepath.Join(dir, "d")}, Recursive: true, ApplyTo: "files", Mask: "*.sh",
		PermSet: 0o100, Modified: ToolsTime{Mode: "set", At: when.UnixMilli()}})
	if err != nil || res.Changed != 1 || len(res.Failed) != 0 {
		t.Fatalf("set attrs: %+v %v", res, err)
	}
	fi, _ := os.Stat(f)
	if fi.Mode().Perm() != 0o744 || !fi.ModTime().Equal(when) {
		t.Fatalf("mode %v time %v", fi.Mode().Perm(), fi.ModTime())
	}
	if _, err := tl.SetAttrs("a2", ToolsAttrRequest{Paths: []string{f}}); err == nil {
		t.Fatal("an empty change should say so")
	}
}
