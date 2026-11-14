package main

import "testing"

func TestWinStateRoundTrip(t *testing.T) {
	old := demoDir
	demoDir = t.TempDir()
	defer func() { demoDir = old }()
	if _, ok := loadWinState(); ok {
		t.Fatal("state before any save")
	}
	saveWinState(winState{X: 10, Y: 20, W: 1000, H: 700, Max: true})
	st, ok := loadWinState()
	if !ok || st != (winState{X: 10, Y: 20, W: 1000, H: 700, Max: true}) {
		t.Fatalf("got %+v %v", st, ok)
	}
	saveWinState(winState{W: 10, H: 10}) // too small: ignored
	if st2, _ := loadWinState(); st2 != st {
		t.Errorf("tiny window saved: %+v", st2)
	}
}

func TestOnScreen(t *testing.T) {
	if !onScreen(winState{X: 100, Y: 50, W: 800, H: 600}, 1920) {
		t.Error("visible window rejected")
	}
	if onScreen(winState{X: 3000, Y: 50, W: 800, H: 600}, 1920) {
		t.Error("window on a gone screen accepted")
	}
	if onScreen(winState{X: 100, Y: -500, W: 800, H: 600}, 1920) {
		t.Error("window above the screen accepted")
	}
}

// A window that is not maximised is saved as it is; a maximised one keeps the place and size saved before it was
// maximised, and when there is none, its maximised bounds, so the next start still opens it maximised (F-018, #37).
func TestStateToSave(t *testing.T) {
	now := winState{X: 0, Y: 25, W: 1920, H: 1055}
	before := winState{X: 120, Y: 140, W: 900, H: 600}
	if got := stateToSave(winState{X: 5, Y: 6, W: 900, H: 700, Max: true}, false, before, true); got != (winState{X: 5, Y: 6, W: 900, H: 700}) {
		t.Errorf("normal window: %+v", got)
	}
	if got := stateToSave(now, true, before, true); got != (winState{X: 120, Y: 140, W: 900, H: 600, Max: true}) {
		t.Errorf("maximised window with an earlier save: %+v", got)
	}
	if got := stateToSave(now, true, winState{}, false); got != (winState{X: 0, Y: 25, W: 1920, H: 1055, Max: true}) {
		t.Errorf("maximised window, nothing saved before: %+v", got)
	}
}

// Saving a maximised window over an earlier save keeps the earlier size and marks it maximised; saving it normal
// again clears the mark.
func TestStateToSaveRoundTrip(t *testing.T) {
	old := demoDir
	demoDir = t.TempDir()
	defer func() { demoDir = old }()
	saveWinState(stateToSave(winState{X: 120, Y: 140, W: 900, H: 600}, false, winState{}, false))
	saved, ok := loadWinState()
	saveWinState(stateToSave(winState{X: 0, Y: 25, W: 1920, H: 1055}, true, saved, ok))
	if st, _ := loadWinState(); st != (winState{X: 120, Y: 140, W: 900, H: 600, Max: true}) {
		t.Fatalf("after maximising: %+v", st)
	}
	saved, ok = loadWinState()
	saveWinState(stateToSave(winState{X: 120, Y: 140, W: 900, H: 600}, false, saved, ok))
	if st, _ := loadWinState(); st.Max {
		t.Fatalf("still maximised after un-maximising: %+v", st)
	}
}
