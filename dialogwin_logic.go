// The pure parts of the dialog windows (feature_dialog_window.go): which spare window a dialog takes, how big a window
// may get on a screen, where it goes, what a click on its close button does, how the place a kind of dialog was
// left in is kept in the settings, and when the watchdog wakes a shown window whose page has not drawn the dialog.
// Nothing here touches a window, so dialogwin_logic_test.go checks it without starting the app.
package main

import (
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// dlgRect is a rectangle in screen pixels (a window's frame, a screen's work area); W == 0 means unknown.
type dlgRect struct{ X, Y, W, H int }

// The smallest inside a dialog window is given, whatever the dialog measured.
const dialogFloorW, dialogFloorH = 80, 40

// takeSpare picks the spare window a new dialog gets: the first whose page is loaded, else the oldest one. It answers
// the pick, the spares left, and false when there is none.
func takeSpare[T any](spares []T, ready func(T) bool) (T, []T, bool) {
	for i, s := range spares {
		if ready(s) {
			rest := append(append([]T(nil), spares[:i]...), spares[i+1:]...)
			return s, rest, true
		}
	}
	var zero T
	if len(spares) == 0 {
		return zero, spares, false
	}
	return spares[0], append([]T(nil), spares[1:]...), true
}

// sparesToMake is how many spare windows have to be made to fill a pool of size, with have spares and making on the way.
func sparesToMake(have, making, size int) int {
	return max(0, size-have-making)
}

// clampDialogSize keeps a window's inside (w x h, at least the floor) within nine tenths of the screen's work area,
// title bar and borders (frameW, frameH) included. An unknown area leaves the size as it is.
func clampDialogSize(w, h int, area dlgRect, frameW, frameH int) (int, int) {
	w, h = max(w, dialogFloorW), max(h, dialogFloorH)
	if area.W > 0 && area.H > 0 {
		w = max(dialogFloorW, min(w, area.W*9/10-frameW))
		h = max(dialogFloorH, min(h, area.H*9/10-frameH))
	}
	return w, h
}

// dialogPosition is where a dialog window's frame (fw x fh) goes: at the kept offset from the owner when the user moved
// that kind before, else in the middle of the window below it (parent); always with the whole frame on the screen when
// it fits, and the title bar on it when it does not.
func dialogPosition(p dialogPlace, kept bool, owner, parent dlgRect, fw, fh int, area dlgRect) (int, int) {
	var x, y int
	if kept && p.Moved {
		x, y = owner.X+p.DX, owner.Y+p.DY
	} else {
		x, y = parent.X+(parent.W-fw)/2, parent.Y+(parent.H-fh)/2
	}
	if area.W > 0 && area.H > 0 {
		x = max(area.X, min(x, area.X+area.W-fw))
		y = max(area.Y, min(y, area.Y+area.H-fh))
	}
	return x, y
}

// dialogPlace is where a kind of dialog was left: its offset from the window it was opened over (when the user moved
// it) and its inside size (when the user resized it).
type dialogPlace struct {
	DX, DY int
	Moved  bool
	W, H   int
}

// placeFrom reads the kept place of a kind from the setting "dialogWindows" ({kind: {dx, dy, w, h}}, numbers as JSON
// gives them).
func placeFrom(all map[string]any, kind string) (dialogPlace, bool) {
	var p dialogPlace
	if kind == "" {
		return p, false
	}
	m, ok := all[kind].(map[string]any)
	if !ok {
		return p, false
	}
	n := func(k string) (int, bool) {
		switch f := m[k].(type) {
		case float64:
			return int(f), true
		case int:
			return f, true
		}
		return 0, false
	}
	if dx, ok := n("dx"); ok {
		if dy, ok := n("dy"); ok {
			p.DX, p.DY, p.Moved = dx, dy, true
		}
	}
	p.W, _ = n("w")
	p.H, _ = n("h")
	return p, true
}

// usableKeptSize tells whether a kept size can be given to a resizable dialog window again.
func usableKeptSize(p dialogPlace, kept bool) bool {
	return kept && p.W >= dialogMinW && p.H >= dialogMinH
}

// placeChanges tells what the user changed of a dialog window between it being put at placed with inside size and it
// closing at x, y with inside w x h: moved (more than 2 pixels) and, for a resizable one, resized.
func placeChanges(placed, size [2]int, x, y, w, h int, resizable bool) (moved, resized bool) {
	moved = abs(x-placed[0]) > 2 || abs(y-placed[1]) > 2
	resized = resizable && (abs(w-size[0]) > 2 || abs(h-size[1]) > 2)
	return
}

// withPlace answers a copy of the setting "dialogWindows" with the kind's entry updated: the offset from the owner when
// moved, the inside size when resized; the other kinds and the entry's other values stay.
func withPlace(all map[string]any, kind string, moved bool, dx, dy int, resized bool, w, h int) map[string]any {
	out := map[string]any{}
	for k, x := range all {
		out[k] = x
	}
	e := map[string]any{}
	if old, ok := out[kind].(map[string]any); ok {
		for k, x := range old {
			e[k] = x
		}
	}
	if moved {
		e["dx"], e["dy"] = dx, dy
	}
	if resized {
		e["w"], e["h"] = w, h
	}
	out[kind] = e
	return out
}

// pickFrame is how much bigger a dialog window is than its inside: what the window itself tells when it can (macOS),
// else what was learnt from the first page (negative: not learnt yet, taken as none).
func pickFrame(nativeW, nativeH int, nativeOK bool, learnt [2]int) (int, int) {
	if nativeOK && nativeW >= 0 && nativeH >= 0 {
		return nativeW, nativeH
	}
	if learnt[0] < 0 {
		return 0, 0
	}
	return learnt[0], learnt[1]
}

// dlgLink is one open dialog window for topAbove: its name, the window it stays above, and when it was opened.
type dlgLink struct {
	Name, Parent string
	Order        int
}

// topAbove is the newest dialog window that stays above the window named, directly or over other dialog windows on top
// of it ("" for none). A dialog window with one above it must not be closed by the user: its close button hands the
// focus to that one instead, so a nested dialog is never left behind attached to nothing.
func topAbove(name string, links []dlgLink) string {
	above := map[string]bool{name: true}
	for grew := true; grew; { // (the chains are short; walk until no new window joins)
		grew = false
		for _, l := range links {
			if !above[l.Name] && above[l.Parent] {
				above[l.Name], grew = true, true
			}
		}
	}
	top, order := "", 0
	for _, l := range links {
		if l.Name != name && above[l.Name] && (top == "" || l.Order > order) {
			top, order = l.Name, l.Order
		}
	}
	return top
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// dialogWakeAt are the times after a dialog window was shown at which the watchdog wakes it (wakeWindow) while its page
// has still not said the dialog is on the screen and painted (Shown). A page normally says so within 100 ms.
var dialogWakeAt = []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond, 3500 * time.Millisecond, 7500 * time.Millisecond}

// dialogWake tells the watchdog of a dialog window what to do: since is how long ago the window was shown, painted
// whether its page has said the dialog is drawn, open whether the window still shows its dialog, woken how many times
// it was woken already. It answers whether to wake it now and how long to wait before asking again (0: stop watching).
func dialogWake(since time.Duration, painted, open bool, woken int) (wake bool, next time.Duration) {
	if painted || !open || woken >= len(dialogWakeAt) {
		return false, 0
	}
	if since >= dialogWakeAt[woken] {
		wake = true
		woken++
		if woken >= len(dialogWakeAt) {
			return true, 0
		}
	}
	return wake, max(dialogWakeAt[woken]-since, time.Millisecond)
}

// What happens when the user clicks a dialog window's close button (closeChoice).
type closeAction int

const (
	closeNow      closeAction = iota // let the window close; the dialog's page is told it was cancelled ("dlg-closed")
	closeAskPage                     // keep the window and ask the dialog's page to close it the way Esc does
	closeWait                        // keep the window: the page was asked a moment ago and has not answered yet
	closeFocusTop                    // keep the window: a dialog window above it has to be answered first
)

// closeAskWait is how long a page may take to answer a close request; a click on the close button after that closes the
// window anyway, so a page that does not answer can never keep a window on the screen.
const closeAskWait = 3 * time.Second

// closeChoice decides what a click on a dialog window's close button does. byUser is false when the page or the app
// closes it (an answer, the owner closing, quitting): that always closes. Else the window must be the top dialog window
// of its owner, and the page is asked first (so a dialog can ask "Save changes?" or refuse); asked is when the page was
// last asked and has not answered yet (zero when it was not).
func closeChoice(byUser, isTop bool, asked, now time.Time) closeAction {
	switch {
	case !byUser:
		return closeNow
	case !isTop:
		return closeFocusTop
	case asked.IsZero():
		return closeAskPage
	case now.Sub(asked) < closeAskWait:
		return closeWait
	default:
		return closeNow
	}
}

// dialogCollectionBehavior keeps a dialog window out of macOS full-screen Spaces: it is never made full screen itself
// (it still goes along with the window it is a child of), in place of Wails' default, FullScreenPrimary (issue #138).
const dialogCollectionBehavior = application.MacWindowCollectionBehaviorFullScreenNone

// dialogZoomState is the state of a dialog window's maximise box (the green button on macOS): only a dialog that can be
// resized may be maximised (zoomed); a fixed-size one has it greyed out (issue #138).
func dialogZoomState(resizable bool) application.ButtonState {
	if resizable {
		return application.ButtonEnabled
	}
	return application.ButtonDisabled
}
