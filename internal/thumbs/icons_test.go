// Tests for the fallback file-type icons: classification by extension,
// labels, and that the SVG and raster renderings are well formed.

package thumbs

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestKindOf(t *testing.T) {
	for _, c := range []struct {
		name string
		want Kind
	}{
		{"a.JPG", KindImage}, {"x.heic", KindImage}, {"movie.mkv", KindVideo},
		{"song.flac", KindAudio}, {"pack.tar.gz", KindArchive}, {"a.7z", KindArchive},
		{"doc.pdf", KindPDF}, {"cv.docx", KindDocument}, {"t.xlsx", KindSpreadsheet},
		{"s.pptx", KindPresentation}, {"readme.md", KindText}, {"main.go", KindCode},
		{"setup.exe", KindExecutable}, {"font.ttf", KindFont}, {"disk.iso", KindDiskImage},
		{"Makefile", KindGeneric}, {"weird.zzz", KindGeneric}, {".bashrc", KindGeneric},
	} {
		if got := KindOf(c.name); got != c.want {
			t.Errorf("KindOf(%q) = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestIconFor(t *testing.T) {
	for _, c := range []struct {
		name  string
		dir   bool
		kind  Kind
		label string
	}{
		{"/x/photos", true, KindFolder, ""},
		{"..", true, KindParent, ""},
		{"/a/b/..", false, KindParent, ""},
		{"pic.jpeg", false, KindImage, "JPEG"},
		{"file.markdown", false, KindText, "MARK"},
		{"noext", false, KindGeneric, ""},
		{"x.<b>", false, KindGeneric, "<B>"},
	} {
		ic := IconFor(c.name, c.dir)
		if ic.Kind != c.kind || ic.Label != c.label || !strings.HasPrefix(ic.Color, "#") {
			t.Errorf("IconFor(%q,%v) = %+v", c.name, c.dir, ic)
		}
	}
}

func TestIconSVGWellFormed(t *testing.T) {
	for _, ic := range []Icon{IconFor("a", true), IconFor("..", true), IconFor("a.pdf", false),
		IconFor("x.<&>", false), IconFor("noext", false), IconFor("a.tiff", false)} {
		svg := ic.SVG(64)
		d := xml.NewDecoder(strings.NewReader(string(svg)))
		for {
			_, err := d.Token()
			if err != nil {
				if err.Error() != "EOF" {
					t.Errorf("%+v: invalid SVG: %v\n%s", ic, err, svg)
				}
				break
			}
		}
		if !strings.Contains(string(svg), `width="64"`) {
			t.Errorf("size missing: %s", svg)
		}
	}
	if !strings.Contains(string(IconFor("a.pdf", false).SVG(8)), `width="16"`) {
		t.Error("tiny SVG not clamped to 16")
	}
}

func TestIconImage(t *testing.T) {
	for _, ic := range []Icon{IconFor("d", true), IconFor("a.mp3", false), {Kind: KindGeneric, Color: "bogus"}} {
		img := ic.Image(48)
		if img.Bounds().Dx() != 48 || img.Bounds().Dy() != 48 {
			t.Errorf("size %v", img.Bounds())
		}
		var painted int
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] != 0 {
				painted++
			}
		}
		if painted == 0 {
			t.Errorf("%+v: empty image", ic)
		}
	}
	if IconFor("a", false).Image(0).Bounds().Dx() != 16 {
		t.Error("min size")
	}
}

func TestHasNativePreview(t *testing.T) {
	for name, want := range map[string]bool{"a.JPG": true, "b.webp": true, "c.tif": true, "d.heic": false, "e.txt": false, "f": false} {
		if got := HasNativePreview(name); got != want {
			t.Errorf("HasNativePreview(%q) = %v", name, got)
		}
	}
}
