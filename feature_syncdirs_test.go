package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A sync must act on the pair of folders compared last, never on folders the window shows after a change.
func TestSyncDirsSamePair(t *testing.T) {
	base := t.TempDir()
	a, b, c := filepath.Join(base, "A"), filepath.Join(base, "B"), filepath.Join(base, "C")
	for _, d := range []string{a, b, c} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s := &SyncDirs{app: &App{}, sess: map[int]*syncSession{}}
	ss := &syncSession{left: a, right: c}
	if err := s.syncSamePair(ss, "", ""); err != nil {
		t.Fatalf("no paths sent: %v", err)
	}
	if err := s.syncSamePair(ss, a, c+string(filepath.Separator)); err != nil {
		t.Fatalf("same pair refused: %v", err)
	}
	if err := s.syncSamePair(ss, a, b); err == nil {
		t.Fatal("a sync to B after comparing A with C was allowed")
	}
	if err := s.syncSamePair(ss, c, a); err == nil {
		t.Fatal("a swapped pair was allowed")
	}
}
