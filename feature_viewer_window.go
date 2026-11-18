// The Lister in a real window of its own (F3), like Total Commander's Lister. The page of the file manager asks
// ListerWindows.Open for a file; this file finds a window for it, the window's page (frontend/viewer.html, started by
// frontend/src/viewer-window.js) shows the file with the same view as quick view, and asks to be shown once the file is
// drawn. Several Lister windows can be open at once; each one is named "lister-<n>".
//
// Making a window and loading its page takes a while (about 70 ms on a Mac), so one hidden window is made in advance
// once the file manager's page is up (Prepare), and F3 only has to fill and show it (about 50 ms). As soon as that
// spare window is used, the next one is made in the background.
//
// N and P (next and previous file) are worked out by the window from the list of names it was given; it tells Go which
// file it shows now (Moved), Go sets the title and sends the file manager's page the "lister-cursor" event so the
// panel's cursor follows. Esc (or the window's close button) closes the window; its size is kept in the setting
// "listerWindow" and the next Lister window gets it. Its place is kept there too, and is used only with Options >
// Lister > "Open Lister windows where the last one was closed" (off by default: the window comes up centred).
//
// This is one of the few files that use the Wails API directly (with app.go, window.go, menu.go and shell.go): the
// features themselves never do. Control mode can list the windows ("windows") and look into a Lister window or drive it
// ("viewer", "viewer_key", "viewer_menu", "viewer_select", "viewer_dialog", "viewer_close"; see docs/control.md).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The Lister window's size when none was kept yet, and the smallest it can get.
const (
	listerWinW, listerWinH       = 900, 680
	listerWinMinW, listerWinMinH = 420, 300
	listerWindowPrefix           = "lister-"
)

// ListerRequest is what the file manager's page asks a Lister window to show.
type ListerRequest struct {
	Path         string   `json:"path"`         // the file
	Names        []string `json:"names"`        // the files N and P walk through (names in Dir)
	Dir          string   `json:"dir"`          // the folder of those names
	Sources      []string `json:"sources"`      // or, when given, the full paths N and P walk (in archives too; unpacked when reached)
	At           int      `json:"at"`           // the place of the file shown first in Sources (-1 for none)
	Side         string   `json:"side"`         // the panel the file came from ("left", "right" or "" for none)
	InternalOnly bool     `json:"internalOnly"` // text, binary and hex only (Alt+Shift+F3)
	Mode         string   `json:"mode"`         // the mode to start in ("auto" and the like)
	KeepPlace    bool     `json:"keepPlace"`    // open where the last Lister window was closed (Options > Lister), not centred
}

// listerWin is one Lister window as Go knows it.
type listerWin struct {
	w       *application.WebviewWindow
	name    string
	ready   bool           // its page is loaded and has asked for work
	req     *ListerRequest // the file it was asked to show (nil while it is the hidden spare)
	path    string         // the file it shows now (changes with N and P)
	shown   bool           // it has been shown (a spare has not)
	session int            // the Lister session (feature_lister.go) of the file it shows, closed with the window
	opened  time.Time
}

// ListerWindows is what the pages call about Lister windows (see the top of this file).
type ListerWindows struct {
	app     *App
	mu      sync.Mutex
	wins    map[string]*listerWin
	spare   *listerWin
	seq     int
	last    string // the Lister window opened last (control mode's default)
	ready   bool   // the file manager's page is up, so a spare may be made
	ctlSeq  int
	waiting map[int]chan string
}

var listerWindows *ListerWindows

func init() {
	registerFeature("listerwindows", func(a *App) []exposed {
		listerWindows = &ListerWindows{app: a, wins: map[string]*listerWin{}, waiting: map[int]chan string{}}
		return []exposed{expose(listerWindows)}
	})
	registerControl("windows", func(a *App, m map[string]any) (map[string]any, error) {
		return map[string]any{"ok": true, "windows": a.windowList()}, nil
	})
	for _, name := range []string{"viewer", "viewer_key", "viewer_menu", "viewer_select", "viewer_dialog", "viewer_popup"} {
		cmd := strings.TrimPrefix(strings.TrimPrefix(name, "viewer"), "_")
		if cmd == "" {
			cmd = "state"
		}
		registerControl(name, func(a *App, m map[string]any) (map[string]any, error) {
			return listerWindows.control(cmd, m)
		})
	}
	registerControl("viewer_close", func(a *App, m map[string]any) (map[string]any, error) {
		lw := listerWindows
		v, err := lw.pick(str(m, "window"))
		if err != nil {
			return nil, err
		}
		v.w.Close()
		time.Sleep(time.Duration(num(m, "ms", 300)) * time.Millisecond)
		return map[string]any{"ok": true, "closed": v.name, "windows": a.windowList()}, nil
	})
}

// Prepare makes the hidden spare window, once the file manager's page is up (it calls this when it is ready).
func (lw *ListerWindows) Prepare() {
	lw.mu.Lock()
	lw.ready = true
	lw.mu.Unlock()
	lw.makeSpare()
}

// Open shows a file in a Lister window: the spare one when there is one, else a new one. It answers the window's name.
func (lw *ListerWindows) Open(req ListerRequest) (string, error) {
	a := lw.app
	if a.wapp == nil {
		return "", errNoApp
	}
	if req.Path == "" {
		return "", errors.New("no file to view")
	}
	lw.mu.Lock()
	v := lw.spare
	lw.spare = nil
	lw.mu.Unlock()
	if v == nil {
		v = lw.newWindow()
	}
	w, h := lw.size()
	if cw, ch := v.w.Size(); cw != w || ch != h {
		v.w.SetSize(w, h)
	}
	v.w.SetTitle(listerTitle(req.Path))
	lw.mu.Lock()
	r := req
	v.req, v.path, v.opened, lw.last = &r, req.Path, time.Now(), v.name
	ready := v.ready
	lw.mu.Unlock()
	if ready { // (a window still loading gets the request when its page asks for it, see Ready)
		lw.send(v, "viewer-open", r)
	}
	// The page shows the window once the file is drawn (ShowMe); should it not manage, the window comes anyway.
	time.AfterFunc(1500*time.Millisecond, func() { lw.show(v) })
	time.AfterFunc(400*time.Millisecond, lw.makeSpare)
	return v.name, nil
}

// Ready is called by a Lister window's page once it is loaded: it answers the window's name and the file to show
// (null for the spare, which gets "viewer-open" later).
func (lw *ListerWindows) Ready(ctx context.Context) map[string]any {
	v := lw.of(ctx)
	if v == nil {
		return map[string]any{"name": "", "request": nil}
	}
	lw.mu.Lock()
	defer lw.mu.Unlock()
	v.ready = true
	var req any
	if v.req != nil {
		req = *v.req
	}
	return map[string]any{"name": v.name, "request": req}
}

// ShowMe shows the window the call came from (its page calls it once the file is drawn) and notes the Lister session
// of the file, which is closed with the window.
func (lw *ListerWindows) ShowMe(ctx context.Context, session int) {
	v := lw.of(ctx)
	if v == nil {
		return
	}
	lw.mu.Lock()
	v.session = session
	lw.mu.Unlock()
	lw.show(v)
}

// Moved tells Go that the window the call came from shows another file now (N or P): the title follows, and the
// panel the file list came from puts its cursor on it.
func (lw *ListerWindows) Moved(ctx context.Context, path string, session int) {
	v := lw.of(ctx)
	if v == nil {
		return
	}
	lw.mu.Lock()
	v.path, v.session = path, session
	var side, dir string
	if v.req != nil {
		side, dir = v.req.Side, v.req.Dir
	}
	lw.mu.Unlock()
	v.w.SetTitle(listerTitle(path))
	if side != "" {
		if mw := lw.app.mainWindow(); mw != nil {
			lw.dispatch(mw, "lister-cursor", map[string]any{"side": side, "dir": dir, "name": baseName(path)})
		}
	}
}

// CloseMe closes the window the call came from (Esc, Q, the File menu's Close).
func (lw *ListerWindows) CloseMe(ctx context.Context) {
	if v := lw.of(ctx); v != nil {
		v.w.Close()
	}
}

// CtlReply is a Lister window's answer to a control command (JSON).
func (lw *ListerWindows) CtlReply(id int, result string) {
	lw.mu.Lock()
	ch := lw.waiting[id]
	delete(lw.waiting, id)
	lw.mu.Unlock()
	if ch != nil {
		ch <- result
	}
}

// ---- the windows ----

// makeSpare makes the hidden spare window when there is none and the file manager is up.
func (lw *ListerWindows) makeSpare() {
	lw.mu.Lock()
	if lw.spare != nil || !lw.ready || lw.app.wapp == nil || lw.app.isQuitting() {
		lw.mu.Unlock()
		return
	}
	lw.mu.Unlock()
	v := lw.newWindow()
	lw.mu.Lock()
	if lw.spare == nil {
		lw.spare = v
		v = nil
	}
	lw.mu.Unlock()
	if v != nil { // (two callers raced: the second window is not needed)
		v.w.Close()
	}
}

// newWindow makes a hidden Lister window, which starts loading its page at once.
func (lw *ListerWindows) newWindow() *listerWin {
	a := lw.app
	lw.mu.Lock()
	lw.seq++
	name := fmt.Sprintf("%s%d", listerWindowPrefix, lw.seq)
	lw.mu.Unlock()
	w, h := lw.size()
	win := a.wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             name,
		Title:            "Lister",
		Width:            w,
		Height:           h,
		MinWidth:         listerWinMinW,
		MinHeight:        listerWinMinH,
		URL:              "/viewer.html",
		Hidden:           true,
		BackgroundColour: windowBackground,
		InitialPosition:  application.WindowCentered,
		// Like the About box: no menu of its own on Windows and Linux (macOS keeps the app's menu bar).
		Windows: application.WindowsWindow{Theme: windowsTheme(a.opts.Theme), DisableMenu: true},
		Mac:     application.MacWindow{Appearance: macAppearance(a.opts.Theme)},
		Linux:   application.LinuxWindow{Icon: appIcon},
	})
	v := &listerWin{w: win, name: name}
	// The hook runs while the window can still be read: its size is kept, and its file is let go.
	win.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) { lw.closing(v) })
	lw.mu.Lock()
	lw.wins[name] = v
	lw.mu.Unlock()
	return v
}

// closing forgets a window that is closing, keeps its size and closes the file it showed.
func (lw *ListerWindows) closing(v *listerWin) {
	lw.mu.Lock()
	shown, session := v.shown, v.session
	delete(lw.wins, v.name)
	if lw.spare == v {
		lw.spare = nil
	}
	if lw.last == v.name {
		lw.last = ""
	}
	lw.mu.Unlock()
	if shown && !v.w.IsMaximised() && !v.w.IsFullscreen() && !v.w.IsMinimised() && lw.app.settings != nil {
		w, h := v.w.Size()
		x, y := v.w.Position()
		if w >= listerWinMinW && h >= listerWinMinH {
			lw.app.settings.put("listerWindow", map[string]any{"w": w, "h": h, "x": x, "y": y})
		}
	}
	if session != 0 && listerInstance != nil {
		listerInstance.Close(session)
	}
	if shown && lw.app.isStandalone() && lw.shownCount() == 0 { // (-S=L: the last Lister window ends the app, F-852)
		go lw.app.endStandalone()
	}
}

// shownCount is the number of Lister windows on screen (the hidden spare is not counted).
func (lw *ListerWindows) shownCount() int {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	n := 0
	for _, v := range lw.wins {
		if v.shown {
			n++
		}
	}
	return n
}

// show shows a window that was asked to show a file (once).
func (lw *ListerWindows) show(v *listerWin) {
	lw.mu.Lock()
	if v.shown || v.req == nil || lw.wins[v.name] != v {
		lw.mu.Unlock()
		return
	}
	v.shown = true
	keep := v.req.KeepPlace
	var others []*application.WebviewWindow
	if keep {
		for _, o := range lw.wins {
			if o != v && o.shown {
				others = append(others, o.w)
			}
		}
	}
	lw.mu.Unlock()
	if keep { // (the windows are asked where they are outside the lock: that may wait for the UI thread)
		taken := make([][2]int, 0, len(others))
		for _, w := range others {
			x, y := w.Position()
			taken = append(taken, [2]int{x, y})
		}
		lw.placeKept(v, taken)
	}
	v.w.Show()
	v.w.Focus()
}

// size is the kept Lister window size, or the default one.
func (lw *ListerWindows) size() (int, int) {
	w, h := listerWinW, listerWinH
	if s := lw.app.settings; s != nil {
		if m, ok := s.Get("listerWindow").(map[string]any); ok {
			if x, ok := m["w"].(float64); ok && int(x) >= listerWinMinW {
				w = int(x)
			}
			if y, ok := m["h"].(float64); ok && int(y) >= listerWinMinH {
				h = int(y)
			}
		}
	}
	return w, h
}

// place is where the last Lister window was closed, and whether that is known (older settings kept the size only).
func (lw *ListerWindows) place() (int, int, bool) {
	if s := lw.app.settings; s != nil {
		if m, ok := s.Get("listerWindow").(map[string]any); ok {
			x, okx := m["x"].(float64)
			y, oky := m["y"].(float64)
			if okx && oky {
				return int(x), int(y), true
			}
		}
	}
	return 0, 0, false
}

// listerCascade moves a kept place down and to the right while another shown Lister window sits there already, so a
// second window does not hide the first one exactly.
func listerCascade(x, y int, taken [][2]int) (int, int) {
	const step = 24
	for range len(taken) + 1 {
		hit := false
		for _, t := range taken {
			if t[0] == x && t[1] == y {
				hit = true
				break
			}
		}
		if !hit {
			break
		}
		x, y = x+step, y+step
	}
	return x, y
}

// placeKept puts a window where the last Lister window was closed (Options > Lister > Open Lister windows where the
// last one was closed), when that place is still on a screen; otherwise the window stays centred.
func (lw *ListerWindows) placeKept(v *listerWin, taken [][2]int) {
	x, y, ok := lw.place()
	if !ok || lw.app.wapp == nil {
		return
	}
	x, y = listerCascade(x, y, taken)
	w, h := v.w.Size()
	total := 0
	for _, s := range lw.app.wapp.Screen.GetAll() {
		total = max(total, s.Bounds.X+s.Bounds.Width)
	}
	if total > 0 && onScreen(winState{X: x, Y: y, W: w, H: h}, total) {
		v.w.SetPosition(x, y)
	}
}

// of is the Lister window a call came from, or nil.
func (lw *ListerWindows) of(ctx context.Context) *listerWin {
	w := windowOf(ctx)
	if w == nil {
		return nil
	}
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.wins[w.Name()]
}

// send gives one Lister window's page an event (only that window gets it).
func (lw *ListerWindows) send(v *listerWin, event string, data ...any) {
	lw.dispatch(v.w, event, data...)
}

// dispatch sends an event to the page of one window. The values reach the page's listener as its arguments, like
// App.emit's (shell.go).
func (lw *ListerWindows) dispatch(w *application.WebviewWindow, event string, data ...any) {
	if data == nil {
		data = []any{}
	}
	w.DispatchWailsEvent(&application.CustomEvent{Name: event, Data: any(data)})
}

func listerTitle(path string) string { return "Lister – " + baseName(path) }

// baseName is the last part of a path, for "/" and "\" paths alike.
func baseName(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return filepath.Base(p)
}

func (a *App) isQuitting() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.quitting
}

// ---- control mode ----

// pick finds the Lister window a control command is about: the one named, or the one opened last that is still open.
func (lw *ListerWindows) pick(name string) (*listerWin, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	if name == "" {
		name = lw.last
		if name == "" { // the newest shown window
			var best *listerWin
			for _, v := range lw.wins {
				if v.shown && (best == nil || v.opened.After(best.opened)) {
					best = v
				}
			}
			if best != nil {
				return best, nil
			}
		}
	}
	if v := lw.wins[name]; v != nil && v.req != nil {
		return v, nil
	}
	if name == "" {
		return nil, errors.New("no Lister window is open")
	}
	return nil, fmt.Errorf("no Lister window %q is open", name)
}

// control gives a command to a Lister window's page and waits for its answer (see viewer-window.js).
func (lw *ListerWindows) control(cmd string, m map[string]any) (map[string]any, error) {
	v, err := lw.pick(str(m, "window"))
	if err != nil {
		return nil, err
	}
	lw.mu.Lock()
	ready := v.ready
	lw.ctlSeq++
	id := lw.ctlSeq
	ch := make(chan string, 1)
	lw.waiting[id] = ch
	lw.mu.Unlock()
	if !ready {
		lw.mu.Lock()
		delete(lw.waiting, id)
		lw.mu.Unlock()
		return nil, fmt.Errorf("the Lister window %s is still loading", v.name)
	}
	msg := map[string]any{}
	for k, x := range m {
		msg[k] = x
	}
	msg["cmd"] = cmd
	lw.send(v, "viewer-ctl", id, msg)
	timeout := time.After(time.Duration(num(m, "timeout", 15)) * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case s := <-ch:
			return lw.answer(v, s)
		case <-tick.C: // a key that closes the window (Esc, Q) leaves no page to answer
			lw.mu.Lock()
			gone := lw.wins[v.name] != v
			if gone {
				delete(lw.waiting, id)
			}
			lw.mu.Unlock()
			if gone {
				return map[string]any{"ok": true, "window": v.name, "closed": true}, nil
			}
		case <-timeout:
			lw.mu.Lock()
			delete(lw.waiting, id)
			lw.mu.Unlock()
			return nil, fmt.Errorf("the Lister window %s did not answer %q in time", v.name, cmd)
		}
	}
}

// answer reads a Lister window's answer to a control command and adds the window's name, title and whether it is
// shown.
func (lw *ListerWindows) answer(v *listerWin, s string) (map[string]any, error) {
	var res map[string]any
	if err := json.Unmarshal([]byte(s), &res); err != nil {
		return nil, fmt.Errorf("the Lister window answered something else: %s", s)
	}
	if e, ok := res["error"].(string); ok && e != "" {
		return nil, errors.New(e)
	}
	lw.mu.Lock()
	res["window"], res["title"], res["shown"] = v.name, listerTitle(v.path), v.shown
	lw.mu.Unlock()
	return res, nil
}

// windowList describes every window of the app for control mode's "windows": its name and kind ("main", "about",
// "lister", "dialog"), whether it is on the screen, where it is, its title (Lister windows) and the file it shows.
func (a *App) windowList() []map[string]any {
	out := []map[string]any{}
	if a.wapp == nil {
		return out
	}
	lw := listerWindows
	for _, w := range a.wapp.Window.GetAll() {
		ww, ok := w.(*application.WebviewWindow)
		if !ok {
			continue
		}
		name := ww.Name()
		e := map[string]any{"name": name, "kind": name, "visible": ww.IsVisible(), "focused": ww.IsFocused()}
		x, y := ww.Position()
		width, height := ww.Size()
		e["x"], e["y"], e["w"], e["h"] = x, y, width, height
		if name == mainWindowName {
			e["title"] = a.windowTitle()
		}
		if strings.HasPrefix(name, listerWindowPrefix) && lw != nil {
			e["kind"] = "lister"
			lw.mu.Lock()
			if v := lw.wins[name]; v != nil {
				e["spare"] = v == lw.spare
				e["ready"] = v.ready
				if v.req != nil {
					e["title"], e["path"], e["side"] = listerTitle(v.path), v.path, v.req.Side
				}
			}
			lw.mu.Unlock()
		}
		if isDialogWindow(name) && dialogWindows != nil {
			dialogWindows.dialogWindowInfo(name, e)
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]["name"]) < fmt.Sprint(out[j]["name"]) })
	return out
}

// windowByName is a window of the app by its name (control mode's screenshot of a Lister window), or nil.
func (a *App) windowByName(name string) *application.WebviewWindow {
	if a.wapp == nil {
		return nil
	}
	for _, w := range a.wapp.Window.GetAll() {
		if ww, ok := w.(*application.WebviewWindow); ok && ww.Name() == name {
			return ww
		}
	}
	return nil
}
