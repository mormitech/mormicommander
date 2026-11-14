// This file checks the shared rule for turning server paths into local relative paths during tree downloads.

package remotefs

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalRel(t *testing.T) {
	ok := map[string]string{"a.txt": "a.txt", "d/a.txt": filepath.Join("d", "a.txt"), "a b/c\"d": filepath.Join("a b", "c\"d")}
	if runtime.GOOS == "windows" {
		delete(ok, "a b/c\"d")
	}
	for in, want := range ok {
		got, err := LocalRel(in)
		if err != nil || got != want {
			t.Errorf("LocalRel(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{"", "..", "../x", "d/../../x", "/abs", "d//x"}
	if runtime.GOOS == "windows" {
		bad = append(bad, `..\x`, `d\x`, "CON", "a:b")
	}
	for _, in := range bad {
		if got, err := LocalRel(in); !errors.Is(err, ErrUnsafeName) {
			t.Errorf("LocalRel(%q) = %q, %v; want ErrUnsafeName", in, got, err)
		}
	}
}
