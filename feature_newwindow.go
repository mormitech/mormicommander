package main

// Commands > Open folder in new window (cm_OpenNewWindow, F-376): a second copy of mormiCommander starts with the
// current folder in its left panel. It is started with /N (a new window, never handed to this one) and with the same
// settings files (/I=, /F=) as this window. With Options > Operation > "Allow only one copy of the program" on, no
// second copy is started: the option says there must be one window only, so the command explains that instead.

import (
	"errors"
	"os"
	"strings"

	"mormicommander/internal/launch"
)

// ConfigNewWindow is what the page calls for the command (window.go.main.ConfigNewWindow).
type ConfigNewWindow struct {
	app *App
	// start starts the program; tests replace it to see the command instead.
	start func(launch.Command) error
	// onlyOnce reads "Allow only one copy of the program"; tests replace it.
	onlyOnce func() bool
	// exe finds this program's file; tests replace it.
	exe func() (string, error)
}

func init() {
	registerFeature("newwindow", func(a *App) []exposed {
		return []exposed{expose(newConfigNewWindow(a))}
	})
}

func newConfigNewWindow(a *App) *ConfigNewWindow {
	return &ConfigNewWindow{app: a, start: launch.New().Start, onlyOnce: configOnlyOnce, exe: func() (string, error) {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		real, _ := resolvedPath(exe) // (macOS finds the app bundle from the real path, see reexec.go)
		return real, nil
	}}
}

// configOnlyOnce reads Options > Operation > "Allow only one copy of the program".
func configOnlyOnce() bool {
	if configFeature == nil {
		return false
	}
	cfg, err := configFeature.get()
	if err != nil {
		return false
	}
	defer configFeature.mu.Unlock()
	return cfg.Settings.Operation.OnlyOnce
}

// errOnlyOneCopy is the answer when the option allows one window only.
var errOnlyOneCopy = errors.New("Only one copy of mormiCommander may run: the option \"Allow only one copy of the program\" is on " +
	"(Options > Operation). Turn it off to open folders in new windows.")

// newWindowArgs is the argument list of the new copy: /N, the folder for its left panel (active), and the settings
// files this window uses (/I=, /F=), so both windows share their settings.
func newWindowArgs(o startOpts, folder string) []string {
	args := []string{"/N", "/L=" + folder, "/P=L"}
	if o.IniPath != "" {
		args = append(args, "/I="+o.IniPath)
	}
	if o.FtpIniPath != "" {
		args = append(args, "/F="+o.FtpIniPath)
	}
	return args
}

// Open starts a new window with folder in its left panel.
func (n *ConfigNewWindow) Open(folder string) error {
	if n.app.mock != nil {
		return errors.New("This is the demo: it can't open more windows.")
	}
	if strings.Contains(folder, "://") {
		return errors.New("Only folders on this computer can be opened in a new window, not folders in archives or on servers.")
	}
	if err := systemLocalDir(folder); err != nil {
		return err
	}
	if n.onlyOnce() {
		return errOnlyOneCopy
	}
	exe, err := n.exe()
	if err != nil {
		return errors.New("The program file of mormiCommander could not be found.")
	}
	if err := n.start(launch.Command{Path: exe, Args: newWindowArgs(n.app.opts, folder), Dir: folder}); err != nil {
		return errors.New("A new window could not be started: " + err.Error())
	}
	return nil
}
