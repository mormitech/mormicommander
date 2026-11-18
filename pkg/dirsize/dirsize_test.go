package dirsize_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mormicommander/pkg/dirsize"
)

// The public package must give the same engine: count a small folder and remember it in a store.
func TestPublicAPI(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), make([]byte, 1000), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "b"), make([]byte, 234), 0o644)
	var r dirsize.Result
	r, err := dirsize.Calculate(context.Background(), dir, dirsize.Options{SameVolume: true})
	if err != nil || r.Bytes != 1234 || r.Files != 2 || r.Dirs != 1 || !r.Complete {
		t.Fatalf("%+v %v", r, err)
	}
	s := dirsize.OpenStore(filepath.Join(dir, "store.bin"))
	s.PutResult(r)
	if got, ok := s.Get(dir); !ok || got.Bytes != 1234 {
		t.Errorf("%+v %v", got, ok)
	}
	if dirsize.FormatSize(1234, dirsize.UnitBytes) == "" {
		t.Error("FormatSize")
	}
}
