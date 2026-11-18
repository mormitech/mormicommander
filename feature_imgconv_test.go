package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"mormicommander/internal/imgconv"
	"mormicommander/internal/thumbs"
)

func TestViewsImageSizeNewFormats(t *testing.T) {
	v := &Views{}
	for _, name := range []string{"s.heic", "s.avif", "s.jxl", "s_c.jxl"} {
		got := v.ImageSize(filepath.Join("internal", "imgconv", "testdata", name))
		if got["w"] != 40 || got["h"] != 24 {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestImgconvThumbLeavesOtherFilesAlone(t *testing.T) {
	p := writeTemp(t, "notes.txt", "hello")
	if _, err := imgconvThumb(context.Background(), p, 96, 96); !errors.Is(err, thumbs.ErrUnsupported) {
		t.Fatal(err)
	}
}

// TestListerDecoded converts the samples through the Lister when this machine has a converter (sips on macOS, or
// libavif/libjxl tools elsewhere).
func TestListerDecoded(t *testing.T) {
	l := newTestLister()
	for _, name := range []string{"s.avif", "s.jxl"} {
		p, _ := filepath.Abs(filepath.Join("internal", "imgconv", "testdata", name))
		info, err := l.Open(p, ListerOptions{Mode: "media"})
		if err != nil {
			t.Fatal(err)
		}
		d, err := l.Decoded(info.ID)
		l.Close(info.ID)
		if err != nil {
			t.Fatal(err)
		}
		if imgconv.Available(imgconv.SniffFile(p), imgconv.Options{}) == "" {
			if d.URL != "" || d.Error == "" {
				t.Errorf("%s without a converter: %+v", name, d)
			}
			continue
		}
		if d.Error != "" && d.URL == "" {
			t.Logf("%s: %s (%s)", name, d.Error, d.Tool) // (a converter without this codec)
			continue
		}
		if !strings.HasPrefix(d.URL, "data:image/") || d.Tool == "" {
			t.Errorf("%s: %.60s %s", name, d.URL, d.Tool)
		}
	}
	p := writeTemp(t, "a.txt", "plain text")
	info, err := l.Open(p, ListerOptions{Mode: "text"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(info.ID)
	if d, _ := l.Decoded(info.ID); d.URL != "" || d.Error == "" {
		t.Errorf("text file: %+v", d)
	}
}
