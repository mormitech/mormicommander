// The pure parts of the dialog windows (feature_dialog_window.go): which spare window a dialog takes, how big a window
// may get on a screen, where it goes, and how the place a kind of dialog was left in is kept in the settings. Nothing
// here touches a window, so dialogwin_logic_test.go checks it without starting the app.
package main

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

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
