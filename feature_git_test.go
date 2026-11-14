// Tests for the Git bound object: Status on a repository made in a temporary folder (through the real runGit, with its
// extra options and environment), on a plain folder, and on a path that is not a folder. Skipped when git is missing.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitStatusBound(t *testing.T) {
	bin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "none"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0o755)
	cmd := exec.Command(bin, "init", "-q")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("x"), 0o644)

	g := &Git{}
	r := g.Status(repo)
	if !r.Repo || !r.Dirty || r.Files["new.txt"] != "?" || r.Oid != "" {
		t.Errorf("fresh repository: %+v", r)
	}
	plain := filepath.Join(dir, "plain")
	os.MkdirAll(plain, 0o755)
	if r := g.Status(plain); r.Repo {
		t.Errorf("plain folder: %+v", r)
	}
	if r := g.Status(filepath.Join(repo, "new.txt")); r.Repo {
		t.Errorf("a file: %+v", r)
	}
}
