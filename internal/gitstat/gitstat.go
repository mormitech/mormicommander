// Package gitstat tells what git thinks of the files of one folder: whether the folder is inside a git work tree, the
// branch it is on (with ahead / behind counts against its upstream and whether anything is changed), and a one-letter
// status for each entry of the folder (M changed, A added, D deleted, R renamed, U conflict, ? untracked, ! ignored).
// It reads the git command line program's machine output (git status --porcelain=v2), so it needs git installed; with
// no git, or outside a work tree, the answer is simply "not a repository". There is no UI here: the panel feature in
// the root package (feature_git.go) runs the program and the page draws the result.
package gitstat

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
)

// Result is what the panels need to know about one folder.
type Result struct {
	Repo     bool              `json:"repo"`     // the folder is inside a git work tree
	NoGit    bool              `json:"noGit"`    // git is not installed (Repo is false then)
	Root     string            `json:"root"`     // the top folder of the work tree
	Branch   string            `json:"branch"`   // the branch name; empty when detached
	Detached bool              `json:"detached"` // HEAD is not on a branch
	Oid      string            `json:"oid"`      // the commit HEAD is on, shortened; empty before the first commit
	Upstream string            `json:"upstream"` // the upstream branch ("origin/main"); empty when none
	Ahead    int               `json:"ahead"`    // commits on the branch that the upstream does not have
	Behind   int               `json:"behind"`   // commits on the upstream that the branch does not have
	Dirty    bool              `json:"dirty"`    // something in the work tree is changed, added, deleted or untracked
	All      string            `json:"all"`      // a status every entry of the folder has (the folder itself is ignored or untracked)
	Files    map[string]string `json:"files"`    // entry name → status letter, for the entries that have one
}

// Runner runs git with the given arguments in dir and returns what it printed on standard output.
type Runner func(ctx context.Context, dir string, args ...string) ([]byte, error)

// ErrNoGit is what a Runner returns when the git program cannot be found.
var ErrNoGit = errors.New("git is not installed")

// Query asks git about the folder dir. Errors from git (not a repository, a repository git refuses to read, a timeout)
// all come back as a Result with Repo false: the panels then just show nothing extra.
func Query(ctx context.Context, dir string, run Runner) Result {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return Result{} // not a real folder (inside an archive, on a server, gone)
	}
	out, err := run(ctx, dir, "rev-parse", "--is-inside-work-tree", "--show-toplevel", "--show-prefix")
	if errors.Is(err, ErrNoGit) {
		return Result{NoGit: true}
	}
	if err != nil {
		return Result{}
	}
	root, prefix, ok := ParseRevParse(out)
	if !ok {
		return Result{}
	}
	out, err = run(ctx, dir, "status", "--porcelain=v2", "--branch", "--ignored", "-z")
	if err != nil {
		return Result{}
	}
	r := ParseStatus(out, prefix)
	r.Root = root
	return r
}

// ParseRevParse reads the output of "git rev-parse --is-inside-work-tree --show-toplevel --show-prefix": the top
// folder and the folder's path below it ("sub/dir/", empty at the top). ok is false outside a work tree.
func ParseRevParse(out []byte) (root, prefix string, ok bool) {
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n"), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "true" {
		return "", "", false
	}
	root = lines[1]
	if len(lines) > 2 {
		prefix = lines[2]
	}
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return root, prefix, true
}

// ParseStatus reads the output of "git status --porcelain=v2 --branch --ignored -z" for the whole work tree and keeps
// the status of the entries directly inside the folder at prefix (the path below the top folder, ending in "/", or
// empty for the top folder). A subfolder with changes somewhere inside it gets M (U when one of them is a conflict).
func ParseStatus(out []byte, prefix string) Result {
	r := Result{Repo: true, Files: map[string]string{}}
	recs := bytes.Split(out, []byte{0})
	for i := 0; i < len(recs); i++ {
		rec := string(recs[i])
		if rec == "" {
			continue
		}
		var code, path string
		switch rec[0] {
		case '#':
			r.header(rec)
			continue
		case '1':
			code, path = entry(rec, 8)
		case '2':
			code, path = entry(rec, 9)
			i++ // the original name of a rename or copy follows as its own record; it is not in the folder any more
		case 'u':
			code, path = "U", field(rec, 10)
		case '?':
			code, path = "?", rec[min(2, len(rec)):]
		case '!':
			code, path = "!", rec[min(2, len(rec)):]
		default:
			continue
		}
		if path == "" {
			continue
		}
		if code != "!" {
			r.Dirty = true
		}
		r.place(code, path, prefix)
	}
	return r
}

// header reads one "# branch.…" line.
func (r *Result) header(rec string) {
	k, v, _ := strings.Cut(strings.TrimPrefix(rec, "# "), " ")
	switch k {
	case "branch.oid":
		if v != "(initial)" {
			r.Oid = v[:min(7, len(v))]
		}
	case "branch.head":
		if v == "(detached)" {
			r.Detached = true
		} else {
			r.Branch = v
		}
	case "branch.upstream":
		r.Upstream = v
	case "branch.ab":
		for _, f := range strings.Fields(v) {
			n, _ := strconv.Atoi(f[1:])
			if strings.HasPrefix(f, "+") {
				r.Ahead = n
			} else if strings.HasPrefix(f, "-") {
				r.Behind = n
			}
		}
	}
}

// place gives the entry of the folder that path (relative to the top folder) is, or is inside, its status.
func (r *Result) place(code, path, prefix string) {
	if !strings.HasPrefix(path, prefix) {
		return
	}
	rest := path[len(prefix):]
	trimmed := strings.TrimSuffix(rest, "/")
	if trimmed == "" {
		// the folder itself is untracked or ignored as a whole ("? dir/", "! build/"): so is all it holds
		r.All = code
		return
	}
	name, deeper, inside := strings.Cut(trimmed, "/")
	if !inside || deeper == "" {
		r.Files[name] = code
		return
	}
	// something inside the subfolder name; an ignored file inside a tracked folder says nothing about the folder
	switch {
	case code == "!":
	case code == "U":
		r.Files[name] = "U"
	case r.Files[name] != "U":
		r.Files[name] = "M"
	}
}

// entry reads an ordinary or rename record: the letter for its XY field and the path after n space-separated fields.
func entry(rec string, n int) (code, path string) {
	parts := strings.SplitN(rec, " ", n+1)
	if len(parts) < n+1 {
		return "", ""
	}
	return letter(parts[1]), parts[n]
}

// field returns what follows the first n space-separated fields of rec (the path, which may hold spaces).
func field(rec string, n int) string {
	parts := strings.SplitN(rec, " ", n+1)
	if len(parts) < n+1 {
		return ""
	}
	return parts[n]
}

// letter turns git's two-letter XY status (index, work tree) into the one letter the panel shows.
func letter(xy string) string {
	if len(xy) != 2 {
		return "M"
	}
	x, y := xy[0], xy[1]
	switch {
	case x == 'D' || y == 'D':
		return "D"
	case x == 'A':
		return "A"
	case x == 'R' || x == 'C':
		return "R"
	default:
		return "M"
	}
}
