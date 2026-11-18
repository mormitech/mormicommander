package main

// Real file drag-out (board issue #108). A drag that starts in a panel is an ordinary HTML drag, which carries the
// files to other programs only as file URLs and paths in text. With the option "Drag files out as real files"
// (settings key "nativeDragOut", Options > Drag & drop, off by default) the page hands the drag to the system instead:
// SystemDrag.Start begins a native drag of the files from the main window, so Finder and other apps get real files.
// Finder and Explorer copy them (the drag never offers a move to other programs, so no file leaves its panel by
// surprise); inside the window the drop comes back as a file drop with the paths, and the page does with it what an
// HTML drop would (frontend/src/features/system-dnd.js), with the modifier keys read from SystemDrag.Modifiers.
//
// macOS (feature_system_dragout_darwin.go) and Windows (feature_system_dragout_windows.go) have the native drag; on
// Linux Supported answers false and the page keeps the HTML drag (file URLs as text/uri-list, which file managers
// there take as files).

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// SystemDrag is what the page calls for native file drags (window.go.main.SystemDrag).
type SystemDrag struct {
	app *App
}

func init() {
	registerFeature("system-dragout", func(a *App) []exposed {
		return []exposed{expose(&SystemDrag{app: a})}
	})
}

// DragModifiers are the modifier keys held right now, in the names of a DOM event (the page picks copy / move / link
// from them as for an HTML drop).
type DragModifiers struct {
	ShiftKey bool `json:"shiftKey"`
	CtrlKey  bool `json:"ctrlKey"`
	AltKey   bool `json:"altKey"`
	MetaKey  bool `json:"metaKey"`
}

// Supported tells whether this system can start a native file drag (and this is not the demo, whose files are not
// real).
func (d *SystemDrag) Supported() bool {
	return nativeDragSupported && d.app.mock == nil
}

// Start begins a native drag of the files from the main window. It answers false when no drag could start (the mouse
// button was already let go, no window, or the system has no native drag); the page then simply ends its drag. When
// the drag ends, the page gets the event "native-drag-end" with what the drop did ("copy", "move", "link", "none").
func (d *SystemDrag) Start(paths []string) (bool, error) {
	if d.app.mock != nil {
		return false, errors.New("This is the demo: its files are not real, so they can't be dragged to other programs.")
	}
	clean, err := dragOutPaths(paths)
	if err != nil {
		return false, err
	}
	if !nativeDragSupported {
		return false, nil
	}
	w := d.app.mainWindow()
	if w == nil {
		return false, nil
	}
	setDragEnded(func(op int) { d.app.emit("native-drag-end", dragOpName(op)) })
	ok := nativeFileDrag(w, clean)
	if !ok {
		setDragEnded(nil)
	}
	return ok, nil
}

// Modifiers are the modifier keys held now (all false where the system can't tell).
func (d *SystemDrag) Modifiers() DragModifiers { return heldModifiers() }

// modifiersFromMacFlags turns macOS modifier flags (NSEventModifierFlags) into the DOM names.
func modifiersFromMacFlags(f uint64) DragModifiers {
	return DragModifiers{
		ShiftKey: f&(1<<17) != 0,
		CtrlKey:  f&(1<<18) != 0,
		AltKey:   f&(1<<19) != 0,
		MetaKey:  f&(1<<20) != 0,
	}
}

// dragOutPaths checks the files of a drag: there is at least one, each is an absolute path to something that exists
// (the drag is only offered in local folders), and none is given twice.
func dragOutPaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, errors.New("No files to drag.")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" || !filepath.IsAbs(p) {
			return nil, errors.New("Only files in a local folder can be dragged to other programs.")
		}
		p = filepath.Clean(p)
		if seen[p] {
			continue
		}
		if _, err := os.Lstat(p); err != nil {
			return nil, errors.New("\"" + filepath.Base(p) + "\" is not there any more.")
		}
		seen[p] = true
		out = append(out, p)
	}
	return out, nil
}

// The operation bits of a finished drag, as macOS reports them (NSDragOperation).
const (
	dragOpCopy    = 1
	dragOpLink    = 2
	dragOpGeneric = 4
	dragOpMove    = 16
	dragOpDelete  = 32
)

// The effects of a finished drag, as Windows reports them (DROPEFFECT).
const (
	dropEffectCopy = 1
	dropEffectMove = 2
	dropEffectLink = 4
)

// dragOpFromDropEffect turns the DROPEFFECT a Windows drop reports into the operation bits dragEnded expects (the
// macOS NSDragOperation values that dragOpName reads).
func dragOpFromDropEffect(e uint32) int {
	switch {
	case e&dropEffectMove != 0:
		return dragOpMove
	case e&dropEffectCopy != 0:
		return dragOpCopy
	case e&dropEffectLink != 0:
		return dragOpLink
	}
	return 0
}

// commonDir is the folder all the paths are in, and whether there is one (the Windows drag gives the files relative
// to it, as Explorer does).
func commonDir(paths []string) (string, bool) {
	if len(paths) == 0 {
		return "", false
	}
	dir := filepath.Dir(paths[0])
	for _, p := range paths[1:] {
		if filepath.Dir(p) != dir {
			return "", false
		}
	}
	return dir, true
}

// dragOpName names what a finished drag did. The drop target picks one operation; "generic" is the target's own
// choice, which Finder uses for a move.
func dragOpName(op int) string {
	switch {
	case op == 0:
		return "none"
	case op&dragOpMove != 0, op&dragOpGeneric != 0, op&dragOpDelete != 0:
		return "move"
	case op&dragOpCopy != 0:
		return "copy"
	case op&dragOpLink != 0:
		return "link"
	}
	return "none"
}

// The function told when the native drag that is under way ends (only one drag runs at a time).
var (
	dragEndMu sync.Mutex
	dragEndFn func(op int)
)

func setDragEnded(fn func(op int)) {
	dragEndMu.Lock()
	dragEndFn = fn
	dragEndMu.Unlock()
}

// dragEnded is called by the platform code when the native drag ends, with the operation the drop did.
func dragEnded(op int) {
	dragEndMu.Lock()
	fn := dragEndFn
	dragEndFn = nil
	dragEndMu.Unlock()
	if fn != nil {
		fn(op)
	}
}
