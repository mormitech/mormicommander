package main

// First-run setup (board issue #6, F-NEW-1): the page shows one setup page the very first time the app starts on a
// computer (no settings file and no saved window yet), where the user picks a keyboard preset, checks whether the
// function keys need fn, and can open the macOS Full Disk Access pane. This file is the Go half: it tells the page
// whether this is a first run, how macOS treats the top row of keys, and opens the two System Settings panes. The
// page (frontend/src/features/setup.js) does the rest and can reopen the page from the Configuration menu at any time.
//
// Nothing here changes how an existing installation behaves: a computer that already has a settings file never sees
// the page unless the user asks for it, and control mode (the QA harness, which starts with an empty settings folder)
// never sees it either.

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// The System Settings panes the setup page opens (macOS only).
const (
	fullDiskAccessURL   = "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"
	keyboardSettingsURL = "x-apple.systempreferences:com.apple.Keyboard-Settings.extension"
)

// setupGOOS, setupOpen and setupDefaults are the operating system, the way a System Settings link is opened and the
// way the macOS "defaults" tool is asked; tests replace them so they never open anything or read the real preferences.
var (
	setupGOOS = runtime.GOOS
	setupOpen = func(url string) error {
		cmd := exec.Command("open", url)
		if err := cmd.Start(); err != nil {
			return err
		}
		go cmd.Wait()
		return nil
	}
	setupDefaults = func(args ...string) ([]byte, error) { return exec.Command("defaults", args...).Output() }
)

// Setup is what the page calls for the first-run setup page (window.go.main.Setup).
type Setup struct{ app *App }

// SetupInfo is what the setup page needs to know when the app starts.
type SetupInfo struct {
	FirstRun bool   `json:"firstRun"` // no settings file and no saved window when the app started
	Control  bool   `json:"control"`  // control mode is on (the page is then never shown by itself)
	Demo     bool   `json:"demo"`     // the --demo files are shown
	Platform string `json:"platform"` // "darwin", "windows" or "linux"
}

func init() {
	registerFeature("setup", func(a *App) []exposed { return []exposed{expose(&Setup{app: a})} })
}

// Info tells the page whether this is the first start of the app on this computer.
func (s *Setup) Info() SetupInfo {
	a := s.app
	info := SetupInfo{Platform: setupGOOS}
	if a == nil {
		return info
	}
	info.FirstRun = a.settings != nil && !a.settings.existed && a.saved == nil
	info.Control = a.opts.Control != ""
	info.Demo = a.mock != nil
	return info
}

// FnKeys says how macOS treats the top row of keys: "standard" (F1, F2… without fn), "media" (brightness, volume…
// unless fn is held) or "" when that can't be told (not a Mac).
func (s *Setup) FnKeys() string {
	if setupGOOS != "darwin" {
		return ""
	}
	out, err := setupDefaults("read", "-g", "com.apple.keyboard.fnState")
	return fnKeysFromDefaults(string(out), err)
}

// fnKeysFromDefaults reads the answer of `defaults read -g com.apple.keyboard.fnState`: 1 means the keys are standard
// function keys; 0, or no such preference at all (the macOS default), means they are media keys.
func fnKeysFromDefaults(out string, err error) string {
	if err != nil {
		return "media"
	}
	switch strings.TrimSpace(out) {
	case "1", "true", "YES":
		return "standard"
	}
	return "media"
}

// OpenFullDiskAccess opens System Settings at Privacy & Security > Full Disk Access.
func (s *Setup) OpenFullDiskAccess() error { return openSystemSettings(fullDiskAccessURL) }

// OpenKeyboardSettings opens System Settings at Keyboard (where the function keys are set).
func (s *Setup) OpenKeyboardSettings() error { return openSystemSettings(keyboardSettingsURL) }

func openSystemSettings(url string) error {
	if setupGOOS != "darwin" {
		return errors.New("System Settings is only on macOS.")
	}
	if err := setupOpen(url); err != nil {
		return errors.New("Can't open System Settings: " + err.Error())
	}
	return nil
}
