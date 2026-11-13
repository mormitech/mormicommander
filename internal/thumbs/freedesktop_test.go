// Tests for reading the freedesktop.org thumbnail cache, using a fake cache
// root inside a temp dir: valid entries are found, stale ones ignored, and
// the size folder preference is respected.

package thumbs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeFDThumb(t *testing.T, root, folder, src string, mtime int64, w int) {
	t.Helper()
	data := encodeAs(t, "png", halves(w, w/2))
	if mtime != 0 {
		data = withPNGChunk(data, "tEXt", []byte("Thumb::MTime\x00"+strconv.FormatInt(mtime, 10)))
	}
	p := FreedesktopThumbPath(root, folder, src)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFreedesktopThumbPath(t *testing.T) {
	// Reference value from the spec's example: MD5 of
	// "file:///home/jens/photos/me.png".
	got := FreedesktopThumbPath("/c", "normal", "/home/jens/photos/me.png")
	if want := filepath.Join("/c", "normal", "c6ee772d9e49320e97ec29a7eb5b1697.png"); got != want {
		t.Errorf("path %s, want %s", got, want)
	}
	if p := FreedesktopThumbPath("/c", "large", "/a b/c.png"); !strings.HasSuffix(p, ".png") {
		t.Errorf("path %s", p)
	}
}

func TestFreedesktopLookup(t *testing.T) {
	root := t.TempDir()
	src := writeFile(t, t.TempDir(), "video.mp4", []byte("x"))
	fi, _ := os.Stat(src)
	mt := fi.ModTime().Unix()
	ctx := context.Background()

	if _, err := FreedesktopLookup(ctx, root, src, 128); !errors.Is(err, ErrUnsupported) {
		t.Errorf("empty cache: %v", err)
	}
	if _, err := FreedesktopLookup(ctx, "", src, 128); !errors.Is(err, ErrUnsupported) {
		t.Errorf("no root: %v", err)
	}
	writeFDThumb(t, root, "normal", src, mt-10, 128) // stale
	if _, err := FreedesktopLookup(ctx, root, src, 128); !errors.Is(err, ErrUnsupported) {
		t.Errorf("stale accepted: %v", err)
	}
	writeFDThumb(t, root, "large", src, mt, 256)
	img, err := FreedesktopLookup(ctx, root, src, 100)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 256 {
		t.Errorf("got width %d, want large (256)", img.Bounds().Dx())
	}
	writeFDThumb(t, root, "normal", src, mt, 128)
	if img, _ := FreedesktopLookup(ctx, root, src, 100); img.Bounds().Dx() != 128 {
		t.Errorf("want normal for 100px, got %d", img.Bounds().Dx())
	}
	if img, _ := FreedesktopLookup(ctx, root, src, 2000); img.Bounds().Dx() != 256 {
		t.Errorf("want largest available for 2000px, got %d", img.Bounds().Dx())
	}
	// Entries without an mtime chunk are accepted (some thumbnailers omit it).
	src2 := writeFile(t, t.TempDir(), "doc.pdf", []byte("y"))
	writeFDThumb(t, root, "normal", src2, 0, 128)
	if _, err := FreedesktopLookup(ctx, root, src2, 128); err != nil {
		t.Errorf("no-mtime entry: %v", err)
	}
	if _, err := FreedesktopLookup(ctx, root, filepath.Join(root, "missing"), 128); err == nil {
		t.Error("missing source: no error")
	}
}

func TestPNGTextChunks(t *testing.T) {
	data := withPNGChunk(encodeAs(t, "png", halves(4, 4)), "tEXt", []byte("Thumb::URI\x00file:///x"))
	data = withPNGChunk(data, "tEXt", []byte("Software\x00test"))
	m := pngTextChunks(data)
	if m["Thumb::URI"] != "file:///x" || m["Software"] != "test" {
		t.Errorf("chunks %v", m)
	}
	if len(pngTextChunks([]byte("garbage"))) != 0 {
		t.Error("garbage parsed")
	}
}
