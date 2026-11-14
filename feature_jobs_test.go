package main

// Tests of the Go side of the transfer error list (board issue #15): the error log file is written whole with a final
// line break, and SkipErrors refuses a job that does not exist.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mormicommander/internal/fileops"
)

func TestWriteErrorLog(t *testing.T) {
	p := filepath.Join(t.TempDir(), "errors.txt")
	if err := writeErrorLog(p, "a: permission denied\r\nb: the disk is full"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.ReplaceAll(string(b), "\r\n", "\n")
	if got != "a: permission denied\nb: the disk is full\n" {
		t.Fatalf("%q", got)
	}
	if os.PathSeparator == '/' && strings.Contains(string(b), "\r") {
		t.Error("Windows line breaks outside Windows")
	}
}

func TestJobsSkipErrorsUnknownJob(t *testing.T) {
	j := &Jobs{q: fileops.NewQueue(nil)}
	if j.SkipErrors(42) {
		t.Error("SkipErrors accepted a job that does not exist")
	}
}
