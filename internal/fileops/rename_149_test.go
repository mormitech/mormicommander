package fileops

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/unicode/norm"
)

// A rename that only changes the Unicode form of a name (NFC to NFD) is a respelling, not a clash with an existing
// file (#149). On APFS both forms reach one entry; elsewhere they are different names and the rename simply happens.
func TestRenameItemUnicodeFormOnly(t *testing.T) {
	dir := t.TempDir()
	nfc := norm.NFC.String("café.txt")
	nfd := norm.NFD.String(nfc)
	src := filepath.Join(dir, nfc)
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	stored := nfc
	if des, _ := os.ReadDir(dir); len(des) == 1 {
		stored = des[0].Name() // a file system may store another form
	}
	want := nfd
	if stored == nfd {
		want = nfc
	}
	got, err := RenameItem(filepath.Join(dir, stored), want)
	if err != nil {
		t.Fatalf("rename %q to %q: %v", stored, want, err)
	}
	if b, err := os.ReadFile(got); err != nil || string(b) != "x" {
		t.Fatalf("after the rename: %v", err)
	}
}

// Renaming one hard link to the name of another link of the same file is refused, and both names stay (#149). It
// used to be taken for a case-only rename and could leave the file under a temporary name.
func TestRenameItemOntoOwnHardLink(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	os.WriteFile(a, []byte("x"), 0o644)
	if err := os.Link(a, b); err != nil {
		t.Skip("no hard links here:", err)
	}
	if _, err := RenameItem(a, "b.txt"); !errors.Is(err, ErrExists) {
		t.Fatalf("rename onto its own hard link: %v, want ErrExists", err)
	}
	for _, p := range []string{a, b} {
		if _, err := os.Lstat(p); err != nil {
			t.Fatalf("%s is gone", filepath.Base(p))
		}
	}
	des, _ := os.ReadDir(dir)
	if len(des) != 2 {
		t.Fatalf("left behind: %v", des)
	}
}

// On a case-sensitive disk "a.txt" and "A.txt" can be two hard links of one file. Renaming one to the other is
// refused rather than treated as a case-only rename, which left the file under a temporary name (#149).
func TestRenameItemCaseOnlyOntoHardLink(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "A.txt")
	os.WriteFile(a, []byte("x"), 0o644)
	if err := os.Link(a, b); err != nil {
		t.Skip("case-insensitive disk or no hard links:", err)
	}
	if _, err := RenameItem(a, "A.txt"); !errors.Is(err, ErrExists) {
		t.Fatalf("case-only rename onto a hard link: %v, want ErrExists", err)
	}
	if des, _ := os.ReadDir(dir); len(des) != 2 {
		t.Fatalf("left behind: %v", des)
	}
}
