package main

import (
	"os"
	"path/filepath"
	"strings"
)

// configDir is the folder of the app's settings (settings.json, window.json) in the user's config folder:
// ~/Library/Application Support/mormicommander, %AppData%\mormicommander, ~/.config/mormicommander, or the folder in
// MORMICOMMANDER_CONFIG_DIR (scripts/mc_ctl.py gives every app it starts a new empty one).
// A demo run (--demo / --mock) uses a new empty folder in the temporary folder instead, so the settings of the person who
// makes screenshots neither show in them nor change.
func configDir() string {
	if demoDir != "" {
		return demoDir
	}
	if d := os.Getenv("MORMICOMMANDER_CONFIG_DIR"); d != "" { // tests and the QA harness use a folder of their own
		return d
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "mormicommander")
}

// demoDir is the settings folder of a demo run, "" in every other run.
var demoDir string

// startDemo makes the settings folder of a demo run.
func startDemo() {
	if d, err := os.MkdirTemp("", "mormicommander-demo-"); err == nil {
		demoDir = d
	}
}

// underHome says whether the path is in the user's home folder, where the names of files and folders are personal
// (used to keep them out of logs and screenshots).
func underHome(path string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	for _, h := range []string{home, evalLinks(home)} {
		for _, p := range []string{path, evalLinks(path)} {
			if rel, err := filepath.Rel(h, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

func evalLinks(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
