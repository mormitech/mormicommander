// Control mode's "screenshot" (macOS), made so the picture is what is on the screen now. Two things used to make it
// show an older state of the window (issue #41): since dialogs show in native windows of their own (0.9.16), an open
// dialog is not part of the window that asked for it, so `screencapture -l` of that window left it out; and a dialog
// window is filled and shown a moment after the command that opened it answered, so a picture taken right away could
// come before it was on the screen. So before the picture is taken, the page is asked to finish what it is doing and
// paint (bounded, a stuck page never holds a screenshot up for long), and the dialog windows over the window are waited
// for until they have drawn their dialog (bounded too). Then each of those dialog windows is taken as well and laid
// over the window's picture where it stands on the screen, oldest first, so the PNG shows the window with its dialogs.
// `dialogs: false` gives the window alone, as before. The laying over (overlayShots) is plain Go and is tested.
package main

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

const (
	shotPageWait   = 3 * time.Second // the longest the page is waited for to be idle and painted
	shotDialogWait = 2 * time.Second // the longest the dialog windows are waited for to draw their dialog
)

// shotOverlay is one picture to lay over the window's picture: where its window's frame is on the screen (points).
type shotOverlay struct {
	img  image.Image
	rect dlgRect
}

// overlayShots lays the overlays over base, the picture of a window whose frame on the screen is frame (points). The
// picture's pixels per point come from base's width against the frame's, so a Retina picture (two pixels per point)
// gets its overlays at the right place; what falls outside the window is cut off. The overlays are drawn in the order
// given (the last one on top).
func overlayShots(base image.Image, frame dlgRect, overlays []shotOverlay) *image.RGBA {
	b := base.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), base, b.Min, draw.Src)
	if frame.W <= 0 || len(overlays) == 0 {
		return out
	}
	scale := float64(b.Dx()) / float64(frame.W)
	for _, o := range overlays {
		ob := o.img.Bounds()
		x := int(float64(o.rect.X-frame.X)*scale + 0.5)
		y := int(float64(o.rect.Y-frame.Y)*scale + 0.5)
		r := image.Rect(x, y, x+ob.Dx(), y+ob.Dy())
		draw.Draw(out, r, o.img, ob.Min, draw.Over)
	}
	return out
}

// shotDialog is what the screenshot needs to know of a dialog window over the window it takes.
type shotDialog struct {
	name    string
	order   int
	shown   bool
	painted bool
}

// dialogsOver are the dialog windows showing (or about to show) a dialog of the window named, oldest first.
func dialogsOver(owner string) []shotDialog {
	dw := dialogWindows
	if dw == nil {
		return nil
	}
	dw.mu.Lock()
	defer dw.mu.Unlock()
	var out []shotDialog
	for _, v := range dw.wins {
		if v.req != nil && v.ownerName == owner {
			out = append(out, shotDialog{v.name, v.order, v.shown, v.painted})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
	return out
}

// allDrawn tells whether every dialog window has been shown and has drawn its dialog.
func allDrawn(ds []shotDialog) bool {
	for _, d := range ds {
		if !d.shown || !d.painted {
			return false
		}
	}
	return true
}

// waitForPaint asks the page to finish its work and paint, giving up after shotPageWait, then waits (at most
// shotDialogWait) for the dialog windows over the window named to be on the screen with their dialog drawn.
func (c *control) waitForPaint(owner string) {
	if c.app.wapp == nil {
		return
	}
	done := make(chan struct{})
	go func() { c.toPage(map[string]any{"cmd": "idle"}); close(done) }()
	select {
	case <-done:
	case <-time.After(shotPageWait):
	}
	end := time.Now().Add(shotDialogWait)
	for time.Now().Before(end) && !allDrawn(dialogsOver(owner)) {
		time.Sleep(50 * time.Millisecond)
	}
}

// captureWindow takes one window (by its number on the screen) into a PNG file with `screencapture -l`.
func captureWindow(wid int, path string) error {
	out, err := exec.Command("screencapture", "-x", "-o", "-l", fmt.Sprint(wid), path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("screencapture: %v %s", err, out)
	}
	if st, err := os.Stat(path); err != nil || st.Size() == 0 {
		return errors.New("screencapture made no file (Screen Recording permission?)")
	}
	return nil
}

// loadShotPNG decodes a PNG file.
func loadShotPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// saveShotPNG writes an image as a PNG file (through a file next to it, so a reader never sees half a picture).
func saveShotPNG(path string, img image.Image) error {
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// addDialogs lays the dialog windows over the window named (whose picture is at path) onto that picture. It answers
// how many were laid over. A dialog window that can't be taken is left out rather than failing the screenshot.
func (c *control) addDialogs(path, owner string) (int, error) {
	ds := dialogsOver(owner)
	if len(ds) == 0 {
		return 0, nil
	}
	target := c.app.windowByName(owner)
	if target == nil {
		return 0, nil
	}
	dir, err := os.MkdirTemp("", "mc-shot-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	var overlays []shotOverlay
	for i, d := range ds {
		w := c.app.windowByName(d.name)
		wid := windowNumber(w)
		if wid == 0 {
			continue
		}
		p := filepath.Join(dir, fmt.Sprintf("d%d.png", i))
		if captureWindow(wid, p) != nil {
			continue
		}
		img, err := loadShotPNG(p)
		if err != nil {
			continue
		}
		overlays = append(overlays, shotOverlay{img, frameOf(w)})
	}
	if len(overlays) == 0 {
		return 0, nil
	}
	base, err := loadShotPNG(path)
	if err != nil {
		return 0, err
	}
	if err := saveShotPNG(path, overlayShots(base, frameOf(target), overlays)); err != nil {
		return 0, err
	}
	return len(overlays), nil
}
