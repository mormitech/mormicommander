// Tests for the SCP client against the in-test SCP server: single files and
// whole folders in both directions, preserving times and modes, the
// "target is an existing folder" rule, remote errors, cancellation, and the
// protection against a malicious server sending "../" file names.
package sftp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/remotefs"
)

func TestSCPFile(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	local := t.TempDir()
	data := randomData(200000)
	src := filepath.Join(local, "f.bin")
	os.WriteFile(src, data, 0o644)

	var last remotefs.Progress
	var onFile []string
	opts := SCPOptions{Progress: func(p remotefs.Progress) { last = p },
		OnFile: func(a, b string) { onFile = append(onFile, b) }}

	// Upload to a new name.
	n, err := c.SCPUpload(t.Context(), src, s.Root+"/renamed.bin", opts)
	if err != nil || n != int64(len(data)) {
		t.Fatalf("SCPUpload = %d, %v", n, err)
	}
	if b, _ := os.ReadFile(filepath.Join(s.Root, "renamed.bin")); !bytes.Equal(b, data) {
		t.Fatal("upload differs")
	}
	if last.Done != int64(len(data)) || len(onFile) != 1 {
		t.Errorf("progress %+v onFile %v", last, onFile)
	}
	// Upload into an existing folder keeps the name.
	os.Mkdir(filepath.Join(s.Root, "dir"), 0o755)
	if _, err := c.SCPUpload(t.Context(), src, s.Root+"/dir", SCPOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "dir", "f.bin")); err != nil {
		t.Fatal(err)
	}

	// Download to a new name and into a folder.
	dst := filepath.Join(local, "back.bin")
	n, err = c.SCPDownload(t.Context(), s.Root+"/renamed.bin", dst, opts)
	if err != nil || n != int64(len(data)) {
		t.Fatalf("SCPDownload = %d, %v", n, err)
	}
	if b, _ := os.ReadFile(dst); !bytes.Equal(b, data) {
		t.Fatal("download differs")
	}
	into := t.TempDir()
	if _, err := c.SCPDownload(t.Context(), s.Root+"/renamed.bin", into, SCPOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(into, "renamed.bin")); err != nil {
		t.Fatal(err)
	}

	// Names that need shell quoting survive.
	odd := filepath.Join(local, "it's a file $(x).txt")
	os.WriteFile(odd, []byte("q"), 0o644)
	if _, err := c.SCPUpload(t.Context(), odd, s.Root+"/dir", SCPOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(s.Root, "dir", "it's a file $(x).txt")); got != "q" {
		t.Errorf("odd name content %q", got)
	}
}

func TestSCPTreePreserve(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	mt := time.Date(2005, 6, 7, 8, 9, 10, 0, time.UTC)
	src := filepath.Join(t.TempDir(), "tree")
	writeFile(t, filepath.Join(src, "a.txt"), "A")
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "BB")
	os.MkdirAll(filepath.Join(src, "empty"), 0o755)
	os.Chmod(filepath.Join(src, "a.txt"), 0o600)
	os.Chtimes(filepath.Join(src, "a.txt"), mt, mt)

	os.Mkdir(filepath.Join(s.Root, "dest"), 0o755)
	n, err := c.SCPUpload(t.Context(), src, s.Root+"/dest", SCPOptions{Preserve: true})
	if err != nil || n != 3 {
		t.Fatalf("SCPUpload tree = %d, %v", n, err)
	}
	up := filepath.Join(s.Root, "dest", "tree")
	if readFile(t, filepath.Join(up, "sub", "b.txt")) != "BB" {
		t.Error("b.txt wrong")
	}
	if fi, err := os.Stat(filepath.Join(up, "empty")); err != nil || !fi.IsDir() {
		t.Error("empty folder missing")
	}
	fi, _ := os.Stat(filepath.Join(up, "a.txt"))
	if !fi.ModTime().Equal(mt) {
		t.Errorf("mtime not preserved: %v", fi.ModTime())
	}
	if !isWindows() && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode not preserved: %v", fi.Mode().Perm())
	}

	// Download the tree to a new name, preserving.
	dl := filepath.Join(t.TempDir(), "copy")
	n, err = c.SCPDownload(t.Context(), up, dl, SCPOptions{Preserve: true})
	if err != nil || n != 3 {
		t.Fatalf("SCPDownload tree = %d, %v", n, err)
	}
	if readFile(t, filepath.Join(dl, "a.txt")) != "A" || readFile(t, filepath.Join(dl, "sub", "b.txt")) != "BB" {
		t.Error("downloaded tree content wrong")
	}
	if fi, err := os.Stat(filepath.Join(dl, "empty")); err != nil || !fi.IsDir() {
		t.Error("downloaded empty folder missing")
	}
	fi, _ = os.Stat(filepath.Join(dl, "a.txt"))
	if !fi.ModTime().Equal(mt) {
		t.Errorf("download mtime not preserved: %v", fi.ModTime())
	}
	if !isWindows() && fi.Mode().Perm() != 0o600 {
		t.Errorf("download mode not preserved: %v", fi.Mode().Perm())
	}

	// Into an existing folder keeps the tree's name.
	into := t.TempDir()
	if _, err := c.SCPDownload(t.Context(), up, into, SCPOptions{}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(into, "tree", "a.txt")) != "A" {
		t.Error("tree into folder wrong")
	}
}

func TestSCPErrors(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	_, err := c.SCPDownload(t.Context(), s.Root+"/missing", filepath.Join(t.TempDir(), "x"), SCPOptions{})
	var se *SCPError
	if !errors.As(err, &se) || !strings.Contains(se.Message, "No such file") {
		t.Errorf("missing file err = %v", err)
	}
	if _, err := c.SCPUpload(t.Context(), filepath.Join(t.TempDir(), "missing"), s.Root, SCPOptions{}); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing local err = %v", err)
	}
	// A remote command that does not exist.
	local := filepath.Join(t.TempDir(), "f")
	writeFile(t, local, "x")
	if _, err := c.SCPUpload(t.Context(), local, s.Root, SCPOptions{Command: "noscp"}); err == nil {
		t.Error("bogus scp command succeeded")
	}
}

func TestSCPRejectsUnsafeNames(t *testing.T) {
	s := newTestServer(t, withSCPEvil())
	c := s.dial(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "inside")
	os.Mkdir(target, 0o755)
	_, err := c.SCPDownload(t.Context(), "/whatever", target, SCPOptions{})
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("err = %v, want unsafe name error", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "evil")); !os.IsNotExist(err) {
		t.Fatal("malicious server wrote outside the target folder")
	}
}

func TestSCPCancel(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	data := randomData(4 * 1024 * 1024)
	src := filepath.Join(t.TempDir(), "big")
	os.WriteFile(src, data, 0o644)
	ctx, cancel := context.WithCancel(t.Context())
	_, err := c.SCPUpload(ctx, src, s.Root+"/big", SCPOptions{Progress: func(p remotefs.Progress) {
		if p.Done > 100000 {
			cancel()
		}
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	os.WriteFile(filepath.Join(s.Root, "big2"), data, 0o644)
	ctx, cancel = context.WithCancel(t.Context())
	_, err = c.SCPDownload(ctx, s.Root+"/big2", filepath.Join(t.TempDir(), "b"), SCPOptions{Progress: func(p remotefs.Progress) {
		if p.Done > 100000 {
			cancel()
		}
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("download err = %v", err)
	}
	// Connection still fine.
	if _, err := c.Stat(s.Root); err != nil {
		t.Fatal(err)
	}
}

func TestParseSCPHeader(t *testing.T) {
	tests := []struct {
		line    string
		mode    os.FileMode
		size    int64
		name    string
		wantErr bool
	}{
		{"C0644 12 a.txt", 0o644, 12, "a.txt", false},
		{"D0755 0 dir name", 0o755, 0, "dir name", false},
		{"C0644 12 ../x", 0, 0, "", true},
		{"C0644 12 a/b", 0, 0, "", true},
		{"C0644 12 ..", 0, 0, "", true},
		{"C0644 12 a\\b", 0, 0, "", true},
		{"C0644 -1 a", 0, 0, "", true},
		{"C0999 1 a", 0, 0, "", true},
		{"C0644 1", 0, 0, "", true},
	}
	for _, tc := range tests {
		m, sz, n, err := parseSCPHeader(tc.line)
		if (err != nil) != tc.wantErr {
			t.Errorf("%q: err = %v", tc.line, err)
			continue
		}
		if !tc.wantErr && (m != tc.mode || sz != tc.size || n != tc.name) {
			t.Errorf("%q: %v %d %q", tc.line, m, sz, n)
		}
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"plain":     "'plain'",
		"it's":      `'it'\''s'`,
		"$(rm -rf)": "'$(rm -rf)'",
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
		if got := shellSplit(shellQuote(in)); len(got) != 1 || got[0] != in {
			t.Errorf("round trip %q -> %q", in, got)
		}
	}
}
