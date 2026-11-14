// Tests for the engine review findings of issue #91 that concern 7z/RAR
// unpacking: with "unpack path names" off, two different entries of the same
// name must not silently replace each other just because the first was
// written in the same run, and stored permission bits are masked with the
// process umask rather than a fixed 022. Everything runs in t.TempDir().
package sevenrar

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"mormicommander/internal/archive/arcapi"
)

func TestExtractFlattenCollisionAsks(t *testing.T) {
	files := []tFile{
		{name: "a/same.txt", data: []byte("first")},
		{name: "b/same.txt", data: []byte("second")},
	}
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			a := openT(t, makeArchive(t, format, files))
			// Default (ask, no callback = skip): the first file stays.
			dest := t.TempDir()
			res, err := a.Extract(context.Background(), dest, ExtractOptions{FlattenPaths: true})
			if err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(filepath.Join(dest, "same.txt")); string(b) != "first" {
				t.Errorf("same.txt = %q, want the first entry kept", b)
			}
			if len(res.Skipped) != 1 {
				t.Errorf("skipped = %+v", res.Skipped)
			}
			// The question is asked, and "rename" keeps both.
			asked := 0
			dest = t.TempDir()
			_, err = a.Extract(context.Background(), dest, ExtractOptions{FlattenPaths: true,
				OnConflict: func(c Conflict) ConflictDecision { asked++; return ConflictDecision{Action: ActionRename} }})
			if err != nil {
				t.Fatal(err)
			}
			if asked != 1 {
				t.Errorf("asked %d times", asked)
			}
			got := tree(t, dest)
			if got != "same (2).txt=second\nsame.txt=first" {
				t.Errorf("tree = %q", got)
			}
		})
	}
}

func TestExtractAppliesUmask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no umask on Windows")
	}
	files := []tFile{{name: "open.sh", data: []byte("#!/bin/sh\n"), mode: 0o777}}
	a := openT(t, makeArchive(t, "7z", files))
	dest := t.TempDir()
	if _, err := a.Extract(context.Background(), dest, ExtractOptions{}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dest, "open.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := st.Mode().Perm(), arcapi.ApplyUmask(0o777); got != want {
		t.Errorf("mode %o, want %o", got, want)
	}
}
