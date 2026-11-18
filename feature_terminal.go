package main

// The embedded terminal pane (issue #31, F-856): a pane below the panels running the user's shell on a pseudo terminal
// (internal/ptyterm), drawn by xterm.js in the page (features/terminal.js). It is off by default and only starts when
// the user opens it. What the user types goes to the shell as it is; the app itself only ever sends a "cd" to the
// folder the user opened in the active panel (while the shell sits at its prompt) and names the user asked to insert,
// never ending that line. Nothing here is reachable from the AI tools (features/aitools-allow.js lists the pane's
// commands as refused).
//
// The shell's output reaches the page as "terminal-data" events (the terminal's id and the bytes in base64, since a
// read can end inside a UTF-8 character), and "terminal-exit" (id, exit code) when the shell ends.

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"mormicommander/internal/ptyterm"
)

// Terminal is what the page calls for the terminal pane (window.go.main.Terminal).
type Terminal struct {
	app  *App
	emit func(event string, data ...any)

	shell string   // the shell to run ("" = the user's); tests set it
	env   []string // its environment (nil = the app's own); tests set it

	mu   sync.Mutex
	next int
	sess map[int]*ptyterm.Session
}

var terminalFeature *Terminal

func init() {
	registerFeature("terminal", func(a *App) []exposed {
		terminalFeature = newTerminal(a)
		return []exposed{expose(terminalFeature)}
	})
	onShutdown(func(a *App) {
		if terminalFeature != nil {
			terminalFeature.closeAll()
		}
	})
}

func newTerminal(a *App) *Terminal {
	return &Terminal{app: a, emit: a.emit, sess: map[int]*ptyterm.Session{}}
}

// Available tells whether the embedded terminal works on this system (not on Windows yet).
func (t *Terminal) Available() bool { return runtime.GOOS != "windows" }

// startDir is the folder to start in: dir when it is a local folder that exists, else the home folder.
func startDir(dir string) string {
	if filepath.IsAbs(dir) {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "/"
}

// Start starts the user's shell in dir (the home folder when dir is not a local folder) on a terminal of cols x rows,
// and returns the terminal's id.
func (t *Terminal) Start(dir string, cols, rows int) (int, error) {
	if !t.Available() {
		return 0, ptyterm.ErrUnsupported
	}
	s, err := ptyterm.Start(ptyterm.Options{Shell: t.shell, Env: t.env, Dir: startDir(dir), Cols: cols, Rows: rows})
	if err != nil {
		return 0, err
	}
	t.mu.Lock()
	t.next++
	id := t.next
	t.sess[id] = s
	t.mu.Unlock()
	go t.pump(id, s)
	return id, nil
}

// pump sends what the shell prints to the page until the shell ends.
func (t *Terminal) pump(id int, s *ptyterm.Session) {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.Read(buf)
		if n > 0 {
			t.emit("terminal-data", id, base64.StdEncoding.EncodeToString(buf[:n]))
		}
		if err != nil {
			break
		}
	}
	<-s.Done()
	t.mu.Lock()
	if t.sess[id] == s {
		delete(t.sess, id)
	}
	t.mu.Unlock()
	s.Close()
	t.emit("terminal-exit", id, s.ExitCode())
}

var errNoTerminal = errors.New("the terminal has ended")

func (t *Terminal) get(id int) (*ptyterm.Session, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s := t.sess[id]; s != nil {
		return s, nil
	}
	return nil, errNoTerminal
}

// Write sends what the user typed in the pane to the shell.
func (t *Terminal) Write(id int, data string) error {
	s, err := t.get(id)
	if err != nil {
		return err
	}
	_, err = s.Write([]byte(data))
	return err
}

// Resize tells the shell the pane's new size in characters.
func (t *Terminal) Resize(id, cols, rows int) error {
	s, err := t.get(id)
	if err != nil {
		return err
	}
	return s.Resize(cols, rows)
}

// Busy tells whether a program (not the shell at its prompt) runs in the terminal.
func (t *Terminal) Busy(id int) bool {
	s, err := t.get(id)
	return err == nil && s.Busy()
}

// Cd makes the shell change to dir, the folder the active panel shows. It returns false, and sends nothing, when a
// program runs in the terminal or dir is not a local folder.
func (t *Terminal) Cd(id int, dir string) (bool, error) {
	s, err := t.get(id)
	if err != nil {
		return false, err
	}
	if st, e := os.Stat(dir); e != nil || !st.IsDir() || !filepath.IsAbs(dir) {
		return false, nil
	}
	if err := s.Cd(dir); err != nil {
		if errors.Is(err, ptyterm.ErrBusy) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Insert puts the names (quoted for the shell) at the prompt, without pressing Enter.
func (t *Terminal) Insert(id int, names []string) error {
	s, err := t.get(id)
	if err != nil {
		return err
	}
	return s.Insert(names)
}

// Close ends the shell and the programs started in it.
func (t *Terminal) Close(id int) error {
	t.mu.Lock()
	s := t.sess[id]
	delete(t.sess, id)
	t.mu.Unlock()
	if s != nil {
		s.Close()
	}
	return nil
}

func (t *Terminal) closeAll() {
	t.mu.Lock()
	all := t.sess
	t.sess = map[int]*ptyterm.Session{}
	t.mu.Unlock()
	var wg sync.WaitGroup
	for _, s := range all {
		wg.Add(1)
		go func() { defer wg.Done(); s.Close() }()
	}
	wg.Wait()
}
