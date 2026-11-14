//go:build unix

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The folder the Claude CLI runs in has a fixed name in the temporary folder, which on Linux is /tmp, shared by every
// user. Another user can make /tmp/mormicommander-ai first (writable for all, or a link to a folder of theirs) and put a
// .claude/settings.json with hooks in it, which the CLI reads from its start folder and runs as this user. Such a
// folder must not be used.
func TestAIHomeRefusesForeignFolder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		plant func(t *testing.T, d, other string)
	}{
		{"writable for all", func(t *testing.T, d, other string) {
			if err := os.Mkdir(d, 0o777); err != nil {
				t.Fatal(err)
			}
			os.Chmod(d, 0o777) // (past the umask)
		}},
		{"a link elsewhere", func(t *testing.T, d, other string) {
			if err := os.Symlink(other, d); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			other := t.TempDir()
			d := filepath.Join(tmp, "mormicommander-ai")
			tc.plant(t, d, other)
			got := aiHome()
			if got == d || got == tmp {
				t.Fatalf("aiHome() = %q: the planted (or the shared) folder is used", got)
			}
			if real, _ := filepath.EvalSymlinks(got); real == other {
				t.Fatalf("aiHome() = %q leads to the planted folder", got)
			}
			fi, err := os.Lstat(got)
			if err != nil || !fi.IsDir() || fi.Mode().Perm()&0o077 != 0 {
				t.Fatalf("aiHome() = %q is not a private folder: %v %v", got, fi, err)
			}
			if again := aiHome(); again != got {
				t.Errorf("aiHome() changed between calls: %q, then %q", got, again)
			}
		})
	}
	// The normal case is unchanged: a folder of our own, made private, is used.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	if got := aiHome(); got != filepath.Join(tmp, "mormicommander-ai") {
		t.Errorf("aiHome() = %q, want the folder in the temporary folder", got)
	}
}
