// Tests for review findings of issue #91 (second engines review).

package fileutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A file named like a part ("data.003") is split into three parts, whose last
// one has that very name. When that last part is missing (on another disk)
// and the original is combined again, the result is created under the
// missing part's name; it must then not be taken for the missing part and
// appended to itself, which grew it until the disk was full.
func TestCombineDoesNotAppendTargetToItself(t *testing.T) {
	dir := t.TempDir()
	src, _ := writeData(t, dir, "data.003", 3000)
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := SplitFile(context.Background(), SplitOptions{Source: src, TargetDir: out, PartSize: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Parts) != 3 || filepath.Base(res.Parts[2]) != "data.003" {
		t.Fatalf("parts = %v", res.Parts)
	}
	if err := os.Remove(res.Parts[2]); err != nil { // the third part is on another disk
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = CombineFiles(ctx, CombineOptions{FirstPart: res.Parts[0]})
	if err == nil {
		t.Fatal("combining without the third part worked")
	}
	if errors.Is(err, ErrCanceled) {
		t.Fatalf("combine ran until the timeout (appending the result to itself): %v", err)
	}
	if fi, err := os.Stat(filepath.Join(out, "data.003")); err == nil {
		t.Errorf("a %d-byte result was left behind", fi.Size())
	}
}

// The part a NextPart callback hands back must not be the file being written
// either.
func TestCombineRefusesTargetAsNextPart(t *testing.T) {
	dir := t.TempDir()
	src, _ := writeData(t, dir, "movie.mkv", 3000)
	res, err := SplitFile(context.Background(), SplitOptions{Source: src, TargetDir: dir, PartSize: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(res.Parts[2]); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = CombineFiles(ctx, CombineOptions{FirstPart: res.Parts[0], NextPart: func(int, string) (string, error) {
		return src, nil
	}})
	if err == nil || errors.Is(err, ErrCanceled) {
		t.Fatalf("err = %v, want a refusal", err)
	}
}
