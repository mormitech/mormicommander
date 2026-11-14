package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Control mode: mormiCommander started with `--control PORT` (or MORMICOMMANDER_CONTROL=PORT; 0 takes a free port)
// listens on 127.0.0.1 for commands from another program, so every feature can be driven from outside: the QA harness,
// scripted tests, demo screenshots. Nothing of it exists without the flag. The commands are JSON (docs/control.md):
// the ones about the window are done here, the rest go to the page, which runs them with the very code the keys and
// the mouse use (frontend/src/core/control.js). Never does it move the system's pointer or press real keys.
//
// Every request needs the token that is written, with the address, to the control file (--control-file,
// MORMICOMMANDER_CONTROL_FILE, or mormicommander-control.json in the temporary folder; readable by the user only).
// Requests with an Origin header or for another Host are refused, so a web page can't reach it.
//
// Dialogs drawn by the page are answered with the page's "dialog" commands. System dialogs (folder pickers and the
// like) can't be clicked from outside, so in this mode they are answered from a queue ("answer"); a question with no
// answer queued gets the safe answer (Cancel, No).

type control struct {
	app     *App
	token   string
	addr    string
	file    string
	mu      sync.Mutex
	seq     int
	waiting map[int]chan string
	answers map[string][]string
	asked   []string
	srv     *http.Server
}

func newToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// controlFile is where the address and token are written.
func controlFile(o startOpts) string {
	if o.ControlFile != "" {
		return o.ControlFile
	}
	if f := os.Getenv("MORMICOMMANDER_CONTROL_FILE"); f != "" {
		return f
	}
	return filepath.Join(os.TempDir(), "mormicommander-control.json")
}

// startControl starts the command server and writes the control file. It returns the address (127.0.0.1:port).
func (a *App) startControl(port string) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return "", err
	}
	c := &control{app: a, token: newToken(), addr: ln.Addr().String(), waiting: map[int]chan string{}, answers: map[string][]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/cmd", c.guard(c.handleCmd))
	mux.HandleFunc("/batch", c.guard(c.handleBatch))
	mux.HandleFunc("/info", c.guard(c.handleInfo))
	c.srv = &http.Server{Handler: mux}
	go c.srv.Serve(ln)
	a.mu.Lock()
	a.ctl = c
	a.mu.Unlock()
	c.file = controlFile(a.opts)
	info, _ := json.Marshal(map[string]any{"url": "http://" + c.addr, "token": c.token, "pid": os.Getpid()})
	if err := writePrivateFile(c.file, info); err != nil {
		return c.addr, err
	}
	return c.addr, nil
}

// writePrivateFile puts data at name as a new file only the user can read. The control file's default place is the
// shared temporary folder (/tmp on Linux), where another user can make the file first: os.WriteFile would keep that
// file's permissions (the token readable by everyone) and follow a link there (overwriting the file it points to). A
// new temporary file (always private) renamed over the name replaces whatever is there instead, and the rename fails
// on another user's file in a sticky folder such as /tmp.
func writePrivateFile(name string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(name), ".mormicommander-control-*")
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(f.Name(), name)
	}
	if werr != nil {
		os.Remove(f.Name())
	}
	return werr
}

// stopControl ends the server and removes the control file (when it is still ours).
func (a *App) stopControl() {
	a.mu.Lock()
	c := a.ctl
	a.ctl = nil
	a.mu.Unlock()
	if c == nil {
		return
	}
	c.srv.Close()
	if b, err := os.ReadFile(c.file); err == nil {
		var info struct {
			Token string `json:"token"`
		}
		if json.Unmarshal(b, &info) == nil && info.Token == c.token {
			os.Remove(c.file)
		}
	}
}

// guard lets a request through when it has the token, comes to the address of this program and was not made by a web
// page.
func (c *control) guard(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if r.Header.Get("Origin") != "" || (host != "127.0.0.1" && host != "localhost") {
			http.Error(w, "refused", http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+c.token)) != 1 {
			http.Error(w, "token missing or wrong", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (c *control) handleCmd(w http.ResponseWriter, r *http.Request) {
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeJSON(w, 400, map[string]any{"error": "not JSON: " + err.Error()})
		return
	}
	res, err := c.do(m)
	if err != nil {
		writeJSON(w, 200, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

// handleBatch runs a list of commands one after the other and stops at the first that fails (unless "continue" is in
// the query).
func (c *control) handleBatch(w http.ResponseWriter, r *http.Request) {
	var list []map[string]any
	if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
		writeJSON(w, 400, map[string]any{"error": "not a JSON list: " + err.Error()})
		return
	}
	out := make([]any, 0, len(list))
	for i, m := range list {
		res, err := c.do(m)
		if err != nil {
			out = append(out, map[string]any{"error": err.Error(), "at": i})
			if r.URL.Query().Get("continue") == "" {
				break
			}
			continue
		}
		out = append(out, res)
	}
	writeJSON(w, 200, out)
}

func (c *control) handleInfo(w http.ResponseWriter, r *http.Request) {
	res, _ := c.do(map[string]any{"cmd": "info"})
	writeJSON(w, 200, res)
}

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

func num(m map[string]any, k string, def int) int {
	if f, ok := m[k].(float64); ok {
		return int(f)
	}
	return def
}

// waitUntil waits at least minWait, then checks cond every step until it holds or maxWait has passed since the start.
// It answers whether cond held. Control mode uses it for things the system does a moment after it is asked (a window
// appearing), so a command answers once the result can be seen rather than after a guessed pause.
func waitUntil(cond func() bool, minWait, maxWait, step time.Duration) bool {
	start := time.Now()
	time.Sleep(minWait)
	for {
		if cond() {
			return true
		}
		if time.Since(start) >= maxWait {
			return false
		}
		time.Sleep(step)
	}
}

// do runs one command: the ones about the window here, registered Go commands next, anything else in the page.
func (c *control) do(m map[string]any) (map[string]any, error) {
	a := c.app
	switch str(m, "cmd") {
	case "info":
		x, y, w, h, maxed := 0, 0, 0, 0, false
		if win := a.mainWindow(); win != nil {
			x, y = win.Position()
			w, h = win.Size()
			maxed = win.IsMaximised()
		}
		return map[string]any{"ok": true, "version": version(), "bundleId": bundleID(), "pid": os.Getpid(), "windowId": a.windowID(), "x": x, "y": y, "w": w, "h": h,
			"demo": a.mock != nil, "platform": runtime.GOOS, "features": featureNames(), "settings": settingsPath(), "title": a.windowTitle(),
			"maximised": maxed, "windows": a.windowNames()}, nil
	case "menu": // what choosing a menu item does: {name: "cm_MatchSrc"} (the native menu can't be clicked from outside)
		a.emit("menu", str(m, "name"))
		time.Sleep(time.Duration(num(m, "ms", 300)) * time.Millisecond)
		return c.toPage(map[string]any{"cmd": "idle"})
	case "window": // size and place, for screenshots that look alike (centred when no x and y); max: true maximises;
		// window: "lister-2" sizes another window of the app (the answer is then that window's entry of "windows")
		win := a.mainWindow()
		other := str(m, "window")
		if other != "" {
			if win = a.windowByName(other); win == nil {
				return nil, fmt.Errorf("no window %q", other)
			}
		}
		if win == nil {
			return nil, errors.New("no window yet")
		}
		// Maximising comes after the size and place: setting a size un-maximises a window (on macOS a zoomed window
		// stops being zoomed when its frame changes), so {w, h, max: true} must end maximised, with w x h as the size it
		// comes back to. Un-maximising comes first, so the size asked for is the one the window gets.
		maxV, hasMax := m["max"].(bool)
		if hasMax && !maxV && win.IsMaximised() {
			win.ToggleMaximise()
		}
		if w, h := num(m, "w", 0), num(m, "h", 0); w > 0 && h > 0 {
			win.SetSize(w, h)
			if _, ok := m["x"]; ok {
				win.SetPosition(num(m, "x", 0), num(m, "y", 0))
			} else {
				win.Center()
			}
		}
		if hasMax && maxV && !win.IsMaximised() {
			if win == a.mainWindow() {
				a.SaveWindow() // (the size to come back to, as when the user maximises with cm_Maximize)
			}
			win.ToggleMaximise()
		}
		time.Sleep(300 * time.Millisecond)
		if other != "" {
			for _, e := range a.windowList() {
				if e["name"] == other {
					return map[string]any{"ok": true, "window": e}, nil
				}
			}
		}
		return c.do(map[string]any{"cmd": "info"})
	case "nativemenu": // the native menu bar as it is built (labels, check marks, greyed items), to check SetMenu
		a.mu.Lock()
		m := a.menuBar
		a.mu.Unlock()
		return map[string]any{"ok": true, "menus": nativeMenuTree(m)}, nil
	case "about": // open (or close: {close: true}) the About window, which is a window of this process
		aboutNow := func() *application.WebviewWindow {
			a.mu.Lock()
			defer a.mu.Unlock()
			return a.about
		}
		shut, _ := m["close"].(bool)
		if shut {
			if w := aboutNow(); w != nil {
				w.Close()
			}
		} else {
			a.ShowAbout()
		}
		// The window is shown (or closed) by the system a moment later, so a fixed pause sometimes answered before it
		// was on the screen and a "windows" right after saw it hidden. Wait at least "ms", then until the window is
		// visible (or gone), for at most "timeout" more.
		minWait := time.Duration(num(m, "ms", 500)) * time.Millisecond
		maxWait := minWait + time.Duration(num(m, "timeout", 5000))*time.Millisecond
		settled := waitUntil(func() bool {
			w := aboutNow()
			if shut {
				return w == nil
			}
			return w != nil && w.IsVisible()
		}, minWait, maxWait, 50*time.Millisecond)
		w := aboutNow()
		return map[string]any{"ok": true, "windows": a.windowNames(), "windowId": windowNumber(w),
			"visible": w != nil && w.IsVisible(), "settled": settled}, nil
	case "screenshot": // the window as a PNG file: {path}; window: "lister-2" takes another window of the app;
		// dialogs: false leaves the window's dialog windows out of the picture
		dialogs, set := m["dialogs"].(bool)
		return c.screenshot(str(m, "path"), str(m, "window"), dialogs || !set)
	case "answer": // the answer for the next system dialog of a kind: "value" or "values"
		kind := str(m, "kind")
		c.mu.Lock()
		defer c.mu.Unlock()
		if v, ok := m["values"].([]any); ok {
			for _, x := range v {
				c.answers[kind] = append(c.answers[kind], fmt.Sprint(x))
			}
		} else {
			c.answers[kind] = append(c.answers[kind], str(m, "value"))
		}
		return map[string]any{"ok": true, "queued": len(c.answers[kind])}, nil
	case "asked": // the system dialogs asked since the last time
		c.mu.Lock()
		defer c.mu.Unlock()
		asked := c.asked
		c.asked = nil
		return map[string]any{"ok": true, "asked": asked}, nil
	case "sleep":
		time.Sleep(time.Duration(num(m, "ms", 100)) * time.Millisecond)
		return map[string]any{"ok": true}, nil
	case "quit": // without asking anything
		if a.wapp != nil {
			go func() { time.Sleep(100 * time.Millisecond); a.Quit() }()
		}
		return map[string]any{"ok": true}, nil
	case "":
		return nil, errors.New("no cmd")
	}
	featMu.Lock()
	f := controlCommands[str(m, "cmd")]
	featMu.Unlock()
	if f != nil {
		return f(a, m)
	}
	return c.toPage(m)
}

// screenshot saves the window as a PNG. On macOS it uses `screencapture -l` with the window's number (the window may
// be behind others). Other systems: the script makes the picture itself (scripts/mc_ctl.py shot()). It first waits for
// the page and the window's dialog windows to be painted, and lays those dialog windows over the picture unless
// dialogs is false (control_shot.go).
func (c *control) screenshot(path, window string, dialogs bool) (map[string]any, error) {
	if path == "" {
		return nil, errors.New("give a path for the PNG")
	}
	if runtime.GOOS != "darwin" {
		return nil, errors.New("screenshots from inside the app work on macOS only")
	}
	owner := mainWindowName
	if window != "" {
		owner = window
	}
	c.waitForPaint(owner)
	wid := c.app.windowID()
	if window != "" {
		w := c.app.windowByName(window)
		if w == nil {
			return nil, fmt.Errorf("no window %q", window)
		}
		wid = windowNumber(w)
	}
	if wid == 0 {
		return nil, errors.New("the window is not on the screen")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	time.Sleep(150 * time.Millisecond) // the last change is painted
	if err := captureWindow(wid, path); err != nil {
		return nil, err
	}
	res := map[string]any{"ok": true, "path": path}
	if dialogs {
		n, err := c.addDialogs(path, owner)
		if err != nil {
			return nil, err
		}
		res["dialogs"] = n
	}
	if st, err := os.Stat(path); err == nil {
		res["bytes"] = st.Size()
	}
	return res, nil
}

func (a *App) windowTitle() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.title
}

// toPage gives a command to the page and waits for its answer.
func (c *control) toPage(m map[string]any) (map[string]any, error) {
	c.mu.Lock()
	c.seq++
	id := c.seq
	ch := make(chan string, 1)
	c.waiting[id] = ch
	c.mu.Unlock()
	c.app.emit("control", id, m)
	timeout := time.Duration(num(m, "timeout", 30)) * time.Second
	select {
	case s := <-ch:
		var res map[string]any
		if err := json.Unmarshal([]byte(s), &res); err != nil {
			return nil, fmt.Errorf("the page answered something else: %s", s)
		}
		if e, ok := res["error"].(string); ok && e != "" {
			return nil, errors.New(e)
		}
		return res, nil
	case <-time.After(timeout):
		c.mu.Lock()
		delete(c.waiting, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("the page did not answer %q in %v", str(m, "cmd"), timeout)
	}
}

// ControlReply is the page's answer to a control command (JSON).
func (a *App) ControlReply(id int, result string) {
	a.mu.Lock()
	c := a.ctl
	a.mu.Unlock()
	if c == nil {
		return
	}
	c.mu.Lock()
	ch := c.waiting[id]
	delete(c.waiting, id)
	c.mu.Unlock()
	if ch != nil {
		ch <- result
	}
}

// mockAnswer is what a system dialog asks first: in control mode the answer comes from the queue ("" when there is
// none, which every dialog takes as Cancel or No), and the question is noted. ok is false outside control mode, and
// the system's dialog is shown.
func (a *App) mockAnswer(kind, detail string) (answer string, ok bool) {
	a.mu.Lock()
	c := a.ctl
	a.mu.Unlock()
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.asked = append(c.asked, kind+": "+detail)
	if q := c.answers[kind]; len(q) > 0 {
		answer, c.answers[kind] = q[0], q[1:]
	}
	return answer, true
}

// PickFolder shows the system's folder picker (or takes the queued "folder" answer in control mode). "" means
// cancelled.
func (a *App) PickFolder(title, start string) (string, error) {
	if v, ok := a.mockAnswer("folder", start); ok {
		return v, nil
	}
	return a.pickFolderDialog(title, start)
}

// windowNames lists the open windows by name ("main", "about"), for the control mode.
func (a *App) windowNames() []string {
	out := []string{}
	if a.wapp == nil {
		return out
	}
	for _, w := range a.wapp.Window.GetAll() {
		out = append(out, w.Name())
	}
	return out
}

// nativeMenuTree is the menu bar as the system got it, for the control mode's "nativemenu".
func nativeMenuTree(m *application.Menu) []map[string]any {
	out := []map[string]any{}
	for _, it := range menuItems(m) {
		e := map[string]any{"label": it.Label()}
		switch {
		case it.IsSeparator():
			e = map[string]any{"sep": true}
		case it.IsSubmenu():
			e["items"] = nativeMenuTree(it.GetSubmenu())
		default:
			if !it.Enabled() {
				e["disabled"] = true
			}
			if it.IsCheckbox() {
				e["checked"] = it.Checked()
			}
		}
		out = append(out, e)
	}
	return out
}
