package fileops

// Tests of the overwrite rules and answers: every policy of TC 11's overwrite-mode dropdown, the dialog answers that
// go beyond overwrite/skip (append, resume, compare, auto-rename, rename target, larger/smaller), the read-only
// question, and that a link in the target is replaced rather than written through.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// pair makes src/f with srcData and dst/f with dstData and returns the folders.
func pair(t *testing.T, srcData, dstData string) (string, string) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "f.txt"), srcData)
	write(t, filepath.Join(dst, "f.txt"), dstData)
	return src, dst
}

func TestPolicies(t *testing.T) {
	for _, c := range []struct {
		policy   Policy
		src, dst string
		want     map[string]string
	}{
		{Overwrite, "new", "old", map[string]string{"f.txt": "new"}},
		{Skip, "new", "old", map[string]string{"f.txt": "old"}},
		{Larger, "longer", "old", map[string]string{"f.txt": "longer"}},
		{Larger, "s", "old", map[string]string{"f.txt": "old"}},
		{Smaller, "s", "old", map[string]string{"f.txt": "s"}},
		{Smaller, "longer", "old", map[string]string{"f.txt": "old"}},
		{AutoRenameSource, "new", "old", map[string]string{"f.txt": "old", "f(2).txt": "new"}},
		{AutoRenameTarget, "new", "old", map[string]string{"f.txt": "new", "f(2).txt": "old"}},
	} {
		src, dst := pair(t, c.src, c.dst)
		p := start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{filepath.Join(src, "f.txt")}, Target: dst, Overwrite: c.policy})
		if p.State != StateDone {
			t.Errorf("%s: %+v", c.policy, p)
		}
		for n, s := range c.want {
			if got := read(t, filepath.Join(dst, n)); got != s {
				t.Errorf("%s: %s = %q, want %q", c.policy, n, got, s)
			}
		}
	}
}

func TestAnswers(t *testing.T) {
	for _, c := range []struct {
		name     string
		src, dst string
		answers  []Choice
		want     map[string]string
	}{
		{"append", "+more", "base", []Choice{ChoiceAppend}, map[string]string{"f.txt": "base+more"}},
		{"resume", "0123456789", "01234", []Choice{ChoiceResume}, map[string]string{"f.txt": "0123456789"}},
		{"auto rename", "new", "old", []Choice{ChoiceAutoRename}, map[string]string{"f.txt": "old", "f(2).txt": "new"}},
		{"rename target", "new", "old", []Choice{ChoiceRenameTarget}, map[string]string{"f.txt": "new", "f(2).txt": "old"}},
		{"larger all", "big source", "old", []Choice{ChoiceLargerAll}, map[string]string{"f.txt": "big source"}},
		{"smaller all", "big source", "old", []Choice{ChoiceSmallerAll}, map[string]string{"f.txt": "old"}},
		{"compare then skip", "same", "same", []Choice{ChoiceCompare, ChoiceSkip}, map[string]string{"f.txt": "same"}},
	} {
		src, dst := pair(t, c.src, c.dst)
		q := NewQueue(nil)
		var asks []Conflict
		q.AskFunc = func(p Progress) Answer {
			asks = append(asks, *p.Ask)
			return Answer{Choice: c.answers[len(asks)-1]}
		}
		p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "f.txt")}, Target: dst})
		if p.State != StateDone || len(asks) != len(c.answers) {
			t.Errorf("%s: asked %d times, %+v", c.name, len(asks), p)
			continue
		}
		for n, s := range c.want {
			if got := read(t, filepath.Join(dst, n)); got != s {
				t.Errorf("%s: %s = %q, want %q", c.name, n, got, s)
			}
		}
		if c.name == "compare then skip" && (asks[1].Same == nil || !*asks[1].Same || asks[0].Same != nil) {
			t.Errorf("compare result not reported: %+v", asks)
		}
		if c.name == "resume" && !hasChoice(asks[0].Choices, ChoiceResume) {
			t.Error("resume not offered")
		}
	}
}

func hasChoice(cs []Choice, c Choice) bool {
	for _, x := range cs {
		if x == c {
			return true
		}
	}
	return false
}

func TestReadOnlyTarget(t *testing.T) {
	src, dst := pair(t, "new", "old")
	os.Chmod(filepath.Join(dst, "f.txt"), 0o444)
	defer os.Chmod(filepath.Join(dst, "f.txt"), 0o644)
	q := NewQueue(nil)
	var types []AskType
	q.AskFunc = func(p Progress) Answer {
		types = append(types, p.Ask.Type)
		return Answer{Choice: ChoiceOverwrite}
	}
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "f.txt")}, Target: dst})
	if p.State != StateDone || len(types) != 2 || types[1] != AskReadOnly || read(t, filepath.Join(dst, "f.txt")) != "new" {
		t.Fatalf("%+v %v", p, types)
	}

	src, dst = pair(t, "new", "old")
	os.Chmod(filepath.Join(dst, "f.txt"), 0o444)
	defer os.Chmod(filepath.Join(dst, "f.txt"), 0o644)
	p = start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{filepath.Join(src, "f.txt")}, Target: dst,
		Overwrite: Overwrite, ReadOnly: Skip})
	if p.Skipped != 1 || read(t, filepath.Join(dst, "f.txt")) != "old" {
		t.Fatalf("read-only skip: %+v", p)
	}
}

func TestOverwriteLinkNotFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	src, dst := t.TempDir(), t.TempDir()
	other := filepath.Join(t.TempDir(), "victim")
	write(t, other, "keep me")
	write(t, filepath.Join(src, "f.txt"), "new")
	os.Symlink(other, filepath.Join(dst, "f.txt"))
	p := start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{filepath.Join(src, "f.txt")}, Target: dst, Overwrite: Overwrite})
	if p.State != StateDone || read(t, other) != "keep me" || read(t, filepath.Join(dst, "f.txt")) != "new" {
		t.Fatalf("%+v", p)
	}
	if fi, _ := os.Lstat(filepath.Join(dst, "f.txt")); isLink(fi) {
		t.Error("link still there")
	}
}
