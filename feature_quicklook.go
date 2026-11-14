package main

// Native Quick Look (board issue #19, F-863, macOS only): ⌘Y shows the file under the cursor in the system's Quick Look
// panel, the one Finder opens with Space, and while the panel is open it follows the cursor. The page
// (frontend/src/features/quicklook.js) owns the keys and sends the file under the cursor; this file checks the paths
// and passes them to the platform half (feature_quicklook_darwin.go; feature_quicklook_other.go elsewhere, where
// nothing is shown). Only files on this computer are shown: entries inside archives or on servers would need a
// download for every cursor step.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// QuickLook is what the page calls for Quick Look (window.go.main.QuickLook).
type QuickLook struct{ app *App }

func init() {
	registerFeature("quicklook", func(a *App) []exposed { return []exposed{expose(&QuickLook{app: a})} })
	// {"cmd": "quicklook"} tells whether Quick Look exists here and whether its panel is open (for QA)
	registerControl("quicklook", func(a *App, m map[string]any) (map[string]any, error) {
		q := &QuickLook{app: a}
		return map[string]any{"ok": true, "supported": q.Supported(), "open": q.IsOpen()}, nil
	})
}

var (
	errQuickLookMacOnly = errors.New("Quick Look is only on macOS.")
	errQuickLookLocal   = errors.New("Quick Look shows files on this computer only.")
	errQuickLookNothing = errors.New("There is no file to show.")
)

// quickLookPaths keeps the paths Quick Look can show: absolute paths of files or folders on this computer that are
// there. Paths of archives' entries or servers (scheme://…) make errQuickLookLocal when nothing else is left.
func quickLookPaths(paths []string) ([]string, error) {
	var out []string
	remote := false
	for _, p := range paths {
		if p == "" {
			continue
		}
		if strings.Contains(p, "://") {
			remote = true
			continue
		}
		if !filepath.IsAbs(p) {
			continue
		}
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		out = append(out, filepath.Clean(p))
	}
	if len(out) == 0 {
		if remote {
			return nil, errQuickLookLocal
		}
		return nil, errQuickLookNothing
	}
	return out, nil
}

// running says whether the real app is up (the platform calls need its main thread; tests have none).
func (q *QuickLook) running() bool { return q.app != nil && q.app.wapp != nil }

// Supported says whether this system has Quick Look (macOS).
func (q *QuickLook) Supported() bool { return quickLookSupported }

// Show opens the Quick Look panel on the files (the first one in front), or switches an open panel to them.
func (q *QuickLook) Show(paths []string) error {
	if !quickLookSupported {
		return errQuickLookMacOnly
	}
	ps, err := quickLookPaths(paths)
	if err != nil {
		return err
	}
	if !q.running() {
		return errNoApp
	}
	if !quickLookShow(q.app.mainWindow(), ps) {
		return errors.New("Quick Look could not open.")
	}
	return nil
}

// Update switches an open Quick Look panel to the files and answers whether the panel is open. A closed panel stays
// closed, and a path Quick Look cannot show (the ".." line, a server) leaves the panel on the file it shows.
func (q *QuickLook) Update(paths []string) bool {
	if !quickLookSupported || !q.running() || !quickLookOpen() {
		return false
	}
	if ps, err := quickLookPaths(paths); err == nil {
		quickLookShow(q.app.mainWindow(), ps)
	}
	return true
}

// IsOpen says whether the Quick Look panel is on the screen.
func (q *QuickLook) IsOpen() bool { return quickLookSupported && q.running() && quickLookOpen() }

// Close takes the Quick Look panel off the screen.
func (q *QuickLook) Close() {
	if quickLookSupported && q.running() {
		quickLookClose()
	}
}
