package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The place, size and maximised state of the window are kept in window.json in the settings folder, so the next start
// comes up the way the window was left (F-018). A full-screen or minimised window is not remembered: that is not where
// the user put it. A maximised one keeps the size it had before, and opens maximised again.

type winState struct {
	X   int  `json:"x"`
	Y   int  `json:"y"`
	W   int  `json:"w"`
	H   int  `json:"h"`
	Max bool `json:"max,omitempty"`
}

const minWinW, minWinH = 480, 320

func winStatePath() string {
	if dir := configDir(); dir != "" {
		return filepath.Join(dir, "window.json")
	}
	return ""
}

// loadWinState is the window as it was left, and whether there is one.
func loadWinState() (winState, bool) {
	var st winState
	path := winStatePath()
	if path == "" {
		return st, false
	}
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &st) != nil || st.W < minWinW || st.H < minWinH {
		return winState{}, false
	}
	return st, true
}

func saveWinState(st winState) {
	path := winStatePath()
	if path == "" || st.W < minWinW || st.H < minWinH {
		return
	}
	b, err := json.Marshal(st)
	if err != nil || os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	writeAtomic(path, b)
}

// writeAtomic writes a file through a temporary one and a rename, so a crash never leaves half a file.
func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// onScreen is whether a window at x, y with this size can be reached on screens that are totalW wide (a screen that is
// gone since the last start must not leave the window out of reach).
func onScreen(st winState, totalW int) bool {
	return st.Y >= 0 && st.X+st.W > 100 && st.X < totalW-100
}

// stateToSave is what window.json gets for a window that is now at bounds (its place and size as the system reports
// them), maximised or not. A maximised window keeps the place and size it had before it was maximised: the ones saved
// earlier (saved, ok), so un-maximising after the next start gives the window the user had. When nothing was saved
// before (the window was maximised before it was ever saved), the maximised bounds are kept, so the next start still
// opens maximised instead of forgetting it.
func stateToSave(bounds winState, maxed bool, saved winState, ok bool) winState {
	if !maxed {
		bounds.Max = false
		return bounds
	}
	st := bounds
	if ok {
		st = saved
	}
	st.Max = true
	return st
}

// SaveWindow remembers the place and size of the window (also done when the window closes).
func (a *App) SaveWindow() {
	w := a.mainWindow()
	if w == nil {
		return
	}
	if w.IsFullscreen() || w.IsMinimised() {
		return
	}
	x, y := w.Position()
	width, height := w.Size()
	saved, ok := loadWinState()
	saveWinState(stateToSave(winState{X: x, Y: y, W: width, H: height}, w.IsMaximised(), saved, ok))
}

// restoreWindow puts the window where it was left, when that is still on a screen. The size is set again here, so the
// window has exactly the size that was saved whatever the system did with the one it was made with. A window left
// maximised is maximised again by maximiseSaved, once the window is shown.
func restoreWindow(wapp *application.App, w *application.WebviewWindow, st winState) {
	w.SetSize(st.W, st.H)
	total := 0
	for _, s := range wapp.Screen.GetAll() {
		total = max(total, s.Bounds.X+s.Bounds.Width)
	}
	if total == 0 || onScreen(st, total) {
		w.SetPosition(st.X, st.Y)
	}
}

// maximiseSaved maximises a window that was left maximised. It runs after the window is shown: on macOS maximising is
// the window's "zoom", which does not stick to a window that is not on screen yet. Maximise does nothing to a window
// that already is maximised, so calling it twice is harmless.
func maximiseSaved(w *application.WebviewWindow, st winState) {
	if st.Max && !w.IsMaximised() {
		w.Maximise()
	}
}
