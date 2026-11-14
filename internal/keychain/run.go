package keychain

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// runResult is what a helper program did: its output and its exit code (-1 when it could not be started).
type runResult struct {
	stdout, stderr string
	code           int
}

// runner runs a helper program with stdin as its standard input. The macOS and Linux stores are reached through one;
// tests put a fake in its place, so the commands are checked without running anything.
type runner func(program string, args []string, stdin string) (runResult, error)

// runTimeout bounds a helper program: a store that waits on something (a locked keychain asking to be unlocked) must
// not hang the connection that needs the secret for ever.
const runTimeout = 2 * time.Minute

// execRunner runs the program for real.
func execRunner(program string, args []string, stdin string) (runResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	r := runResult{stdout: out.String(), stderr: errb.String()}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		r.code = ee.ExitCode()
		err = nil
	default:
		r.code = -1
	}
	if ctx.Err() != nil {
		return r, errors.New("keychain: the system keychain did not answer in time")
	}
	return r, err
}

// firstLine is the first non-empty line of a helper's message, for an error.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}
