// The small helpers the features use instead of the Wails API: events to the page, the clipboard and the system's file
// dialogs. They keep every feature file free of Wails, so a change of the shell (as from Wails v2 to v3) happens here and
// not in thirty files. Each one is a no-op, or an error, when there is no application yet (in tests).
package main

import (
	"errors"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// fileFilter is one entry of a file dialog's type list: a name and patterns separated by ";" ("*.txt;*.csv").
type fileFilter = application.FileFilter

var errNoApp = errors.New("the app is not running")

// emit sends an event to the page. The arguments always reach it as a list, so the page's listener is called with them
// as its own arguments (frontend/src/core/go.js spreads them), the way Wails v2 did it.
func (a *App) emit(event string, data ...any) {
	if a.wapp == nil {
		return
	}
	if data == nil {
		data = []any{}
	}
	a.wapp.Event.Emit(event, any(data))
}

// emitExcept sends an event to the pages of every window but the one named (all of them when it is "").
func (a *App) emitExcept(window, event string, data ...any) {
	if a.wapp == nil {
		return
	}
	if window == "" {
		a.emit(event, data...)
		return
	}
	if data == nil {
		data = []any{}
	}
	for _, w := range a.wapp.Window.GetAll() {
		if ww, ok := w.(*application.WebviewWindow); ok && ww.Name() != window {
			ww.DispatchWailsEvent(&application.CustomEvent{Name: event, Data: any(data)})
		}
	}
}

// clipboardSetText puts text on the system's clipboard.
func (a *App) clipboardSetText(text string) error {
	if a.wapp == nil {
		return errNoApp
	}
	if !a.wapp.Clipboard.SetText(text) {
		return errors.New("the clipboard did not take the text")
	}
	return nil
}

// clipboardText is the text on the system's clipboard ("" when it holds none).
func (a *App) clipboardText() (string, error) {
	if a.wapp == nil {
		return "", errNoApp
	}
	text, _ := a.wapp.Clipboard.Text()
	return text, nil
}

// openFileDialog asks for one existing file; "" means cancelled.
func (a *App) openFileDialog(title, dir string, filters []fileFilter) (string, error) {
	if a.wapp == nil {
		return "", errNoApp
	}
	d := a.wapp.Dialog.OpenFile().SetTitle(title).CanChooseFiles(true).CanChooseDirectories(false)
	if dir != "" {
		d.SetDirectory(dir)
	}
	for _, f := range dialogFilters(filters) {
		d.AddFilter(f.DisplayName, f.Pattern)
	}
	if w := a.mainWindow(); w != nil {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// saveFileDialog asks for a file to write; "" means cancelled.
func (a *App) saveFileDialog(title, dir, name string, filters []fileFilter) (string, error) {
	if a.wapp == nil {
		return "", errNoApp
	}
	d := a.wapp.Dialog.SaveFile().CanCreateDirectories(true)
	d.SetMessage(title)
	if dir != "" {
		d.SetDirectory(dir)
	}
	if name != "" {
		d.SetFilename(name)
	}
	for _, f := range dialogFilters(filters) {
		d.AddFilter(f.DisplayName, f.Pattern)
	}
	if w := a.mainWindow(); w != nil {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// pickFolderDialog asks for a folder; "" means cancelled.
func (a *App) pickFolderDialog(title, start string) (string, error) {
	if a.wapp == nil {
		return "", errNoApp
	}
	d := a.wapp.Dialog.OpenFile().SetTitle(title).CanChooseFiles(false).CanChooseDirectories(true).CanCreateDirectories(true)
	if start != "" {
		d.SetDirectory(start)
	}
	if w := a.mainWindow(); w != nil {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// dialogFilters adapts a type list to the system: macOS shows every allowed type at once (it has no list to choose
// from), so a list with an "all files" entry must allow everything there, while "*" as an extension would allow
// nothing.
func dialogFilters(filters []fileFilter) []fileFilter {
	if runtime.GOOS != "darwin" {
		return filters
	}
	for _, f := range filters {
		if f.Pattern == "*" || f.Pattern == "*.*" {
			return nil
		}
	}
	return filters
}
