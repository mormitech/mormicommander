package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTakeSpare(t *testing.T) {
	type w struct {
		name  string
		ready bool
	}
	isReady := func(x *w) bool { return x.ready }
	a, b, c := &w{"a", false}, &w{"b", true}, &w{"c", true}
	pool := []*w{a, b, c}
	got, rest, ok := takeSpare(pool, isReady)
	if !ok || got != b || len(rest) != 2 || rest[0] != a || rest[1] != c {
		t.Fatalf("ready spare first: got %v rest %v", got, rest)
	}
	if pool[1] != b { // the pool passed in is not changed under the caller
		t.Fatal("takeSpare changed its input")
	}
	got, rest, ok = takeSpare([]*w{a}, isReady)
	if !ok || got != a || len(rest) != 0 {
		t.Fatalf("no ready spare: the oldest one; got %v", got)
	}
	if _, _, ok = takeSpare([]*w{}, isReady); ok {
		t.Fatal("empty pool gave a spare")
	}
}

func TestSparesToMake(t *testing.T) {
	for _, c := range []struct{ have, making, want int }{{0, 0, 2}, {1, 0, 1}, {1, 1, 0}, {2, 0, 0}, {3, 0, 0}, {0, 3, 0}} {
		if got := sparesToMake(c.have, c.making, dialogPoolSize); got != c.want {
			t.Errorf("sparesToMake(%d, %d) = %d, want %d", c.have, c.making, got, c.want)
		}
	}
}

func TestClampDialogSize(t *testing.T) {
	area := dlgRect{0, 25, 1440, 875}
	if w, h := clampDialogSize(400, 200, area, 0, 28); w != 400 || h != 200 {
		t.Errorf("small dialog changed: %dx%d", w, h)
	}
	if w, h := clampDialogSize(3000, 2000, area, 0, 28); w != 1440*9/10 || h != 875*9/10-28 {
		t.Errorf("big dialog: %dx%d", w, h)
	}
	if w, h := clampDialogSize(10, 5, area, 0, 0); w != dialogFloorW || h != dialogFloorH {
		t.Errorf("tiny dialog: %dx%d", w, h)
	}
	if w, h := clampDialogSize(3000, 2000, dlgRect{}, 0, 0); w != 3000 || h != 2000 {
		t.Errorf("unknown screen: %dx%d", w, h)
	}
	if w, h := clampDialogSize(500, 500, dlgRect{0, 0, 50, 50}, 10, 10); w != dialogFloorW || h != dialogFloorH {
		t.Errorf("tiny screen keeps the floor: %dx%d", w, h)
	}
}

func TestDialogPosition(t *testing.T) {
	area := dlgRect{0, 0, 1920, 1080}
	owner := dlgRect{100, 100, 1000, 700}
	parent := dlgRect{300, 200, 400, 300} // a dialog window below the new one (nested dialogs)
	if x, y := dialogPosition(dialogPlace{}, false, owner, owner, 400, 200, area); x != 400 || y != 350 {
		t.Errorf("centred over the owner: %d,%d", x, y)
	}
	if x, y := dialogPosition(dialogPlace{}, false, owner, parent, 200, 100, area); x != 400 || y != 300 {
		t.Errorf("centred over the dialog below: %d,%d", x, y)
	}
	p := dialogPlace{DX: 50, DY: 60, Moved: true}
	if x, y := dialogPosition(p, true, owner, parent, 200, 100, area); x != 150 || y != 160 {
		t.Errorf("kept offset from the owner: %d,%d", x, y)
	}
	if x, y := dialogPosition(dialogPlace{W: 900, H: 700}, true, owner, owner, 400, 200, area); x != 400 || y != 350 {
		t.Errorf("a kept size only is still centred: %d,%d", x, y)
	}
	far := dialogPlace{DX: 5000, DY: -900, Moved: true}
	if x, y := dialogPosition(far, true, owner, owner, 400, 200, area); x != 1920-400 || y != 0 {
		t.Errorf("kept place off the screen is pulled back: %d,%d", x, y)
	}
	if x, y := dialogPosition(dialogPlace{}, false, owner, owner, 400, 200, dlgRect{}); x != 400 || y != 350 {
		t.Errorf("unknown screen: %d,%d", x, y)
	}
}

func TestPlaceMemory(t *testing.T) {
	var all map[string]any
	if _, ok := placeFrom(all, "copy"); ok {
		t.Fatal("nothing kept yet")
	}
	all = withPlace(all, "copy", true, -20, 35, false, 0, 0)
	all = withPlace(all, "options", false, 0, 0, true, 900, 640)
	all = withPlace(all, "options", true, 10, 12, false, 0, 0)
	// through JSON, as the settings file gives it back (numbers become float64)
	raw, _ := json.Marshal(all)
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	p, ok := placeFrom(back, "copy")
	if !ok || !p.Moved || p.DX != -20 || p.DY != 35 || p.W != 0 || usableKeptSize(p, ok) {
		t.Errorf("copy: %+v", p)
	}
	p, ok = placeFrom(back, "options")
	if !ok || !p.Moved || p.DX != 10 || p.DY != 12 || p.W != 900 || p.H != 640 || !usableKeptSize(p, ok) {
		t.Errorf("options keeps both its size and its place: %+v", p)
	}
	if _, ok := placeFrom(back, ""); ok {
		t.Error("a dialog without a kind has no kept place")
	}
	if p, ok := placeFrom(map[string]any{"x": map[string]any{"dx": 3.0}}, "x"); !ok || p.Moved {
		t.Errorf("half an offset is no place: %+v", p)
	}
}

func TestPlaceChanges(t *testing.T) {
	placed, size := [2]int{100, 100}, [2]int{400, 300}
	if m, r := placeChanges(placed, size, 101, 102, 400, 300, true); m || r {
		t.Error("a pixel or two is no move")
	}
	if m, r := placeChanges(placed, size, 150, 100, 400, 300, false); !m || r {
		t.Error("moved")
	}
	if m, r := placeChanges(placed, size, 100, 100, 600, 300, false); m || r {
		t.Error("a fixed-size dialog is never resized")
	}
	if m, r := placeChanges(placed, size, 100, 100, 600, 300, true); m || !r {
		t.Error("resized")
	}
}

func TestTopOfStacksPerOwner(t *testing.T) {
	req := &DialogOpenRequest{Kind: "k"}
	dw := &DialogWindows{wins: map[string]*dialogWin{
		"dialog-1": {name: "dialog-1", req: req, ownerName: "main", order: 1},
		"dialog-2": {name: "dialog-2", req: req, ownerName: "main", order: 3},
		"dialog-3": {name: "dialog-3", req: req, ownerName: "lister-1", order: 4},
		"dialog-4": {name: "dialog-4", ownerName: "main", order: 9}, // a spare (no dialog) never counts
	}}
	if v := dw.topOf("main"); v == nil || v.name != "dialog-2" {
		t.Fatalf("top over main: %v", v)
	}
	if v := dw.topOf("lister-1"); v == nil || v.name != "dialog-3" {
		t.Fatalf("top over lister-1: %v", v)
	}
	delete(dw.wins, "dialog-2") // the nested one closed: the one below is on top again
	if v := dw.topOf("main"); v == nil || v.name != "dialog-1" {
		t.Fatalf("after closing: %v", v)
	}
	if v := dw.topOf("about"); v != nil {
		t.Fatal("no dialog over about")
	}
}

func TestCSSColour(t *testing.T) {
	if c, ok := cssColour("rgb(34, 49, 72)"); !ok || c.Red != 34 || c.Green != 49 || c.Blue != 72 {
		t.Errorf("rgb: %+v %v", c, ok)
	}
	if c, ok := cssColour("rgba(1, 2, 3, 0.5)"); !ok || c.Red != 1 || c.Blue != 3 {
		t.Errorf("rgba: %+v %v", c, ok)
	}
	if _, ok := cssColour("transparent"); ok {
		t.Error("a name is not read")
	}
	if !isDialogWindow("dialog-3") || isDialogWindow("lister-1") {
		t.Error("isDialogWindow")
	}
}

// Issue #135: on macOS the frame learnt from a hidden spare's page came out as none, so an unresized window's outer
// height (title bar included) was taken for its inside, saved as a resize and grew by the title bar on every open.
func TestPickFrameKeepsUnresizedWindowUnchanged(t *testing.T) {
	if w, h := pickFrame(0, 32, true, [2]int{0, 0}); w != 0 || h != 32 {
		t.Fatalf("the window's own frame wins over the learnt one: %d x %d", w, h)
	}
	if w, h := pickFrame(0, 0, false, [2]int{2, 30}); w != 2 || h != 30 {
		t.Fatalf("the learnt frame when the window does not tell: %d x %d", w, h)
	}
	if w, h := pickFrame(0, 0, false, [2]int{-1, -1}); w != 0 || h != 0 {
		t.Fatalf("nothing known yet: %d x %d", w, h)
	}
	// Options opened at an inside of 920 x 560 and closed untouched: the frame is 920 x 592.
	placed, size := [2]int{100, 100}, [2]int{920, 560}
	frameW, frameH := pickFrame(0, 32, true, [2]int{0, 0})
	if _, r := placeChanges(placed, size, 100, 100, 920-frameW, 592-frameH, true); r {
		t.Fatal("an untouched window was taken as resized")
	}
}

// TestDialogWake checks the watchdog of a shown dialog window (issue #134: now and then a window was shown but its page
// never drew the dialog, and it stayed an empty box): it is woken at each time of dialogWakeAt while its page has not
// said the dialog is drawn, and it stops once the page has, once the window is closed, or after the last wake.
func TestDialogWake(t *testing.T) {
	ms := time.Millisecond
	// a window that never draws: simulate the watch loop and collect when it is woken
	var at []time.Duration
	since, woken := time.Duration(0), 0
	wait := dialogWakeAt[0]
	for steps := 0; wait > 0; steps++ {
		if steps > 100 {
			t.Fatal("the watchdog never stops")
		}
		since += wait
		var wake bool
		wake, wait = dialogWake(since, false, true, woken)
		if wake {
			woken++
			at = append(at, since)
		}
	}
	if len(at) != len(dialogWakeAt) {
		t.Fatalf("woken %d times, want %d (%v)", len(at), len(dialogWakeAt), at)
	}
	for i := range at {
		if at[i] != dialogWakeAt[i] {
			t.Fatalf("wake %d at %v, want %v", i+1, at[i], dialogWakeAt[i])
		}
	}
	if dialogWakeAt[0] > 600*ms {
		t.Fatalf("first wake after %v: an empty window should be woken within about half a second", dialogWakeAt[0])
	}
	// too early: no wake, wait for the rest of the time
	if wake, next := dialogWake(200*ms, false, true, 0); wake || next != dialogWakeAt[0]-200*ms {
		t.Fatalf("early: wake=%v next=%v", wake, next)
	}
	// a late check (the loop slept longer) wakes once and waits for the next time
	if wake, next := dialogWake(2*time.Second, false, true, 1); !wake || next != dialogWakeAt[2]-2*time.Second {
		t.Fatalf("late: wake=%v next=%v", wake, next)
	}
	// drawn, closed, or woken enough: stop
	for _, c := range []struct {
		painted, open bool
		woken         int
	}{{true, true, 0}, {false, false, 0}, {false, true, len(dialogWakeAt)}} {
		if wake, next := dialogWake(10*time.Second, c.painted, c.open, c.woken); wake || next != 0 {
			t.Fatalf("%+v: wake=%v next=%v, want stop", c, wake, next)
		}
	}
}

// A dialog window with a nested dialog window above it points at that one (issue #137), however deep the chain.
func TestTopAbove(t *testing.T) {
	links := []dlgLink{
		{"dialog-1", "main", 1},     // the Multi-rename tool over the main window
		{"dialog-2", "dialog-1", 2}, // "Save settings as" over it
		{"dialog-3", "dialog-2", 3}, // a confirmation over that one
		{"dialog-4", "lister-1", 4}, // a dialog over another window
	}
	cases := map[string]string{"dialog-1": "dialog-3", "dialog-2": "dialog-3", "dialog-3": "", "dialog-4": "", "main": "dialog-3", "lister-1": "dialog-4", "x": ""}
	for name, want := range cases {
		if got := topAbove(name, links); got != want {
			t.Errorf("topAbove(%q) = %q, want %q", name, got, want)
		}
	}
	if got := topAbove("dialog-1", links[:2]); got != "dialog-2" {
		t.Errorf("one nested dialog: got %q", got)
	}
	if got := topAbove("dialog-1", nil); got != "" {
		t.Errorf("no dialogs: got %q", got)
	}
}

// The close button asks the page first, like Esc (board #136: Compare by content lost unsaved edits); a page that does
// not answer cannot keep the window, and a window under another dialog window waits for that one.
func TestCloseChoice(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name          string
		byUser, isTop bool
		asked         time.Time
		want          closeAction
	}{
		{"closed by the page or the app", false, true, time.Time{}, closeNow},
		{"closed by the page, not top", false, false, now, closeNow},
		{"first click asks the page", true, true, time.Time{}, closeAskPage},
		{"second click while the page is answering", true, true, now.Add(-time.Second), closeWait},
		{"the page never answered", true, true, now.Add(-closeAskWait), closeNow},
		{"a dialog window above it", true, false, time.Time{}, closeFocusTop},
	}
	for _, c := range cases {
		if got := closeChoice(c.byUser, c.isTop, c.asked, now); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}
