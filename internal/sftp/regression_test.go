// This file holds regression tests for bugs found after the first version
// of the engine: a folder download must never write outside the chosen
// local folder, even when the server sends names with backslashes that
// Windows reads as path separators; an SCP download must accept only the
// one item that was asked for (the second half of CVE-2019-6111); and a
// rename that only changes the letter case must work on servers whose file
// system ignores case, as Shift+F6 does in Total Commander.
package sftp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalRel(t *testing.T) {
	win := runtime.GOOS == "windows"
	tests := []struct {
		name, base, target string
		ok                 bool
		want               string // slash-separated; converted per OS
	}{
		{"plain", "/r", "/r/a/b.txt", true, "a/b.txt"},
		{"root base", "/", "/a", true, "a"},
		{"not below base", "/r", "/x/a", false, ""},
		{"backslash climb", "/r", `/r/..\..\evil.txt`, !win, `..\..\evil.txt`},
		{"backslash in folder", "/r", `/r/d/..\x`, !win, `d/..\x`},
		{"windows device name", "/r", "/r/CON", !win, "CON"},
		{"windows stream", "/r", "/r/x:stream", !win, "x:stream"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := localRel(tc.base, tc.target)
			if !tc.ok {
				if err == nil {
					t.Fatalf("localRel = %q, want error", got)
				}
				return
			}
			if err != nil || got != filepath.FromSlash(tc.want) {
				t.Fatalf("localRel = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// TestDownloadTreeBackslashName downloads a server file whose name holds
// backslashes. On UNIX that is an ordinary name and must land inside the
// target folder under that literal name; on Windows the download must be
// refused. Either way nothing may appear outside the target.
func TestDownloadTreeBackslashName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test server cannot create such a name on Windows")
	}
	s := newTestServer(t)
	c := s.dial(t)
	evil := `..\..\evil.txt`
	writeFile(t, filepath.Join(s.Root, "r", evil), "evil")
	parent := t.TempDir()
	local := filepath.Join(parent, "a", "b")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DownloadTree(t.Context(), s.Root+"/r", local, TreeOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(local, "r", evil)); got != "evil" {
		t.Fatalf("content = %q", got)
	}
	if _, err := os.Stat(filepath.Join(parent, "evil.txt")); err == nil {
		t.Fatal("file written outside the target folder")
	}
}

func TestSCPDownloadOnlyRequestedItem(t *testing.T) {
	tests := []struct {
		name  string
		send  []string
		local string // "" = existing folder
	}{
		{"different name into folder", []string{".bashrc"}, ""},
		{"extra item after the requested one", []string{"a.txt", ".bashrc"}, ""},
		{"extra item with a file target", []string{"a.txt", ".bashrc"}, "renamed.txt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t, withSCPSend(tc.send...))
			c := s.dial(t)
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, ".bashrc"), "mine")
			target := dir
			if tc.local != "" {
				target = filepath.Join(dir, tc.local)
			}
			_, err := c.SCPDownload(t.Context(), "/home/x/a.txt", target, SCPOptions{})
			if err == nil || !strings.Contains(err.Error(), ".bashrc") {
				t.Fatalf("err = %v, want refusal naming .bashrc", err)
			}
			if got := readFile(t, filepath.Join(dir, ".bashrc")); got != "mine" {
				t.Fatalf(".bashrc overwritten with %q", got)
			}
		})
	}

	// The requested item alone is still accepted.
	s := newTestServer(t, withSCPSend("a.txt"))
	c := s.dial(t)
	dir := t.TempDir()
	if _, err := c.SCPDownload(t.Context(), "/home/x/a.txt", dir, SCPOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "a.txt")); got != "evil" {
		t.Fatalf("a.txt = %q", got)
	}
}

func TestParseSCPHeaderLocalNames(t *testing.T) {
	win := runtime.GOOS == "windows"
	for _, name := range []string{"x:stream", "CON"} {
		_, _, _, err := parseSCPHeader("C0644 1 " + name)
		if win && err == nil {
			t.Errorf("%q accepted on Windows", name)
		}
		if !win && err != nil {
			t.Errorf("%q refused on %s: %v", name, runtime.GOOS, err)
		}
	}
}

func TestRenameCaseOnly(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	writeFile(t, filepath.Join(s.Root, "case.txt"), "data")
	if err := c.Rename(s.Root+"/case.txt", s.Root+"/CASE.txt"); err != nil {
		t.Fatalf("case-only rename: %v", err)
	}
	es, err := c.ReadDir(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if n := names(es); len(n) != 1 || n[0] != "CASE.txt" {
		t.Fatalf("listing = %q, want [CASE.txt]", n)
	}
	if got := readFile(t, filepath.Join(s.Root, "CASE.txt")); got != "data" {
		t.Fatalf("content = %q", got)
	}
	// RenameOverwrite must not delete the file as an "existing target".
	if err := c.RenameOverwrite(s.Root+"/CASE.txt", s.Root+"/Case.txt"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(s.Root, "Case.txt")); got != "data" {
		t.Fatalf("content after RenameOverwrite = %q", got)
	}

	// On a case-sensitive file system two names differing in case are two
	// files, and renaming onto the other one must still be refused.
	writeFile(t, filepath.Join(s.Root, "x.txt"), "lower")
	writeFile(t, filepath.Join(s.Root, "X.txt"), "upper")
	if readFile(t, filepath.Join(s.Root, "x.txt")) == "upper" {
		return // case-insensitive file system: the second write hit the first file
	}
	if err := c.Rename(s.Root+"/x.txt", s.Root+"/X.txt"); !os.IsExist(err) {
		t.Fatalf("rename onto a distinct file = %v, want exists", err)
	}
}
