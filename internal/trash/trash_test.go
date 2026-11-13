package trash

// These tests never touch the real trash: runCommand and lookPath are replaced by fakes that record the command
// lines the package would run. They check the path rules (full, existing paths only) and that a batch is sent in as
// few calls as the platform allows.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fake replaces the command runner for one test and returns the recorded argument lists.
func fake(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	oldRun, oldLook := runCommand, lookPath
	runCommand = func(cmd *exec.Cmd) ([]byte, error) {
		calls = append(calls, append([]string{filepath.Base(cmd.Path)}, cmd.Args[1:]...))
		return nil, nil
	}
	lookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	t.Cleanup(func() { runCommand, lookPath = oldRun, oldLook })
	return &calls
}

func TestCheckRefusesBadPaths(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "missing"), broken); err != nil {
		t.Skip("symlinks not available:", err)
	}
	tests := []struct {
		name string
		path string
		ok   bool
	}{
		{"empty", "", false},
		{"dot", ".", false},
		{"relative", "f.txt", false},
		{"missing", filepath.Join(dir, "nope"), false},
		{"file", file, true},
		{"folder", dir, true},
		{"broken link", broken, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := fake(t)
			err := Move(tc.path)
			if (err == nil) != tc.ok {
				t.Fatalf("Move(%q) = %v, want ok=%v", tc.path, err, tc.ok)
			}
			if !tc.ok && len(*calls) != 0 {
				t.Fatalf("a refused path must not reach the system: %v", *calls)
			}
		})
	}
}

func TestMoveAllChecksEverythingFirst(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := fake(t)
	if err := MoveAll([]string{file, ""}); err == nil {
		t.Fatal("MoveAll with an empty path must fail")
	}
	if len(*calls) != 0 {
		t.Fatalf("nothing may be moved when one path is bad: %v", *calls)
	}
	if err := MoveAll(nil); err != nil {
		t.Fatalf("MoveAll(nil) = %v", err)
	}
}

func TestMoveAllBatches(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("batching is implemented on macOS and Linux")
	}
	dir := t.TempDir()
	var paths []string
	for _, n := range []string{"a", "b", "c"} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	calls := fake(t)
	if err := MoveAll(paths); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("want one call for three plain files, got %d: %v", len(*calls), *calls)
	}
	got := (*calls)[0]
	if strings.Join(got[len(got)-3:], "|") != strings.Join(paths, "|") {
		t.Fatalf("the paths must be the last arguments, in order: %v", got)
	}
	for _, a := range got[:len(got)-3] {
		for _, p := range paths {
			if strings.Contains(a, p) {
				t.Fatalf("a path must never appear inside script text: %q", a)
			}
		}
	}
}

func TestDarwinTrashesLinksAsLinks(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	calls := fake(t)
	if err := MoveAll([]string{link, file}); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 {
		t.Fatalf("want a link call and a Finder call, got %v", *calls)
	}
	linkCall, finderCall := strings.Join((*calls)[0], "\n"), strings.Join((*calls)[1], "\n")
	if !strings.Contains(linkCall, "trashItemAtURL") || strings.Contains(linkCall, "alias") || !strings.HasSuffix(linkCall, link) {
		t.Errorf("a link must go through NSFileManager without an alias coercion:\n%s", linkCall)
	}
	if !strings.Contains(finderCall, `"Finder"`) || !strings.HasSuffix(finderCall, file) || strings.Contains(finderCall, link) {
		t.Errorf("a plain file goes through Finder, and the link must not:\n%s", finderCall)
	}
}

func TestLinuxNoTool(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	fake(t)
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	if err := Move(t.TempDir()); err != errNoTool {
		t.Fatalf("Move without gio or trash-put = %v, want errNoTool", err)
	}
}
