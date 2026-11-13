// QuickLook test. Running qlmanage is slow and depends on the machine's
// plug-ins, so it only runs when MORMI_TEST_QUICKLOOK=1 is set; it then
// thumbnails a PNG written into a temp dir.

package thumbs

import (
	"context"
	"os"
	"testing"
)

func TestQuickLook(t *testing.T) {
	if os.Getenv("MORMI_TEST_QUICKLOOK") != "1" {
		t.Skip("set MORMI_TEST_QUICKLOOK=1 to run qlmanage")
	}
	if systemThumbnailer() == nil {
		t.Skip("qlmanage not found")
	}
	p := writeFile(t, t.TempDir(), "q.png", encodeAs(t, "png", halves(300, 150)))
	img, err := quickLook(context.Background(), p, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() > 64 && b.Dy() > 64 {
		t.Errorf("preview too large: %v", b)
	}
}
