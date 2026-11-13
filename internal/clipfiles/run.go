// This file runs the helper programs the macOS and Linux backends use (osascript, wl-copy, wl-paste, xclip, xsel)
// behind a small interface, so tests can replace them with a fake and check the exact commands and data without
// touching the user's real clipboard. Data always travels on stdin and stdout, never on the command line, so no file
// name can be read as an option or a script and long selections do not hit the argument length limit. Linux clipboard
// tools that keep serving the selection fork into the background after reading their input; serve gives them a temp
// file for stderr and no stdout pipe, because Go would otherwise wait for the background child to close the pipe and
// hang until something else took the clipboard.

package clipfiles

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runner starts helper programs. It exists so tests can fake them.
type runner interface {
	// output runs name with args, feeding stdin, and returns what it wrote to stdout.
	output(stdin []byte, name string, args ...string) ([]byte, error)
	// serve runs a program that may fork a background child to keep owning the clipboard. It returns once the
	// foreground process has exited.
	serve(stdin []byte, name string, args ...string) error
	// has says whether name can be found on PATH.
	has(name string) bool
}

// toolTimeout bounds every helper run. Reading the clipboard can wait for a privacy prompt on newer macOS versions,
// so the limit is generous; it only exists so a wedged tool cannot hang the UI forever.
const toolTimeout = 30 * time.Second

// execRunner is the real runner. env entries are added to the inherited environment.
type execRunner struct {
	env []string
}

func (r execRunner) command(ctx context.Context, stdin []byte, name string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	if len(r.env) > 0 {
		cmd.Env = append(os.Environ(), r.env...)
	}
	return cmd
}

func (r execRunner) output(stdin []byte, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	cmd := r.command(ctx, stdin, name, args)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), toolError(name, err, errb.Bytes())
	}
	return out.Bytes(), nil
}

func (r execRunner) serve(stdin []byte, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	cmd := r.command(ctx, stdin, name, args)
	errf, err := os.CreateTemp("", "mormicommander-clip-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(errf.Name())
	defer errf.Close()
	cmd.Stderr = errf // a real file: Go hands over the descriptor and does not wait for the child to close it
	if err := cmd.Run(); err != nil {
		msg, _ := os.ReadFile(errf.Name())
		return toolError(name, err, msg)
	}
	return nil
}

func (execRunner) has(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// toolErr is a failed helper run. It keeps the tool's own message, which is usually the most useful part.
type toolErr struct {
	tool string
	err  error
	msg  string
}

func (e *toolErr) Error() string {
	if e.msg == "" {
		return fmt.Sprintf("%s: %v", e.tool, e.err)
	}
	return fmt.Sprintf("%s: %s", e.tool, e.msg)
}

func (e *toolErr) Unwrap() error { return e.err }

func toolError(tool string, err error, stderr []byte) error {
	return &toolErr{tool: tool, err: err, msg: strings.TrimSpace(string(stderr))}
}
