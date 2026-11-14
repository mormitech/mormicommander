// Tests for the git status reader: the porcelain v2 parser on fixed output (branch headers, every record kind, names
// with spaces, renames, folders aggregated, a folder ignored as a whole), the rev-parse reader, Query's answers when
// git is missing or fails, and one run against a real repository made in a temporary folder when git is installed.
package gitstat

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// z joins records the way "git status -z" prints them.
func z(recs ...string) []byte { return []byte(strings.Join(recs, "\x00") + "\x00") }

const h40 = "0123456789abcdef0123456789abcdef01234567"

func TestParseStatusTop(t *testing.T) {
	out := z(
		"# branch.oid "+h40,
		"# branch.head main",
		"# branch.upstream origin/main",
		"# branch.ab +2 -3",
		"1 .M N... 100644 100644 100644 "+h40+" "+h40+" sub/a",
		"1 A. N... 000000 100644 100644 "+h40+" "+h40+" new file.txt",
		"1 D. N... 100644 000000 000000 "+h40+" "+h40+" gone",
		"2 R. N... 100644 100644 100644 "+h40+" "+h40+" R100 top2", "top",
		"u UU N... 100644 100644 100644 100644 "+h40+" "+h40+" "+h40+" clash",
		"? sub/deep/",
		"? loose",
		"! build/",
		"! sub/x.log",
	)
	r := ParseStatus(out, "")
	if !r.Repo || r.Branch != "main" || r.Detached || r.Oid != "0123456" || r.Upstream != "origin/main" || r.Ahead != 2 || r.Behind != 3 || !r.Dirty {
		t.Fatalf("headers: %+v", r)
	}
	want := map[string]string{"sub": "M", "new file.txt": "A", "gone": "D", "top2": "R", "clash": "U", "loose": "?", "build": "!"}
	if len(r.Files) != len(want) {
		t.Errorf("files = %v, want %v", r.Files, want)
	}
	for k, v := range want {
		if r.Files[k] != v {
			t.Errorf("%s = %q, want %q (all: %v)", k, r.Files[k], v, r.Files)
		}
	}
	if _, ok := r.Files["top"]; ok {
		t.Error("the old name of a rename is listed")
	}
	if r.All != "" {
		t.Errorf("All = %q", r.All)
	}
}

func TestParseStatusSubfolder(t *testing.T) {
	out := z(
		"# branch.oid (initial)",
		"# branch.head (detached)",
		"1 .M N... 100644 100644 100644 "+h40+" "+h40+" sub/a",
		"? sub/deep/",
		"! sub/x.log",
		"? other/y",
		"u AA N... 100644 100644 100644 100644 "+h40+" "+h40+" "+h40+" sub/inner/c",
		"? sub/inner/d",
	)
	r := ParseStatus(out, "sub/")
	if !r.Detached || r.Branch != "" || r.Oid != "" {
		t.Errorf("headers: %+v", r)
	}
	want := map[string]string{"a": "M", "deep": "?", "x.log": "!", "inner": "U"}
	if len(r.Files) != len(want) {
		t.Errorf("files = %v, want %v", r.Files, want)
	}
	for k, v := range want {
		if r.Files[k] != v {
			t.Errorf("%s = %q, want %q", k, r.Files[k], v)
		}
	}
}

func TestParseStatusIgnoredInsideAndWholeFolder(t *testing.T) {
	r := ParseStatus(z("# branch.head main", "! lib/cache.tmp", "! build/"), "")
	if r.Dirty {
		t.Error("only ignored files makes the tree dirty")
	}
	if _, ok := r.Files["lib"]; ok {
		t.Error("an ignored file marks its tracked folder")
	}
	r = ParseStatus(z("# branch.head main", "! build/"), "build/")
	if r.All != "!" {
		t.Errorf("inside an ignored folder All = %q", r.All)
	}
	r = ParseStatus(z("# branch.head main", "? fresh/"), "fresh/out/")
	if r.All != "" || len(r.Files) != 0 {
		t.Errorf("a folder below an untracked one: %+v (git lists only the top untracked folder)", r)
	}
}

func TestParseStatusClean(t *testing.T) {
	r := ParseStatus(z("# branch.oid "+h40, "# branch.head dev"), "")
	if !r.Repo || r.Dirty || len(r.Files) != 0 || r.Branch != "dev" || r.Upstream != "" {
		t.Errorf("%+v", r)
	}
	if ParseStatus(nil, "").Repo != true {
		t.Error("empty output is still a repository")
	}
}

func TestLetter(t *testing.T) {
	for xy, want := range map[string]string{".M": "M", "M.": "M", "MM": "M", "A.": "A", "AM": "A", ".D": "D", "D.": "D", "R.": "R", "C.": "R", ".T": "M", "x": "M"} {
		if got := letter(xy); got != want {
			t.Errorf("letter(%q) = %q, want %q", xy, got, want)
		}
	}
}

func TestParseRevParse(t *testing.T) {
	root, prefix, ok := ParseRevParse([]byte("true\n/x/repo\nsub/dir/\n"))
	if !ok || root != "/x/repo" || prefix != "sub/dir/" {
		t.Errorf("%q %q %v", root, prefix, ok)
	}
	root, prefix, ok = ParseRevParse([]byte("true\r\nC:/r\r\n\r\n"))
	if !ok || root != "C:/r" || prefix != "" {
		t.Errorf("top: %q %q %v", root, prefix, ok)
	}
	if _, _, ok := ParseRevParse([]byte("false\n")); ok {
		t.Error("inside .git counted as a work tree")
	}
	if _, _, ok := ParseRevParse(nil); ok {
		t.Error("no output counted as a work tree")
	}
}

func TestQueryFallbacks(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	noGit := func(context.Context, string, ...string) ([]byte, error) { return nil, ErrNoGit }
	if r := Query(ctx, dir, noGit); r.Repo || !r.NoGit {
		t.Errorf("no git: %+v", r)
	}
	fails := func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("exit 128") }
	if r := Query(ctx, dir, fails); r.Repo || r.NoGit {
		t.Errorf("not a repo: %+v", r)
	}
	called := false
	spy := func(context.Context, string, ...string) ([]byte, error) { called = true; return nil, nil }
	if r := Query(ctx, filepath.Join(dir, "missing"), spy); r.Repo || called {
		t.Errorf("a missing folder ran git (%v) or is a repo: %+v", called, r)
	}
	f := filepath.Join(dir, "a.zip")
	os.WriteFile(f, []byte("x"), 0o644)
	if r := Query(ctx, filepath.Join(f, "inner"), spy); r.Repo || called {
		t.Error("a path inside a file ran git")
	}
}

// TestQueryRealRepo builds a small repository in a temporary folder and checks what Query reports there.
func TestQueryRealRepo(t *testing.T) {
	bin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "none"), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	repo := filepath.Join(dir, "repo")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir, cmd.Env = repo, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(rel, s string) {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(repo, 0o755)
	git("init", "-q")
	git("checkout", "-q", "-b", "work")
	write(".gitignore", "build/\n*.log\n")
	write("sub/a", "a\n")
	write("keep", "k\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	write("sub/a", "changed\n")
	write("sub/new", "n\n")
	write("sub/x.log", "l\n")
	write("build/out", "o\n")
	write("staged", "s\n")
	git("add", "staged")

	run := func(ctx context.Context, d string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Dir, cmd.Env = d, env
		return cmd.Output()
	}
	ctx := context.Background()
	r := Query(ctx, repo, run)
	if !r.Repo || r.Branch != "work" || !r.Dirty || r.Oid == "" || r.Root == "" {
		t.Fatalf("top: %+v", r)
	}
	for k, v := range map[string]string{"sub": "M", "build": "!", "staged": "A"} {
		if r.Files[k] != v {
			t.Errorf("top %s = %q, want %q (%v)", k, r.Files[k], v, r.Files)
		}
	}
	if _, ok := r.Files["keep"]; ok {
		t.Error("an unchanged file has a status")
	}
	r = Query(ctx, filepath.Join(repo, "sub"), run)
	for k, v := range map[string]string{"a": "M", "new": "?", "x.log": "!"} {
		if r.Files[k] != v {
			t.Errorf("sub %s = %q, want %q (%v)", k, r.Files[k], v, r.Files)
		}
	}
	if r = Query(ctx, filepath.Join(repo, "build"), run); r.All != "!" {
		t.Errorf("build: All = %q", r.All)
	}
	if r = Query(ctx, filepath.Join(repo, ".git"), run); r.Repo {
		t.Error("the .git folder counted as a work tree")
	}
	outside := filepath.Join(dir, "plain")
	os.MkdirAll(outside, 0o755)
	if r = Query(ctx, outside, run); r.Repo {
		t.Errorf("a plain folder counted as a repository: %+v", r)
	}
}
