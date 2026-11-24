package dirsize

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

// The folder a count starts on decides whether cloud storage counts: a home folder leaves its OneDrive and iCloud
// Drive folders (and OneDrive's own mirror of them) out, a cloud folder counted on its own gives its logical size.
func TestCloudStorageCountsOnlyWhenTheCountStartsInIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	gc := "Library/Group Containers/UBF8T346G9.OneDriveSyncClientSuite/"
	makeTree(t, home, map[string]int{
		"Documents/a.txt": 1000,
		"Library/CloudStorage/OneDrive-Firm/report.mov":           5000,
		"Library/Mobile Documents/com~apple~CloudDocs/notes.txt":  300,
		gc + "OneDrive - Firm.noindex/OneDrive - Firm/report.mov": 5000,
		gc + "logs/sync.log":                        200,
		gc + ".Dbfs.dbfs_bootstrap.noindex/dbfs.db": 50,
	})
	opts := Options{SameVolume: true}

	r, err := Calculate(context.Background(), home, opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Cloud || r.Bytes != 1250 {
		t.Errorf("home folder: cloud=%v bytes=%d, want false and 1250 (the cloud folders left out)", r.Cloud, r.Bytes)
	}
	if r.Size != r.Allocated {
		t.Errorf("home folder: size %d, want the size on disk %d", r.Size, r.Allocated)
	}
	want := []string{
		filepath.Join(home, "Library/CloudStorage"),
		filepath.Join(home, "Library/Mobile Documents"),
		filepath.Join(home, gc+"OneDrive - Firm.noindex"),
	}
	got := slices.Clone(r.SkippedMounts)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("skipped %v, want %v", got, want)
	}

	for _, c := range []struct {
		path  string
		bytes int64
	}{
		{"Library/CloudStorage", 5000},
		{"Library/CloudStorage/OneDrive-Firm", 5000},
		{"Library/Mobile Documents/com~apple~CloudDocs", 300},
		{gc + "OneDrive - Firm.noindex", 5000},
	} {
		r, err := Calculate(context.Background(), filepath.Join(home, c.path), opts)
		if err != nil {
			t.Fatal(err)
		}
		if !r.Cloud || r.Bytes != c.bytes || r.Size != c.bytes {
			t.Errorf("%s: cloud=%v bytes=%d size=%d, want true, %d, %d (the logical size)", c.path, r.Cloud, r.Bytes, r.Size, c.bytes, c.bytes)
		}
	}

	// OneDrive's group container itself is not cloud storage: its own files count, its mirror does not
	r, err = Calculate(context.Background(), filepath.Join(home, gc), opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Cloud || r.Bytes != 250 {
		t.Errorf("group container: cloud=%v bytes=%d, want false and 250", r.Cloud, r.Bytes)
	}
}
