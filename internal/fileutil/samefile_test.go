// Regression tests for the "output is the input under another spelling"
// data-loss bug: Split, Combine and Decode must recognise their own source
// files through a symlinked folder or a different letter case (on
// case-insensitive disks) even when overwriting is allowed, and refuse or
// rename instead of truncating the source.

package fileutil

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// caseInsensitive reports whether dir lives on a case-insensitive file system.
func caseInsensitive(t *testing.T, dir string) bool {
	t.Helper()
	p := filepath.Join(dir, "probe")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(p)
	_, err := os.Stat(filepath.Join(dir, "PROBE"))
	return err == nil
}

// aliasDirs returns other spellings of dir that name the same folder: a
// symlink (when the platform allows one) and an upper-case name (when the
// file system ignores case). dir must end in a lower-case component.
func aliasDirs(t *testing.T, root, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	link := filepath.Join(root, "link")
	if err := os.Symlink(dir, link); err == nil {
		out["symlink"] = link
	}
	if caseInsensitive(t, dir) {
		out["case"] = filepath.Join(filepath.Dir(dir), strings.ToUpper(filepath.Base(dir)))
	}
	if len(out) == 0 {
		t.Skip("no symlinks and a case-sensitive file system: nothing to test")
	}
	return out
}

func TestSplitRefusesSourceUnderAlias(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, alias := range aliasDirs(t, root, work) {
		t.Run(name, func(t *testing.T) {
			src := filepath.Join(work, "x.001")
			data := bytes.Repeat([]byte("0123456789"), 10)
			if err := os.WriteFile(src, data, 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := SplitFile(context.Background(), SplitOptions{
				Source: src, TargetDir: alias, PartSize: 10, Overwrite: true,
			})
			if err == nil {
				t.Fatal("split into an alias of the source folder succeeded; want a refusal")
			}
			got, rerr := os.ReadFile(src)
			if rerr != nil || !bytes.Equal(got, data) {
				t.Fatalf("source damaged: err=%v len=%d", rerr, len(got))
			}
		})
	}
}

func TestCombineRefusesPartUnderAlias(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	p1 := filepath.Join(work, "data.001")
	p2 := filepath.Join(work, "data.002")
	cases := map[string]CombineOptions{}
	for name, alias := range aliasDirs(t, root, work) {
		cases[name+"-dir"] = CombineOptions{Parts: []string{p1, p2}, TargetDir: alias, TargetName: "data.001", Overwrite: true, NoCRCCheck: true}
	}
	if caseInsensitive(t, work) {
		cases["case-name"] = CombineOptions{Parts: []string{p1, p2}, TargetName: "DATA.001", Overwrite: true, NoCRCCheck: true}
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			os.WriteFile(p1, []byte("AAAA"), 0o644)
			os.WriteFile(p2, []byte("BBBB"), 0o644)
			if _, err := CombineFiles(context.Background(), opts); err == nil {
				t.Fatal("combine onto one of its parts succeeded; want a refusal")
			}
			if got, _ := os.ReadFile(p1); string(got) != "AAAA" {
				t.Fatalf("part 1 now holds %q", got)
			}
		})
	}
}

func TestDecodeDoesNotOverwriteSourceUnderAlias(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	tmp := filepath.Join(root, "tmp")
	for _, d := range []string{work, tmp} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The encoded file carries its own name ("doc.uue") inside, so decoding
	// it into its own folder would land on top of it.
	plain := filepath.Join(tmp, "doc.uue")
	if err := os.WriteFile(plain, []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	enc := filepath.Join(work, "doc.uue")
	if _, err := EncodeFile(context.Background(), EncodeOptions{Source: plain, Encoding: EncodingUUE, Target: enc}); err != nil {
		t.Fatal(err)
	}
	encoded, _ := os.ReadFile(enc)
	for name, alias := range aliasDirs(t, root, work) {
		t.Run(name, func(t *testing.T) {
			res, err := DecodeFiles(context.Background(), DecodeOptions{Sources: []string{enc}, TargetDir: alias, Overwrite: true})
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(enc); !bytes.Equal(got, encoded) {
				t.Fatalf("encoded source was overwritten: %q", got)
			}
			if len(res.Files) != 1 || !strings.HasSuffix(res.Files[0].Path, ".decoded") {
				t.Fatalf("decoded files = %+v; want a renamed .decoded file", res.Files)
			}
			os.Remove(res.Files[0].Path)
		})
	}
}

func TestSFVLeadingSpaceRoundTrip(t *testing.T) {
	dir := makeTree(t, map[string]string{" lead.txt": "abc", "plain.txt": "abc"})
	_, err := CreateChecksums(context.Background(), CreateOptions{
		Algorithms: []Algorithm{CRC32}, BaseDir: dir,
		Paths: []string{" lead.txt", "plain.txt"}, OutputName: "all", Format: FormatUnix,
	})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := VerifyChecksumFile(context.Background(), filepath.Join(dir, "all.sfv"), VerifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.AllOK() || rep.OK != 2 {
		t.Fatalf("verify: %s\n%s", rep.Summary(), rep.Text())
	}

	// An indented SFV line from another tool still finds the unindented name.
	other := filepath.Join(dir, "indented.sfv")
	os.WriteFile(other, []byte("   plain.txt 352441C2\n"), 0o644)
	rep, err = VerifyChecksumFile(context.Background(), other, VerifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.AllOK() || rep.OK != 1 {
		t.Fatalf("indented verify: %s\n%s", rep.Summary(), rep.Text())
	}
}
