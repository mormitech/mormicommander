package ziparc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mormicommander/internal/archive"
)

// A split that fails part way leaves the parts that were already there alone (#149): parts are written to temporary
// files and get their real names only when the whole set is written. A finished split still replaces an older set.
func TestSplitFailureKeepsExistingParts(t *testing.T) {
	src, _ := makeTree(t)
	d := t.TempDir()
	single := filepath.Join(d, "single.zip")
	pack(t, single, archive.Changes{Add: []archive.AddItem{{Source: src}}}, archive.PackOptions{Level: 9})
	main := filepath.Join(d, "multi.zip")
	old := filepath.Join(d, "multi.z01")
	os.WriteFile(old, []byte("an older part"), 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := splitVolumes(ctx, single, main, 100<<10); err == nil {
		t.Fatal("a cancelled split succeeded")
	}
	if b, err := os.ReadFile(old); err != nil || string(b) != "an older part" {
		t.Fatalf("the existing part was changed or removed by a failed split: %q, %v", b, err)
	}
	if des, _ := os.ReadDir(d); len(des) != 2 {
		t.Fatalf("left behind: %v", des)
	}

	names, err := splitVolumes(context.Background(), single, main, 100<<10)
	if err != nil || len(names) < 2 {
		t.Fatalf("split: %v %v", names, err)
	}
	if b, _ := os.ReadFile(old); string(b) == "an older part" {
		t.Fatal("a finished split did not replace the older part")
	}
	if _, err := Open(main, archive.OpenOptions{}); err != nil {
		t.Fatalf("the split set does not open: %v", err)
	}
}
