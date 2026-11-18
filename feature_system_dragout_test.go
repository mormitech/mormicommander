// Tests for the platform-free half of native file drag-out (board issue #108, feature_system_dragout.go): the checks
// on the dragged paths, the demo refusing, the names of finished drag operations, the modifier keys from macOS flags
// and the end-of-drag hand-off. The native drag itself needs a window and a mouse, so it is not started here.
package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDragOutPaths(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "sub")
	if err := os.WriteFile(a, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(b, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := dragOutPaths([]string{a, b, a, b + string(filepath.Separator)})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{a, b}; !reflect.DeepEqual(got, want) {
		t.Errorf("dragOutPaths = %v, want %v", got, want)
	}
	for name, in := range map[string][]string{
		"none":     nil,
		"empty":    {""},
		"relative": {"a.txt"},
		"missing":  {filepath.Join(dir, "gone")},
		"vfs":      {"sftp://host/x"},
	} {
		if _, err := dragOutPaths(in); err == nil {
			t.Errorf("%s: %v was accepted", name, in)
		}
	}
}

func TestSystemDragDemo(t *testing.T) {
	d := &SystemDrag{app: NewApp(startOpts{Demo: true})}
	if d.Supported() {
		t.Error("the demo offers native drags")
	}
	if ok, err := d.Start([]string{"/x"}); ok || err == nil {
		t.Errorf("the demo started a drag: %v, %v", ok, err)
	}
}

func TestSystemDragNoWindow(t *testing.T) {
	// Without a window (as in tests) no drag starts, and nothing is left waiting for its end.
	f := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	d := &SystemDrag{app: NewApp(startOpts{})}
	ok, err := d.Start([]string{f})
	if ok || err != nil {
		t.Errorf("Start without a window = %v, %v", ok, err)
	}
	if _, err := d.Start(nil); err == nil {
		t.Error("an empty drag was accepted")
	}
}

func TestDragOpName(t *testing.T) {
	for op, want := range map[int]string{
		0: "none", dragOpCopy: "copy", dragOpLink: "link", dragOpMove: "move", dragOpGeneric: "move",
		dragOpDelete: "move", dragOpCopy | dragOpLink: "copy", 64: "none",
	} {
		if got := dragOpName(op); got != want {
			t.Errorf("dragOpName(%d) = %q, want %q", op, got, want)
		}
	}
}

func TestModifiersFromMacFlags(t *testing.T) {
	if got := modifiersFromMacFlags(0); got != (DragModifiers{}) {
		t.Errorf("no flags = %+v", got)
	}
	got := modifiersFromMacFlags(1<<17 | 1<<20 | 1<<16) // Shift, Command and Caps Lock
	if want := (DragModifiers{ShiftKey: true, MetaKey: true}); got != want {
		t.Errorf("flags = %+v, want %+v", got, want)
	}
	got = modifiersFromMacFlags(1<<18 | 1<<19)
	if want := (DragModifiers{CtrlKey: true, AltKey: true}); got != want {
		t.Errorf("flags = %+v, want %+v", got, want)
	}
}

func TestDragEndedOnce(t *testing.T) {
	var got []int
	setDragEnded(func(op int) { got = append(got, op) })
	dragEnded(dragOpCopy)
	dragEnded(dragOpMove) // a second end with nobody waiting is dropped
	if !reflect.DeepEqual(got, []int{dragOpCopy}) {
		t.Errorf("ends seen = %v", got)
	}
}

func TestDragOpFromDropEffect(t *testing.T) {
	for e, want := range map[uint32]string{
		0: "none", dropEffectCopy: "copy", dropEffectMove: "move", dropEffectLink: "link",
		dropEffectCopy | dropEffectLink: "copy", 0x80000000: "none",
	} {
		if got := dragOpName(dragOpFromDropEffect(e)); got != want {
			t.Errorf("drop effect %#x = %q, want %q", e, got, want)
		}
	}
}

func TestCommonDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if got, ok := commonDir([]string{a, b}); !ok || got != dir {
		t.Errorf("commonDir(a, b) = %q, %v", got, ok)
	}
	if _, ok := commonDir([]string{a, filepath.Join(a, "c")}); ok {
		t.Error("files of two folders have a common folder")
	}
	if _, ok := commonDir(nil); ok {
		t.Error("no files have a common folder")
	}
}

func TestMouseLParam(t *testing.T) {
	if got := mouseLParam(10, 20); got != 20<<16|10 {
		t.Errorf("mouseLParam(10, 20) = %#x", got)
	}
	// a pointer left of or above the window gives negative 16-bit values, as Windows packs them
	if got := mouseLParam(-1, -2); got != 0xfffeffff {
		t.Errorf("mouseLParam(-1, -2) = %#x", got)
	}
}
