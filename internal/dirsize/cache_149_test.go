package dirsize

import (
	"runtime"
	"testing"

	"golang.org/x/text/unicode/norm"
)

// On macOS a size stored under the NFD form of a folder name is found under the NFC form (#149).
func TestCacheKeyUnicodeForm(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("only APFS folds the Unicode form")
	}
	var c Cache
	c.Put(Result{Path: norm.NFD.String("/x/café"), Complete: true, Bytes: 7})
	if r, ok := c.Get(norm.NFC.String("/x/café")); !ok || r.Bytes != 7 {
		t.Fatalf("NFC lookup of an NFD key: %v %v", r, ok)
	}
}
