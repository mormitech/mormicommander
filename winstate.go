package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
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

// SaveWindow remembers the place and size of the window (also done when the window closes).
func (a *App) SaveWindow() {
	if a.ctx == nil || a.mode != "" {
		return
	}
	if wruntime.WindowIsFullscreen(a.ctx) || wruntime.WindowIsMinimised(a.ctx) {
		return
	}
	if wruntime.WindowIsMaximised(a.ctx) {
		st, ok := loadWinState()
		if !ok {
			return
		}
		st.Max = true
		saveWinState(st)
		return
	}
	x, y := wruntime.WindowGetPosition(a.ctx)
	w, h := wruntime.WindowGetSize(a.ctx)
	saveWinState(winState{X: x, Y: y, W: w, H: h})
}

// restoreWindow puts the window where it was left, when that is still on a screen. The size is set again here: the
// size given to wails.Run is the inside of the window on macOS, while WindowGetSize (what was saved) is the whole
// window with its title bar, so without this the window grew by a title bar's height on every start.
func restoreWindow(ctx context.Context, st winState) {
	wruntime.WindowSetSize(ctx, st.W, st.H)
	total := 0
	if screens, err := wruntime.ScreenGetAll(ctx); err == nil {
		for _, s := range screens {
			total += s.Size.Width
		}
	}
	if total == 0 || onScreen(st, total) {
		wruntime.WindowSetPosition(ctx, st.X, st.Y)
	}
	if st.Max {
		wruntime.WindowMaximise(ctx)
	}
}
