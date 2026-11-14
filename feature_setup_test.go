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
	oldGOOS, oldOpen, oldDefaults, oldHome, oldRead := setupGOOS, setupOpen, setupDefaults, setupHome, setupRead
	setupGOOS = goos
	setupOpen = func(url string) error { opened = append(opened, url); return nil }
	setupDefaults = func(args ...string) ([]byte, error) { return []byte(defaultsOut), defaultsErr }
	// the Full Disk Access probe never reads the real protected data in a test
	setupHome = func() (string, error) { return "/fake-home", nil }
	setupRead = func(string) bool { return false }
	t.Cleanup(func() {
		setupGOOS, setupOpen, setupDefaults, setupHome, setupRead = oldGOOS, oldOpen, oldDefaults, oldHome, oldRead
	})
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

// TestFullDiskAccessProbe: opening the pane first tries every protected path (so macOS lists the app even when some of
// them don't exist on this Mac), HasFullDiskAccess is true when any of them could be read, and nothing is tried
// outside macOS.
func TestFullDiskAccessProbe(t *testing.T) {
	opened := withSetupFakes(t, "darwin", "", nil)
	var tried []string
	readable := ""
	setupRead = func(p string) bool { tried = append(tried, p); return p == readable }
	s := &Setup{}
	if err := s.OpenFullDiskAccess(); err != nil {
		t.Fatal(err)
	}
	if len(tried) != len(fdaProbes) || tried[0] != filepath.Join("/fake-home", filepath.FromSlash(fdaProbes[0])) {
		t.Fatalf("tried %v, want every probe under the home folder", tried)
	}
	if len(*opened) != 1 || (*opened)[0] != fullDiskAccessURL {
		t.Fatalf("opened %v", *opened)
	}
	if s.HasFullDiskAccess() {
		t.Error("nothing readable, yet Full Disk Access is reported")
	}
	readable = filepath.Join("/fake-home", filepath.FromSlash(fdaProbes[len(fdaProbes)-1]))
	if !s.HasFullDiskAccess() {
		t.Error("a probe could be read, yet no Full Disk Access is reported")
	}

	withSetupFakes(t, "windows", "", nil)
	tried = nil
	setupRead = func(p string) bool { tried = append(tried, p); return true }
	if s.HasFullDiskAccess() || len(tried) != 0 {
		t.Errorf("outside macOS nothing is tried (tried %v)", tried)
	}
}

// TestReadProtected reads a file and a folder it may read, and fails on one that is not there.
func TestReadProtected(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !readProtected(f) || !readProtected(dir) || readProtected(filepath.Join(dir, "missing")) {
		t.Error("readProtected gives the wrong answer")
	}
}
