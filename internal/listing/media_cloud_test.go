package listing

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// Media fields leave an online-only cloud placeholder (#30) unread only when SkipCloudContent says so; by default it is
// read as before. The placeholder is faked with the attribute bit on an ordinary local file (nothing is downloaded).
func TestMediaSkipsCloudPlaceholders(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.jpg"), buildExifJPEG("2021:05:06 07:08:09", binary.LittleEndian), 0o644)
	extFacts := ExternalImageFacts
	t.Cleanup(func() { SkipCloudContent = nil; ExternalImageFacts = extFacts })
	ExternalImageFacts = nil
	for _, c := range []struct {
		skip func() bool
		want bool
	}{{nil, true}, {func() bool { return false }, true}, {func() bool { return true }, false}} {
		SkipCloudContent = c.skip
		it := Item{Name: "x.jpg", Dir: dir, Attr: AttrCloud}
		LoadImageFacts(&it)
		if _, got := it.Extra[ExtraExifDate]; got != c.want {
			t.Errorf("skip=%v: read=%v, want %v", c.skip != nil && c.skip(), got, c.want)
		}
		plain := Item{Name: "x.jpg", Dir: dir}
		LoadImageFacts(&plain)
		if _, ok := plain.Extra[ExtraExifDate]; !ok {
			t.Error("an ordinary file was not read")
		}
	}
}
