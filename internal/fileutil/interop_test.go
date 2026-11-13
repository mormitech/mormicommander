// Interoperability checks against the classic command-line tools, when they
// happen to be installed: our UUE output must decode with uudecode, uuencode
// output (classic and -m base64) must decode with us, and shasum must accept
// our SHA256 files. The tests skip themselves on machines without the tools,
// so CI on any OS still passes; they only ever touch temporary folders.

package fileutil

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func needTool(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not installed", name)
	}
	return p
}

func TestInteropUUEncodeTools(t *testing.T) {
	uuenc := needTool(t, "uuencode")
	uudec := needTool(t, "uudecode")
	data := randomData(10000, 11)
	dir := t.TempDir()
	src := filepath.Join(dir, "blob.bin")
	os.WriteFile(src, data, 0o644)

	for _, args := range [][]string{{src, "blob.bin"}, {"-m", src, "blob.bin"}} {
		out, err := exec.Command(uuenc, args...).Output()
		if err != nil {
			t.Fatalf("uuencode %v: %v", args, err)
		}
		enc := filepath.Join(dir, "tool.uue")
		os.WriteFile(enc, out, 0o644)
		dst := t.TempDir()
		dr, err := DecodeFiles(context.Background(), DecodeOptions{Sources: []string{enc}, TargetDir: dst})
		if err != nil {
			t.Fatalf("decode uuencode %v output: %v", args, err)
		}
		if got, _ := os.ReadFile(dr.Files[0].Path); !bytes.Equal(got, data) {
			t.Errorf("uuencode %v output decoded wrongly", args)
		}
	}

	res, err := EncodeFile(context.Background(), EncodeOptions{Source: src, Encoding: EncodingUUE, Target: filepath.Join(dir, "ours.uue"), Format: FormatUnix})
	if err != nil {
		t.Fatal(err)
	}
	outFile := filepath.Join(t.TempDir(), "decoded.bin")
	if out, err := exec.Command(uudec, "-o", outFile, res.Files[0]).CombinedOutput(); err != nil {
		t.Fatalf("uudecode: %v %s", err, out)
	}
	if got, _ := os.ReadFile(outFile); !bytes.Equal(got, data) {
		t.Error("uudecode decoded our UUE wrongly")
	}
}

func TestInteropShasum(t *testing.T) {
	shasum := needTool(t, "shasum")
	dir := makeTree(t, map[string]string{"a.txt": "abc", "sub/b.txt": "hello"})
	_, err := CreateChecksums(context.Background(), CreateOptions{
		Algorithms: []Algorithm{SHA256}, BaseDir: dir, Paths: []string{"a.txt", "sub"}, OutputName: "s", Format: FormatUnix,
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(shasum, "-a", "256", "-c", "s.sha256")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("shasum -c rejected our file: %v\n%s", err, out)
	}
	// And the other way round: shasum's own output verifies here.
	cmd = exec.Command(shasum, "-a", "1", "a.txt", "sub/b.txt")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "tool.sha"), out, 0o644)
	rep, err := VerifyChecksumFile(context.Background(), filepath.Join(dir, "tool.sha"), VerifyOptions{})
	if err != nil || !rep.AllOK() || rep.OK != 2 {
		t.Errorf("verify shasum output: %v\n%s", err, rep.Text())
	}
}
