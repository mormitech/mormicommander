package main

// Tests of the first-run setup's Go half (feature_setup.go): when a start counts as the first run, how the macOS
// function key preference is read, and which System Settings links are opened. Nothing is really opened and the real
// preferences are never read: the opener and the defaults tool are replaced for the tests, and every settings file is
// in a temporary folder.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// withSetupFakes replaces the operating system, the opener and the defaults tool for one test.
func withSetupFakes(t *testing.T, goos string, defaultsOut string, defaultsErr error) *[]string {
	t.Helper()
	opened := []string{}
	oldGOOS, oldOpen, oldDefaults := setupGOOS, setupOpen, setupDefaults
	setupGOOS = goos
	setupOpen = func(url string) error { opened = append(opened, url); return nil }
	setupDefaults = func(args ...string) ([]byte, error) { return []byte(defaultsOut), defaultsErr }
	t.Cleanup(func() { setupGOOS, setupOpen, setupDefaults = oldGOOS, oldOpen, oldDefaults })
	return &opened
}

func TestSetupFirstRunOnlyWithoutSettingsFile(t *testing.T) {
	withSetupFakes(t, "darwin", "", nil)
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")

	// no settings file and no saved window: the first run
	a := &App{settings: loadSettings(p)}
	if !(&Setup{app: a}).Info().FirstRun {
		t.Fatal("a start without a settings file should be the first run")
	}

	// a settings file is there: not the first run, even when it is empty
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	a = &App{settings: loadSettings(p)}
	if (&Setup{app: a}).Info().FirstRun {
		t.Fatal("a start with a settings file should not be the first run")
	}

	// no settings file but a saved window (an older installation): not the first run either
	a = &App{settings: loadSettings(filepath.Join(dir, "other.json")), saved: &winState{}}
	if (&Setup{app: a}).Info().FirstRun {
		t.Fatal("a start with a saved window should not be the first run")
	}

	// control mode is reported, so the page can stay out of the QA harness's way
	a = &App{settings: loadSettings(filepath.Join(dir, "third.json")), opts: startOpts{Control: "0"}}
	if info := (&Setup{app: a}).Info(); !info.Control || info.Platform != "darwin" {
		t.Fatalf("info: %+v", info)
	}
}

func TestSetupFnKeys(t *testing.T) {
	cases := []struct {
		out  string
		err  error
		want string
	}{
		{"1\n", nil, "standard"},
		{"0\n", nil, "media"},
		{"", errors.New("The domain/default pair does not exist"), "media"},
	}
	for _, c := range cases {
		withSetupFakes(t, "darwin", c.out, c.err)
		if got := (&Setup{}).FnKeys(); got != c.want {
			t.Errorf("fnState %q / %v: got %q, want %q", c.out, c.err, got, c.want)
		}
	}
	withSetupFakes(t, "windows", "1", nil)
	if got := (&Setup{}).FnKeys(); got != "" {
		t.Errorf("not a Mac: got %q, want empty", got)
	}
}

func TestSetupOpensSystemSettings(t *testing.T) {
	opened := withSetupFakes(t, "darwin", "", nil)
	s := &Setup{}
	if err := s.OpenFullDiskAccess(); err != nil {
		t.Fatal(err)
	}
	if err := s.OpenKeyboardSettings(); err != nil {
		t.Fatal(err)
	}
	if len(*opened) != 2 || (*opened)[0] != fullDiskAccessURL || (*opened)[1] != keyboardSettingsURL {
		t.Fatalf("opened: %v", *opened)
	}

	opened = withSetupFakes(t, "linux", "", nil)
	if err := s.OpenFullDiskAccess(); err == nil {
		t.Fatal("System Settings should not open outside macOS")
	}
	if len(*opened) != 0 {
		t.Fatalf("nothing should be opened outside macOS: %v", *opened)
	}
}
