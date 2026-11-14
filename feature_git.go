package main

// The git status of the panels (board issue #23, F-NEW-7): in a git work tree the page shows a status letter beside
// each name and the branch in the path bar. This bound object only runs the git program for the page; reading its
// output is internal/gitstat's job. The page asks only while the feature is turned on (it is off by default), so
// nothing runs git unless the user wants it.
//
// git runs with GIT_OPTIONAL_LOCKS=0 (a status never takes the index lock away from the user's own git commands) and
// with the file system monitor hook turned off, so looking at a folder never starts a program a repository's own
// config names. Every run has a time limit, so a huge or slow repository leaves the panel as it is instead of hanging.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	"mormicommander/internal/gitstat"
)

// Git is what the page calls for the git status of a folder (window.go.main.Git).
type Git struct{ app *App }

// gitTimeout is how long one git run may take.
const gitTimeout = 8 * time.Second

func init() {
	registerFeature("git", func(a *App) []exposed { return []exposed{expose(&Git{app: a})} })
}

// Status tells whether the folder is in a git work tree, its branch, and the status of each of its entries.
func (g *Git) Status(path string) gitstat.Result {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	return gitstat.Query(ctx, path, runGit)
}

// runGit runs the git program in dir and returns its standard output.
func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, gitstat.ErrNoGit
	}
	full := append([]string{"-c", "core.fsmonitor=false", "-c", "core.quotepath=false"}, args...)
	cmd := exec.CommandContext(ctx, bin, full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, ctx.Err()
	}
	return out, err
}
