package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestViewsTipFacts: the tooltip fields read from the file (Options > Tooltips > Tooltip fields per file type, F-821):
// a picture's size; nothing for a folder, a missing file or a group not asked for.
func TestViewsTipFacts(t *testing.T) {
	dir := t.TempDir()
	pic := filepath.Join(dir, "a.png")
	f, _ := os.Create(pic)
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 7, 3)))
	f.Close()
	v := &Views{}
	if r := v.TipFacts(pic, []string{"image"}); r["w"] != 7 || r["h"] != 3 {
		t.Errorf("picture: %v", r)
	}
	if r := v.TipFacts(pic, nil); r["w"] != 0 {
		t.Errorf("no group asked: %v", r)
	}
	for _, p := range []string{dir, filepath.Join(dir, "missing.png")} {
		if r := v.TipFacts(p, []string{"image", "audio"}); r["w"] != 0 || r["artist"] != "" {
			t.Errorf("%s: %v", p, r)
		}
	}
}

// TestIconsFromFiles: an internal association's icon file comes back as a data: URL; other files, large ones,
// relative paths and server paths are left out (F-323).
func TestIconsFromFiles(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "i.png")
	os.WriteFile(ok, []byte("\x89PNG"), 0o644)
	txt := filepath.Join(dir, "i.txt")
	os.WriteFile(txt, []byte("x"), 0o644)
	big := filepath.Join(dir, "big.png")
	os.WriteFile(big, make([]byte, iconFileMax+1), 0o644)
	r := (&Icons{}).FromFiles([]string{ok, txt, big, "i.png", "sftp://h/i.png", filepath.Join(dir, "none.png")})
	if len(r) != 1 || !strings.HasPrefix(r[ok], "data:image/png;base64,") {
		t.Errorf("got %v", r)
	}
}
