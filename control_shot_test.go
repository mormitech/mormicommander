// Tests of control mode's screenshot helpers (control_shot.go): laying dialog windows' pictures over the window's
// picture at the right place (also at two pixels per point), cutting off what falls outside, the order the dialog
// windows are drawn in, and which dialog windows count as over a window and as drawn. No screen is used.
package main

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"
)

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

var (
	cBase = color.RGBA{10, 10, 10, 255}
	cRed  = color.RGBA{200, 0, 0, 255}
	cBlue = color.RGBA{0, 0, 200, 255}
)

func same(a, b color.Color) bool {
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

func TestOverlayShotsPlacesDialogs(t *testing.T) {
	base := solid(100, 80, cBase)
	frame := dlgRect{X: 200, Y: 100, W: 100, H: 80}
	out := overlayShots(base, frame, []shotOverlay{{solid(20, 10, cRed), dlgRect{X: 230, Y: 120, W: 20, H: 10}}})
	if !same(out.At(30, 20), cRed) || !same(out.At(49, 29), cRed) {
		t.Fatal("dialog not laid at its place")
	}
	if !same(out.At(29, 20), cBase) || !same(out.At(50, 29), cBase) || !same(out.At(30, 30), cBase) {
		t.Fatal("dialog laid wider than itself")
	}
	if !same(base.At(30, 20), cBase) {
		t.Fatal("the base picture was changed")
	}
}

func TestOverlayShotsRetinaScale(t *testing.T) {
	// The window is 100x80 points, its picture 200x160 pixels; the dialog's picture is in pixels too.
	base := solid(200, 160, cBase)
	frame := dlgRect{X: 0, Y: 0, W: 100, H: 80}
	out := overlayShots(base, frame, []shotOverlay{{solid(40, 20, cRed), dlgRect{X: 10, Y: 5, W: 20, H: 10}}})
	if !same(out.At(20, 10), cRed) || !same(out.At(59, 29), cRed) || !same(out.At(19, 10), cBase) || !same(out.At(60, 29), cBase) {
		t.Fatal("dialog not scaled to the picture's pixels per point")
	}
}

func TestOverlayShotsClipsAndOrders(t *testing.T) {
	base := solid(100, 80, cBase)
	frame := dlgRect{X: 0, Y: 0, W: 100, H: 80}
	out := overlayShots(base, frame, []shotOverlay{
		{solid(50, 50, cRed), dlgRect{X: 80, Y: 60, W: 50, H: 50}},  // hangs off the bottom right
		{solid(10, 10, cBlue), dlgRect{X: 85, Y: 65, W: 10, H: 10}}, // over the first one
		{solid(10, 10, cBlue), dlgRect{X: -5, Y: -5, W: 10, H: 10}}, // hangs off the top left
	})
	if out.Bounds().Dx() != 100 || out.Bounds().Dy() != 80 {
		t.Fatalf("picture size changed: %v", out.Bounds())
	}
	if !same(out.At(99, 79), cRed) || !same(out.At(90, 70), cBlue) || !same(out.At(0, 0), cBlue) || !same(out.At(6, 6), cBase) {
		t.Fatal("overlays not cut off or not drawn in order")
	}
}

func TestOverlayShotsNothingToLay(t *testing.T) {
	base := solid(4, 4, cBase)
	if out := overlayShots(base, dlgRect{W: 0}, []shotOverlay{{solid(2, 2, cRed), dlgRect{}}}); !same(out.At(0, 0), cBase) {
		t.Fatal("a frame without a width must leave the picture as it is")
	}
}

func TestDialogsOverAndDrawn(t *testing.T) {
	old := dialogWindows
	t.Cleanup(func() { dialogWindows = old })
	dialogWindows = nil
	if dialogsOver("main") != nil {
		t.Fatal("no dialog windows feature: nothing over the window")
	}
	req := &DialogOpenRequest{Kind: "x"}
	dialogWindows = &DialogWindows{wins: map[string]*dialogWin{
		"dialog-1": {name: "dialog-1", req: req, ownerName: "main", order: 2, shown: true, painted: true},
		"dialog-2": {name: "dialog-2", req: req, ownerName: "main", order: 1, shown: true, painted: false},
		"dialog-3": {name: "dialog-3", req: req, ownerName: "lister-1", order: 3, shown: true, painted: true},
		"dialog-4": {name: "dialog-4", ownerName: "main", order: 4}, // a spare: shows nothing
	}}
	ds := dialogsOver("main")
	if len(ds) != 2 || ds[0].name != "dialog-2" || ds[1].name != "dialog-1" {
		t.Fatalf("dialogs over main, oldest first: %+v", ds)
	}
	if allDrawn(ds) {
		t.Fatal("dialog-2 has not drawn its dialog yet")
	}
	dialogWindows.wins["dialog-2"].painted = true
	if !allDrawn(dialogsOver("main")) || !allDrawn(nil) {
		t.Fatal("all drawn")
	}
}

func TestShotPNGRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.png")
	if err := saveShotPNG(p, solid(3, 2, cRed)); err != nil {
		t.Fatal(err)
	}
	img, err := loadShotPNG(p)
	if err != nil || img.Bounds().Dx() != 3 || !same(img.At(1, 1), cRed) {
		t.Fatalf("round trip: %v", err)
	}
}

func TestScreenshotNeedsPath(t *testing.T) {
	_, c := startTestControl(t)
	if _, err := c.screenshot("", "", true); err == nil {
		t.Fatal("a screenshot without a path must fail")
	}
}
