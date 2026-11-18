//go:build !windows

package fileops

// F-467: with Request.ConfirmHidden a hidden target (a dot name here) gets its own question before it is overwritten;
// without it (the default) nothing changes. Everything runs in temp folders.

import (
	"path/filepath"
	"testing"
)

func TestHiddenTargetQuestion(t *testing.T) {
	setup := func() (string, string) {
		src, dst := t.TempDir(), t.TempDir()
		write(t, filepath.Join(src, ".h"), "new")
		write(t, filepath.Join(dst, ".h"), "old")
		write(t, filepath.Join(src, "v.txt"), "new")
		write(t, filepath.Join(dst, "v.txt"), "old")
		return src, dst
	}
	srcs := func(src string) []string { return []string{filepath.Join(src, ".h"), filepath.Join(src, "v.txt")} }

	// default: overwrite all asks nothing more
	src, dst := setup()
	q := NewQueue(nil)
	asked := 0
	q.AskFunc = func(p Progress) Answer { asked++; return Answer{Choice: ChoiceSkip} }
	p := start(t, q, Request{Kind: Copy, Sources: srcs(src), Target: dst, Overwrite: Overwrite})
	if p.State != StateDone || asked != 0 || read(t, filepath.Join(dst, ".h")) != "new" {
		t.Fatalf("default: %+v asked %d", p, asked)
	}

	// on, answered Skip: the hidden file stays, the visible one is overwritten
	src, dst = setup()
	q = NewQueue(nil)
	var types []AskType
	q.AskFunc = func(p Progress) Answer { types = append(types, p.Ask.Type); return Answer{Choice: ChoiceSkip} }
	p = start(t, q, Request{Kind: Copy, Sources: srcs(src), Target: dst, Overwrite: Overwrite, ConfirmHidden: true})
	if len(types) != 1 || types[0] != AskHidden || p.Skipped != 1 || read(t, filepath.Join(dst, ".h")) != "old" || read(t, filepath.Join(dst, "v.txt")) != "new" {
		t.Fatalf("skip: %+v %v", p, types)
	}

	// on, answered Overwrite all: written
	src, dst = setup()
	q = NewQueue(nil)
	q.AskFunc = func(p Progress) Answer { return Answer{Choice: ChoiceOverwriteAll} }
	p = start(t, q, Request{Kind: Copy, Sources: srcs(src), Target: dst, Overwrite: Overwrite, ConfirmHidden: true})
	if p.State != StateDone || read(t, filepath.Join(dst, ".h")) != "new" {
		t.Fatalf("overwrite: %+v", p)
	}
}
