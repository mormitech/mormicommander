package fileops

// Section 39 of FEATURES.md (issue #87): plain error sentences (F-718) and the access-denied mark on the error
// question that lets the page explain what to do (F-720).

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestPlainErrSentences(t *testing.T) {
	cases := map[error]string{
		&fs.PathError{Op: "write", Path: "/x", Err: syscall.ENOSPC}:            "the disk is full",
		&fs.PathError{Op: "open", Path: "/x", Err: fs.ErrPermission}:           "permission denied",
		&os.LinkError{Op: "rename", Old: "a", New: "b", Err: fs.ErrNotExist}:   "it is not there any more",
		fmt.Errorf("wrapped: %w", &fs.PathError{Op: "x", Err: syscall.ENOSPC}): "the disk is full",
		errVerify: "the copy differs from the original",
	}
	for err, want := range cases {
		if got := plainErr(err); got != want {
			t.Errorf("plainErr(%v) = %q, want %q", err, got, want)
		}
	}
	if got := fmt.Sprintf("%s: %s.", "Can't copy a.txt", plainErr(&fs.PathError{Err: syscall.ENOSPC})); got != "Can't copy a.txt: the disk is full." {
		t.Fatal(got)
	}
	if plainErr(nil) != "" || plainErr(errors.New("odd")) != "odd" {
		t.Fatal("nil or unknown error")
	}
}

func TestErrorQuestionMarksAccessDenied(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs an unreadable file")
	}
	src, dst := t.TempDir(), t.TempDir()
	f := filepath.Join(src, "f")
	write(t, f, "data")
	os.Chmod(f, 0)
	defer os.Chmod(f, 0o644)
	q := NewQueue(nil)
	var asked []Conflict
	q.AskFunc = func(p Progress) Answer {
		asked = append(asked, *p.Ask)
		return Answer{Choice: ChoiceSkip}
	}
	start(t, q, Request{Kind: Copy, Sources: []string{f}, Target: dst, OnError: ErrorsAsk})
	if len(asked) != 1 || asked[0].Type != AskError || !asked[0].Denied || asked[0].Error != "Can't copy f: permission denied." {
		t.Fatalf("%+v", asked)
	}
}

func TestErrorQuestionNotDeniedForOtherErrors(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	f := filepath.Join(src, "f")
	write(t, f, "data")
	q := NewQueue(nil)
	var asked []Conflict
	q.AskFunc = func(p Progress) Answer {
		asked = append(asked, *p.Ask)
		return Answer{Choice: ChoiceSkip}
	}
	// the target folder is a file, so the folder can't be made: not an access problem
	block := filepath.Join(dst, "block")
	write(t, block, "x")
	start(t, q, Request{Kind: Copy, Sources: []string{f}, Target: filepath.Join(block, "sub"), OnError: ErrorsAsk})
	for _, a := range asked {
		if a.Type == AskError && a.Denied {
			t.Fatalf("%+v", a)
		}
	}
}
