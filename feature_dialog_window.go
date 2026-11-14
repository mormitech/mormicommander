// Dialogs in real windows of their own. Every dialog of the app is still built by the page that asks for it (the core's
// modal() in frontend/src/core/dialogs.js), so the features' code, their closures and control mode keep working on the
// real elements. That page keeps the dialog hidden and has a native window show a live copy of it: the window's page
// (frontend/dialog.html, started by frontend/src/dialog-window.js) draws the copy, gets every change of it as a patch,
// and sends back what the user does there (typing, clicks, keys, focus, scrolling), which the dialog's page replays on
// the real dialog (frontend/src/core/mirror.js). This file only manages the windows and carries the messages:
//
//   - a pool of two hidden windows is made in advance (Prepare), so a dialog only has to be filled and shown; when one
//     is taken, the next is made in the background;
//   - Open sizes the window to the dialog, puts it in the middle of the window that asked for it (or where a dialog of
//     that kind was last left), keeps it above that window (a child window on macOS, an owned window on Windows, a
//     transient window on Linux) and hands it the dialog; the window asks to be shown once it has drawn it (ShowMe);
//   - Push carries patches from the dialog's page to its window, Input the user's actions the other way ("dlg-patch"
//     and "dlg-input" events sent to that one window only); every message is numbered, so their order is kept;
//   - closing the window with its close button cancels the dialog ("dlg-closed" to the dialog's page); Close is the
//     page closing it after an answer; the window's place (and its size, when it was resized) is kept per dialog kind in
//     the setting "dialogWindows".
//
// While a dialog window is open, the window that asked for it stays dimmed and gives the keyboard to the dialog; when it
// gets the focus (a click on it), the top dialog window takes it back, the way system dialogs behave. Control mode
// lists the dialog windows with "windows" (kind "dialog", the dialog's kind and title, shown, bounds, timings).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	dialogWindowPrefix = "dialog-"
	dialogPoolSize     = 2
	dialogRefill       = 400 * time.Millisecond // how long after a window was taken the next spare is made
	// The smallest a resizable dialog window gets, and what a window starts with before it is sized.
	dialogMinW, dialogMinH = 260, 120
)

// DialogOpenRequest is what a page asks for when a dialog should show in a window.
type DialogOpenRequest struct {
	Kind      string          `json:"kind"`  // the dialog's kind (meta.kind), for the kept place
	Title     string          `json:"title"` // the window's title (the dialog's heading)
	W         int             `json:"w"`     // the dialog's size in CSS pixels (the window's inside)
	H         int             `json:"h"`
	Resizable bool            `json:"resizable"` // big tool windows can be resized
	Bg        string          `json:"bg"`        // the dialog's background ("rgb(r, g, b)"), so the window never flashes another colour
	Payload   json.RawMessage `json:"payload"`   // the dialog itself, for the window's page (opaque here)
}

// dialogWin is one dialog window as Go knows it.
type dialogWin struct {
	w         *application.WebviewWindow
	name      string
	ready     bool               // its page is loaded and has asked for work
	req       *DialogOpenRequest // the dialog it shows (nil while it is a hidden spare)
	owner     *application.WebviewWindow
	ownerName string                     // the owner's name (for topOf)
	parent    *application.WebviewWindow // the window it stays above (the owner, or the dialog below it)
	shown     bool
	byPage    bool // closed by the page (an answer), not by the user
	pending   []json.RawMessage
	title     string
	placed    [2]int        // where it was put (x, y), to tell whether the user moved it
	placedCh  chan struct{} // closed once it is sized and placed (it is not shown before)
	size      [2]int        // the inside size it was given
	opened    time.Time
	shownAt   time.Time
	order     int
	openMs    float64 // ms from the page's modal() to the window on the screen and painted (its page measures it)
}

// DialogWindows is what the pages call about dialog windows (see the top of this file).
type DialogWindows struct {
	app    *App
	mu     sync.Mutex
	wins   map[string]*dialogWin
	spares []*dialogWin
	making int
	seq    int
	order  int
	ready  bool   // a page asked for the pool, so spares may be made
	frame  [2]int // how much bigger a window is than its inside (title bar, borders), learnt from the first page
	hooked map[string]bool
}

var dialogWindows *DialogWindows

func init() {
	registerFeature("dialogwindows", func(a *App) []any {
		dialogWindows = &DialogWindows{app: a, wins: map[string]*dialogWin{}, hooked: map[string]bool{}, frame: [2]int{-1, -1}}
		return []any{dialogWindows}
	})
}

// Prepare makes the hidden spare windows (the pages call it once they are up and dialogs in windows are on).
func (dw *DialogWindows) Prepare() {
	dw.mu.Lock()
	dw.ready = true
	dw.mu.Unlock()
	dw.fill()
}

// Open shows a dialog in a window: a spare one when there is one, else a new one. It answers the window's name and the
// inside size the window got (a kept size for a resizable kind, or the one asked for, limited to the screen).
func (dw *DialogWindows) Open(ctx context.Context, req DialogOpenRequest) (map[string]any, error) {
	a := dw.app
	if a.wapp == nil {
		return nil, errNoApp
	}
	owner, _ := windowOf(ctx).(*application.WebviewWindow)
	if owner == nil {
		owner = a.mainWindow()
	}
	if owner == nil {
		return nil, errors.New("no window to show the dialog over")
	}
	dw.mu.Lock()
	dw.ready = true
	v, rest, _ := takeSpare(dw.spares, func(s *dialogWin) bool { return s.ready }) // a loaded spare, else any
	dw.spares = rest
	parent := owner
	if top := dw.topOf(owner.Name()); top != nil {
		parent = top.w
	}
	dw.mu.Unlock()
	if v == nil {
		v = dw.newWindow()
	}
	dw.hookOwner(owner)

	// The size: what the dialog measured, or the kept size of a resizable kind; never more than the screen.
	w, h := req.W, req.H
	keptPlace, kept := dw.kept(req.Kind)
	sized := false
	if req.Resizable && usableKeptSize(keptPlace, kept) {
		w, h, sized = keptPlace.W, keptPlace.H, true
	}
	frameW, frameH := dw.frameSize(v.w)
	area := screenArea(owner)
	w, h = clampDialogSize(w, h, area, frameW, frameH)
	title := req.Title
	if title == "" {
		title = "mormiCommander"
	}
	// The window's page gets the dialog first, so it draws it while the window is sized and placed here; it is shown
	// once both are done (show waits for placed).
	dw.mu.Lock()
	r := req
	dw.order++
	v.req, v.owner, v.ownerName, v.parent, v.title, v.opened, v.order = &r, owner, owner.Name(), parent, title, time.Now(), dw.order
	v.size, v.placedCh = [2]int{w, h}, make(chan struct{})
	ready := v.ready
	dw.mu.Unlock()
	dlgDebug("open %s ready=%v", v.name, ready)
	if ready { // (a window still loading gets the dialog when its page asks for it, see Ready)
		dispatchTo(v.w, "dlg-open", dw.openMessage(v))
	}

	// The place: where this kind was left (relative to the owner), else the middle of the window below it.
	x, y := dialogPosition(keptPlace, kept, frameOf(owner), frameOf(parent), w+frameW, h+frameH, area)
	v.w.SetTitle(title)
	if c, ok := cssColour(req.Bg); ok {
		v.w.SetBackgroundColour(c)
	}
	v.w.SetResizable(req.Resizable)
	if req.Resizable {
		v.w.SetMinSize(dialogMinW, dialogMinH)
	} else {
		v.w.SetMinSize(0, 0)
	}
	placeWindow(v.w, x, y, w, h, frameW, frameH)
	attachOwned(v.w, parent)
	dw.mu.Lock()
	v.placed = [2]int{x, y}
	dw.mu.Unlock()
	close(v.placedCh)
	dlgDebug("placed %s", v.name)

	// The page shows the window once the dialog is drawn (ShowMe); should it not manage, the window comes anyway.
	time.AfterFunc(2*time.Second, func() { dw.show(v) })
	// The next spare is made once this window is on the screen: making a window keeps the main thread busy for tens of
	// milliseconds, which would hold up showing this one.
	time.AfterFunc(dialogRefill, dw.fill)
	return map[string]any{"name": v.name, "w": w, "h": h, "sized": sized}, nil
}

// openMessage is what a window's page gets to draw: its name, the request and the time Go got it.
func (dw *DialogWindows) openMessage(v *dialogWin) map[string]any {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	return map[string]any{"name": v.name, "kind": v.req.Kind, "w": v.size[0], "h": v.size[1], "payload": v.req.Payload}
}

// Ready is called by a dialog window's page once it is loaded, with the size of its inside (from which Go learns how
// much the title bar and borders add). It answers the window's name and the dialog to draw (null for a spare).
func (dw *DialogWindows) Ready(ctx context.Context, innerW, innerH int) map[string]any {
	v := dw.of(ctx)
	if v == nil {
		return map[string]any{"name": "", "request": nil}
	}
	fw, fh := v.w.Size()
	dw.mu.Lock()
	if ew, eh := fw-innerW, fh-innerH; innerW > 0 && ew >= 0 && eh >= 0 && ew < 200 && eh < 200 && dw.frame[0] < 0 {
		dw.frame = [2]int{ew, eh}
	}
	v.ready = true
	has := v.req != nil
	dlgDebug("ready %s has=%v", v.name, has)
	pending := v.pending
	v.pending = nil
	dw.mu.Unlock()
	if !has {
		return map[string]any{"name": v.name, "request": nil}
	}
	msg := dw.openMessage(v)
	if len(pending) > 0 {
		go func() {
			for _, p := range pending {
				dispatchTo(v.w, "dlg-patch", p)
			}
		}()
	}
	return map[string]any{"name": v.name, "request": msg}
}

// ShowMe shows the window the call came from (its page calls it once the dialog is drawn).
func (dw *DialogWindows) ShowMe(ctx context.Context) {
	if v := dw.of(ctx); v != nil {
		dw.show(v)
	}
}

// Shown records how long the dialog took from the page's modal() to its window being on the screen and painted.
func (dw *DialogWindows) Shown(ctx context.Context, ms float64) {
	v := dw.of(ctx)
	if v == nil {
		return
	}
	dw.mu.Lock()
	v.openMs = ms
	dw.mu.Unlock()
}

// Push gives a dialog window's page a patch from the dialog's page (the changes of the dialog since the last one).
func (dw *DialogWindows) Push(name string, msg json.RawMessage) {
	dw.mu.Lock()
	v := dw.wins[name]
	if v == nil {
		dw.mu.Unlock()
		return
	}
	if !v.ready {
		v.pending = append(v.pending, append(json.RawMessage(nil), msg...))
		dw.mu.Unlock()
		return
	}
	dw.mu.Unlock()
	dispatchTo(v.w, "dlg-patch", msg)
}

// Input gives the page that owns the dialog what the user did in the dialog window the call came from.
func (dw *DialogWindows) Input(ctx context.Context, msg json.RawMessage) {
	v := dw.of(ctx)
	if v == nil {
		return
	}
	dw.mu.Lock()
	owner := v.owner
	dw.mu.Unlock()
	if owner != nil {
		dispatchTo(owner, "dlg-input", v.name, msg)
	}
}

// SetTitle changes a dialog window's title (the dialog's heading changed).
func (dw *DialogWindows) SetTitle(name, title string) {
	dw.mu.Lock()
	v := dw.wins[name]
	if v != nil {
		v.title = title
	}
	dw.mu.Unlock()
	if v != nil {
		v.w.SetTitle(title)
	}
}

// Resize changes the inside size of a dialog window whose dialog grew or shrank; its top left corner stays.
func (dw *DialogWindows) Resize(name string, w, h int) {
	dw.mu.Lock()
	v := dw.wins[name]
	if v == nil || v.req == nil {
		dw.mu.Unlock()
		return
	}
	owner := v.owner
	v.size = [2]int{w, h}
	dw.mu.Unlock()
	frameW, frameH := dw.frameSize(v.w)
	w, h = clampDialogSize(w, h, screenArea(owner), frameW, frameH)
	x, y := v.w.Position()
	placeWindow(v.w, x, y, w, h, frameW, frameH)
}

// FocusTop gives the focus to the top dialog window over the window the call came from (a click on its dimmed page).
func (dw *DialogWindows) FocusTop(ctx context.Context) {
	w := windowOf(ctx)
	if w == nil {
		return
	}
	dw.mu.Lock()
	v := dw.topOf(w.Name())
	dw.mu.Unlock()
	if v != nil {
		v.Focus()
	}
}

// Close closes a dialog window after its dialog got its answer.
func (dw *DialogWindows) Close(name string) {
	dw.mu.Lock()
	v := dw.wins[name]
	if v != nil {
		v.byPage = true
	}
	dw.mu.Unlock()
	if v != nil {
		v.w.Close()
	}
}

// ---- the windows ----

// fill makes spare windows until the pool is full (one at a time, in the background).
func (dw *DialogWindows) fill() {
	for {
		dw.mu.Lock()
		if !dw.ready || dw.app.wapp == nil || dw.app.isQuitting() || sparesToMake(len(dw.spares), dw.making, dialogPoolSize) == 0 {
			dw.mu.Unlock()
			return
		}
		dw.making++
		dw.mu.Unlock()
		v := dw.newWindow()
		dw.mu.Lock()
		dw.making--
		dw.spares = append(dw.spares, v)
		dw.mu.Unlock()
	}
}

// newWindow makes a hidden dialog window, which starts loading its page at once.
func (dw *DialogWindows) newWindow() *dialogWin {
	a := dw.app
	dw.mu.Lock()
	dw.seq++
	name := fmt.Sprintf("%s%d", dialogWindowPrefix, dw.seq)
	dw.mu.Unlock()
	win := a.wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             name,
		Title:            "mormiCommander",
		Width:            420,
		Height:           240,
		URL:              "/dialog.html",
		Hidden:           true,
		BackgroundColour: windowBackground,
		// No menu of its own on Windows and Linux (macOS keeps the app's menu bar); it cannot be minimised on its own.
		Windows:             application.WindowsWindow{Theme: windowsTheme(a.opts.Theme), DisableMenu: true},
		Mac:                 application.MacWindow{Appearance: macAppearance(a.opts.Theme)},
		Linux:               application.LinuxWindow{Icon: appIcon},
		MinimiseButtonState: application.ButtonDisabled,
	})
	v := &dialogWin{w: win, name: name}
	win.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) { dw.closing(v) })
	dw.mu.Lock()
	dw.wins[name] = v
	dw.mu.Unlock()
	return v
}

// closing forgets a dialog window that is closing: the dialog's page is told when the user closed it (= cancel), its
// place is kept, and the window below it gets the focus back.
func (dw *DialogWindows) closing(v *dialogWin) {
	dw.mu.Lock()
	delete(dw.wins, v.name)
	for i, s := range dw.spares {
		if s == v {
			dw.spares = append(dw.spares[:i], dw.spares[i+1:]...)
			break
		}
	}
	req, owner, byPage, shown := v.req, v.owner, v.byPage, v.shown
	placed, size := v.placed, v.size
	var next *dialogWin
	if owner != nil {
		next = dw.topOf(owner.Name())
	}
	dw.mu.Unlock()
	if req == nil {
		return
	}
	detachOwned(v.w, v.parent)
	if shown && !v.w.IsMinimised() {
		dw.keep(v, req, placed, size)
	}
	if !byPage && owner != nil {
		dispatchTo(owner, "dlg-closed", v.name)
	}
	go func() { // (after the window has gone, so the focus does not come back to it)
		time.Sleep(10 * time.Millisecond)
		if next != nil {
			next.w.Focus()
		} else if owner != nil && !dw.app.isQuitting() {
			owner.Focus()
		}
	}()
	time.AfterFunc(dialogRefill, dw.fill)
}

// show shows a window that was given a dialog (once) and gives it the focus.
func (dw *DialogWindows) show(v *dialogWin) {
	dw.mu.Lock()
	if v.shown || v.req == nil || dw.wins[v.name] != v {
		dw.mu.Unlock()
		return
	}
	v.shown = true
	v.shownAt = time.Now()
	parent, placed := v.parent, v.placedCh
	dw.mu.Unlock()
	if placed != nil {
		select {
		case <-placed:
		case <-time.After(time.Second):
		}
	}
	dlgDebug("show %s", v.name)
	v.w.Show()
	dlgDebug("shown %s visible=%v", v.name, v.w.IsVisible())
	if !showOwned(v.w, parent) { // (macOS: showing made it the key window already)
		v.w.Focus()
	}
}

// Focus gives the window the focus.
func (v *dialogWin) Focus() { v.w.Focus() }

// topOf is the newest shown-or-showing dialog window over the window named (nil for none). dw.mu is held.
func (dw *DialogWindows) topOf(owner string) *dialogWin {
	var top *dialogWin
	for _, v := range dw.wins {
		if v.req != nil && v.ownerName == owner && (top == nil || v.order > top.order) {
			top = v
		}
	}
	return top
}

// hookOwner makes a window that opens dialogs hand the focus on to its top dialog window when it gets it, and close its
// dialog windows when it closes itself (once per window).
func (dw *DialogWindows) hookOwner(owner *application.WebviewWindow) {
	name := owner.Name()
	dw.mu.Lock()
	if dw.hooked[name] {
		dw.mu.Unlock()
		return
	}
	dw.hooked[name] = true
	dw.mu.Unlock()
	owner.OnWindowEvent(events.Common.WindowFocus, func(*application.WindowEvent) {
		dw.mu.Lock()
		top := dw.topOf(name)
		dw.mu.Unlock()
		if top != nil && top.shown {
			go top.w.Focus()
		}
	})
	owner.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		dw.mu.Lock()
		var mine []*dialogWin
		for _, v := range dw.wins {
			if v.owner == owner {
				v.byPage = true
				mine = append(mine, v)
			}
		}
		delete(dw.hooked, name)
		dw.mu.Unlock()
		for _, v := range mine {
			v.w.Close()
		}
	})
}

// of is the dialog window a call came from, or nil.
func (dw *DialogWindows) of(ctx context.Context) *dialogWin {
	w := windowOf(ctx)
	if w == nil {
		return nil
	}
	dw.mu.Lock()
	defer dw.mu.Unlock()
	return dw.wins[w.Name()]
}

// frameSize is how much bigger a dialog window is than its inside: asked of the window where the system tells it
// (macOS), else learnt from the first page (0, 0 until a page told).
func (dw *DialogWindows) frameSize(win *application.WebviewWindow) (int, int) {
	nw, nh, ok := frameExtra(win)
	dw.mu.Lock()
	defer dw.mu.Unlock()
	return pickFrame(nw, nh, ok, dw.frame)
}

// ---- the kept places (dialogwin_logic.go) ----

func (dw *DialogWindows) kept(kind string) (dialogPlace, bool) {
	s := dw.app.settings
	if s == nil {
		return dialogPlace{}, false
	}
	all, _ := s.Get("dialogWindows").(map[string]any)
	return placeFrom(all, kind)
}

// keep notes where the user left a dialog window, and its size when it was resized.
func (dw *DialogWindows) keep(v *dialogWin, req *DialogOpenRequest, placed, size [2]int) {
	s := dw.app.settings
	if s == nil || req.Kind == "" || v.owner == nil {
		return
	}
	x, y := v.w.Position()
	fw, fh := v.w.Size()
	frameW, frameH := dw.frameSize(v.w)
	w, h := fw-frameW, fh-frameH
	moved, resized := placeChanges(placed, size, x, y, w, h, req.Resizable)
	if !moved && !resized {
		return
	}
	ox, oy := v.owner.Position()
	all, _ := s.Get("dialogWindows").(map[string]any)
	_ = s.put("dialogWindows", withPlace(all, req.Kind, moved, x-ox, y-oy, resized, w, h))
}

// frameOf is a window's frame on the screen.
func frameOf(w *application.WebviewWindow) dlgRect {
	if w == nil {
		return dlgRect{}
	}
	x, y := w.Position()
	width, height := w.Size()
	return dlgRect{x, y, width, height}
}

// screenArea is the work area of the screen a window is on (zero when unknown).
func screenArea(w *application.WebviewWindow) dlgRect {
	if w == nil {
		return dlgRect{}
	}
	s, err := w.GetScreen()
	if err != nil || s == nil {
		return dlgRect{}
	}
	a := s.WorkArea
	return dlgRect{a.X, a.Y, a.Width, a.Height}
}

// cssColour reads a colour as getComputedStyle gives it ("rgb(34, 49, 72)" or "rgba(…)").
func cssColour(s string) (application.RGBA, bool) {
	var r, g, b int
	if _, err := fmt.Sscanf(strings.TrimPrefix(strings.TrimPrefix(s, "rgba("), "rgb("), "%d, %d, %d", &r, &g, &b); err != nil {
		return application.RGBA{}, false
	}
	return application.NewRGB(uint8(r), uint8(g), uint8(b)), true
}

// dispatchTo sends an event to the page of one window; the values reach the page's listener as its arguments.
func dispatchTo(w *application.WebviewWindow, event string, data ...any) {
	if w == nil {
		return
	}
	if data == nil {
		data = []any{}
	}
	w.DispatchWailsEvent(&application.CustomEvent{Name: event, Data: any(data)})
}

// ---- control mode ----

// dialogWindowInfo adds what Go knows of a dialog window to its entry in control mode's "windows".
func (dw *DialogWindows) dialogWindowInfo(name string, e map[string]any) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	e["kind"] = "dialog"
	v := dw.wins[name]
	if v == nil {
		return
	}
	e["ready"] = v.ready
	e["spare"] = v.req == nil
	if v.req != nil {
		e["dialogKind"], e["title"], e["shown"] = v.req.Kind, v.title, v.shown
		e["innerW"], e["innerH"] = v.size[0], v.size[1]
		if v.owner != nil {
			e["owner"] = v.owner.Name()
		}
		if v.parent != nil {
			e["parent"] = v.parent.Name()
		}
		if v.openMs > 0 {
			e["openMs"] = v.openMs
		}
	}
}

// isDialogWindow tells whether a window name is a dialog window's.
func isDialogWindow(name string) bool { return strings.HasPrefix(name, dialogWindowPrefix) }

func dlgDebug(f string, a ...any) {
	if os.Getenv("MC_DLG_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "%s dlg: "+f+"\n", append([]any{time.Now().Format("15:04:05.000")}, a...)...)
	}
}
