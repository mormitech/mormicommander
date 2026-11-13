// These tests run the real macOS backend, osascript and all, against a private named pasteboard instead of the
// general one, so they exercise the actual NSPasteboard calls without ever touching what the user has copied. The
// pasteboard is released at the end of each test.

package clipfiles

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func testBoard(t *testing.T) *Clipboard {
	t.Helper()
	j := &jxaBackend{r: execRunner{}, board: fmt.Sprintf("org.mormicommander.test.%d.%d", os.Getpid(), time.Now().UnixNano())}
	t.Cleanup(func() { j.release() })
	return &Clipboard{b: j}
}

func TestDarwinFiles(t *testing.T) {
	c := testBoard(t)
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "a b.txt"), filepath.Join(dir, "ünï cödé"), filepath.Join(dir, "q\"uote'$(x)")}
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := c.ReadFiles(); !errors.Is(err, ErrEmpty) {
		t.Fatalf("fresh pasteboard: %v", err)
	}
	for _, op := range []Op{Copy, Cut} {
		if err := c.WriteFiles(paths, op); err != nil {
			t.Fatal(err)
		}
		got, gotOp, err := c.ReadFiles()
		if err != nil {
			t.Fatal(err)
		}
		// NSURL hands back the file system's own spelling of the name (NFD on APFS), so compare after resolving.
		if len(got) != len(paths) || gotOp != op {
			t.Fatalf("got %q %v, want %d paths %v", got, gotOp, len(paths), op)
		}
		for i := range paths {
			a, _ := os.Stat(paths[i])
			b, err := os.Stat(got[i])
			if err != nil || !os.SameFile(a, b) {
				t.Errorf("path %d: %q does not name %q", i, got[i], paths[i])
			}
		}
	}
	cont, err := c.Content()
	if err != nil || !cont.Files || !cont.Text {
		t.Fatalf("content %+v %v", cont, err)
	}
	if s, _ := c.ReadText(); s == "" {
		t.Error("file names were not added as text, as Finder does")
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.ReadFiles(); !errors.Is(err, ErrEmpty) {
		t.Fatalf("after clear: %v", err)
	}
}

func TestDarwinText(t *testing.T) {
	c := testBoard(t)
	if _, err := c.ReadText(); !errors.Is(err, ErrEmpty) {
		t.Fatalf("fresh pasteboard text: %v", err)
	}
	for _, s := range []string{"", "plain", "line1\nline2\r\n", "ünïcödé 😀 \"quotes\" `back` $(echo no)"} {
		if err := c.WriteText(s); err != nil {
			t.Fatal(err)
		}
		if got, err := c.ReadText(); err != nil || got != s {
			t.Errorf("text %q: got %q, %v", s, got, err)
		}
	}
	if _, _, err := c.ReadFiles(); !errors.Is(err, ErrEmpty) {
		t.Errorf("text read as files: %v", err)
	}
}

func TestDarwinImage(t *testing.T) {
	c := testBoard(t)
	if _, err := c.ReadImage(); !errors.Is(err, ErrEmpty) {
		t.Fatalf("fresh pasteboard image: %v", err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(2, 1, color.NRGBA{0, 128, 255, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteImage(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	got, err := c.ReadImage()
	if err != nil || !reflect.DeepEqual(got, buf.Bytes()) {
		t.Fatalf("image round trip: %d bytes, %v", len(got), err)
	}
	if cont, _ := c.Content(); !cont.Image || cont.Files {
		t.Fatalf("content %+v", cont)
	}
}
