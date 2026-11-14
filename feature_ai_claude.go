package main

// The Claude Code CLI as the AI layer's provider (issue #146): finding the program, asking its version, and running it
// for one prompt. The command line and the reading of its output are in internal/ai (ClaudeArgs, ParseLine).

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/ai"
)

// systemFinder looks for claude the way the user's system would: this process's PATH, the login shell's PATH, the
// places the installers use.
func (x *AI) systemFinder() ai.Finder {
	home, _ := os.UserHomeDir()
	return ai.Finder{GOOS: runtime.GOOS, Home: home, Env: os.Getenv, LookPath: exec.LookPath, LoginShell: x.loginShellClaude}
}

// loginShellClaude asks the user's login shell where claude is, once per run of the app: an app started from the Dock
// has a minimal PATH, and the shell is slow to start, so the answer is kept (also when it is "not found").
func (x *AI) loginShellClaude() string {
	x.shellOnce.Do(func() {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
			if runtime.GOOS == "darwin" {
				shell = "/bin/zsh"
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, shell, "-l", "-c", "command -v claude").Output()
		if err == nil {
			x.shellPath = ai.LastPathLine(string(out))
		}
	})
	return x.shellPath
}

// claudeVersion is the version claude reports, kept per program path ("" when it does not answer).
func (x *AI) claudeVersion(path string) string {
	x.mu.Lock()
	v, ok := x.versions[path]
	x.mu.Unlock()
	if ok {
		return v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = aiChildEnv(path)
	aiHide(cmd)
	out, err := cmd.Output()
	if err == nil {
		v = ai.VersionText(string(out))
	}
	x.mu.Lock()
	x.versions[path] = v
	x.mu.Unlock()
	return v
}

// aiChildEnv is the environment the CLI runs with: ours, with the program's own folder first in PATH (an npm install
// keeps node next to claude, which a minimal PATH would not find), and without the variables that tell Claude Code it
// runs inside another Claude Code session (when mormiCommander was itself started from one), which would change how
// it behaves.
func aiChildEnv(program string) []string {
	var env []string
	pathKey, pathVal := "PATH", ""
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		switch {
		case strings.EqualFold(k, "PATH"):
			pathKey, pathVal = k, v
			continue
		case k == "CLAUDECODE" || k == "CLAUDE_CODE_ENTRYPOINT":
			continue
		}
		env = append(env, kv)
	}
	if dir := filepath.Dir(program); filepath.IsAbs(program) {
		if pathVal == "" {
			pathVal = dir
		} else {
			pathVal = dir + string(os.PathListSeparator) + pathVal
		}
	}
	return append(env, pathKey+"="+pathVal)
}

// tailBuffer keeps the last bytes written to it: the end of the CLI's error output, for the message of a failed run.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}

// startClaude starts the CLI for one run and follows its output in the background.
func (x *AI) startClaude(program string, m *aiMCP, req AIRequest, model string) (int, error) {
	x.mu.Lock()
	x.runSeq++
	id := x.runSeq
	x.mu.Unlock()

	// The MCP config holds the token, so it is a file only the user can read (CreateTemp makes it 0600), and it is
	// removed when the run ends.
	f, err := os.CreateTemp("", "mormicommander-mcp-*.json")
	if err != nil {
		return 0, fmt.Errorf("The AI run could not be prepared: %v", err)
	}
	_, werr := f.Write(ai.MCPConfig(m.url(id), m.token))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(f.Name())
		return 0, fmt.Errorf("The AI run could not be prepared: %v", werr)
	}

	cmd := exec.Command(program, ai.ClaudeArgs(f.Name(), model, ai.SystemPrompt(req.Context))...)
	cmd.Dir = aiHome()
	cmd.Env = aiChildEnv(program)
	cmd.Stdin = strings.NewReader(req.Prompt)
	stderr := &tailBuffer{max: 4096}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.Remove(f.Name())
		return 0, fmt.Errorf("Claude could not be started: %v", err)
	}
	aiProcessGroup(cmd)
	r := &aiRun{id: id, cmd: cmd, ended: make(chan struct{}), exited: make(chan struct{}), cfgFile: f.Name()}
	// The run is known before the CLI starts, so its first MCP request is not refused.
	x.mu.Lock()
	x.runs[id] = r
	x.mu.Unlock()
	if err := cmd.Start(); err != nil {
		x.mu.Lock()
		delete(x.runs, id)
		x.mu.Unlock()
		os.Remove(f.Name())
		return 0, fmt.Errorf("Claude could not be started: %v", err)
	}
	go x.follow(r, stdout, stderr)
	return id, nil
}

// follow reads the CLI's output until it ends, sends the steps to the page, and ends the run: with the CLI's result
// when it gave one, else with what went wrong.
func (x *AI) follow(r *aiRun, stdout io.Reader, stderr *tailBuffer) {
	x.readStream(r, stdout)
	err := r.cmd.Wait()
	close(r.exited)
	os.Remove(r.cfgFile)
	msg := "Claude ended without an answer."
	if s := stderr.String(); s != "" {
		msg = "Claude ended without an answer: " + aiLastLines(s, 3)
	} else if err != nil {
		msg = fmt.Sprintf("Claude ended without an answer (%v).", err)
	}
	x.finish(r, ai.Progress{Kind: "error", Error: msg})
	x.mu.Lock()
	delete(x.runs, r.id)
	x.mu.Unlock()
}

// readStream sends the steps of each output line to the page; the final result ends the run. Lines are read whole
// whatever their length (a tool's answer echoed back can be long), and steps after the run ended (after Stop) are
// dropped, so the page never sees anything after the run's last event.
func (x *AI) readStream(r *aiRun, stdout io.Reader) {
	br := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			steps, final := ai.ParseLine(line)
			for _, st := range steps {
				if final {
					x.finish(r, st)
					continue
				}
				select {
				case <-r.ended:
				default:
					x.emit("ai-progress", r.id, st)
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// aiLastLines is the last n lines of a text, for an error message that fits in a dialog.
func aiLastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, " "))
}
