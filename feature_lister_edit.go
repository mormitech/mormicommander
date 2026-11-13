package main

// The editor (F4, Shift+F4; FEATURES.md section 28) and the external viewer (F3 when set up so, Alt+F3): starting
// other programs through internal/launch. The page passes the configured line (TC style: a program, optionally with
// parameters in which %1 is the file) and the panels for the other placeholders; an empty editor line means the
// system's text editor, an empty viewer line the file's default app.

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"mormicommander/internal/launch"
)

// ListerEdit is what the page calls to edit files and to view them outside the app (window.go.main.ListerEdit).
type ListerEdit struct {
	app    *App
	launch *launch.Launcher
}

func init() {
	registerFeature("lister-edit", func(a *App) []any { return []any{&ListerEdit{app: a, launch: launch.New()}} })
}

// ListerNewFile is the file Shift+F4 opened.
type ListerNewFile struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Dir     string `json:"dir"`
	Created bool   `json:"created"`
}

func (e *ListerEdit) check(path string) error {
	if e.app.mock != nil {
		return errors.New("This is the demo: its files are not real, so they can't be opened.")
	}
	if strings.Contains(path, "://") {
		return errors.New("Editing files inside archives or on servers is not ready yet.")
	}
	return nil
}

// Edit opens a file in the editor: the configured line, or the system's text editor when it is empty.
func (e *ListerEdit) Edit(path, tool string, ctx launch.ParamContext) error {
	if err := e.check(path); err != nil {
		return err
	}
	if err := e.launch.OpenInTool(launch.ParseTool(tool), path, ctx); err != nil {
		return listerLaunchError("the editor", filepath.Base(path), err)
	}
	return nil
}

// EditNew makes sure the file name (relative to dir, or a full path) exists, creating it empty when it does not, and
// opens it in the editor (Shift+F4).
func (e *ListerEdit) EditNew(dir, name, tool string, ctx launch.ParamContext) (ListerNewFile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ListerNewFile{}, errors.New("Type a file name.")
	}
	if err := e.check(dir); err != nil {
		return ListerNewFile{}, err
	}
	p, created, err := launch.CreateForEdit(dir, name)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return ListerNewFile{}, fmt.Errorf("Can't create %s: its folder is not there.", name)
		case errors.Is(err, fs.ErrPermission):
			return ListerNewFile{}, fmt.Errorf("You don't have permission to create %s.", name)
		}
		return ListerNewFile{}, fmt.Errorf("Can't create %s: %s", name, strings.TrimPrefix(err.Error(), "launch: "))
	}
	out := ListerNewFile{Path: p, Name: filepath.Base(p), Dir: filepath.Dir(p), Created: created}
	if err := e.launch.OpenInTool(launch.ParseTool(tool), p, ctx); err != nil {
		return out, listerLaunchError("the editor", out.Name, err)
	}
	return out, nil
}

// View opens a file in the external viewer line, or with its default app when the line is empty.
func (e *ListerEdit) View(path, tool string, ctx launch.ParamContext) error {
	if err := e.check(path); err != nil {
		return err
	}
	var err error
	if strings.TrimSpace(tool) == "" {
		err = e.launch.OpenDefault(path)
	} else {
		err = e.launch.OpenInTool(launch.ParseTool(tool), path, ctx)
	}
	if err != nil {
		return listerLaunchError("the viewer", filepath.Base(path), err)
	}
	return nil
}

func listerLaunchError(what, name string, err error) error {
	return fmt.Errorf("Can't open %s in %s: %s", name, what, strings.TrimPrefix(err.Error(), "launch: "))
}
