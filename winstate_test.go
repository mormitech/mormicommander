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
