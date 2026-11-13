// End-to-end tests with the real backends on a temporary folder: the native backend of the platform running the
// tests and the polling backend. Each scenario changes the folder the way another program would and checks that an
// Event naming the right entries arrives within the time FEATURES F-065 allows, and that deleting the watched folder
// itself produces Gone.
package fswatch

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// waitNames reads Events until all wanted names have been seen (or a Full event arrives), or fails.
func waitNames(t *testing.T, w *Watcher, within time.Duration, want ...string) {
	t.Helper()
	seen := map[string]bool{}
	deadline := time.After(within)
	for {
		missing := []string{}
		for _, n := range want {
			if !seen[n] {
				missing = append(missing, n)
			}
		}
		if len(missing) == 0 {
			return
		}
		select {
		case ev := <-w.Events():
			if ev.Kind == Changed && ev.Full {
				return
			}
			if ev.Kind != Changed {
				t.Fatalf("unexpected event %+v", ev)
			}
			for _, n := range ev.Names {
				seen[n] = true
			}
		case <-deadline:
			got := make([]string, 0, len(seen))
			for n := range seen {
				got = append(got, n)
			}
			sort.Strings(got)
			t.Fatalf("missing %v within %v (saw %v)", missing, within, got)
		}
	}
}

func drain(w *Watcher, d time.Duration) {
	end := time.After(d)
	for {
		select {
		case <-w.Events():
		case <-end:
			return
		}
	}
}

func TestRealBackends(t *testing.T) {
	methods := []Method{MethodNative, MethodPoll}
	for _, m := range methods {
		t.Run(m.String(), func(t *testing.T) {
			dir := t.TempDir()
			existing := filepath.Join(dir, "existing.txt")
			if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			o := Options{Debounce: 50 * time.Millisecond, MaxDelay: 300 * time.Millisecond,
				PollInterval: 100 * time.Millisecond, Local: m, Network: m}
			w := New(o)
			defer w.Close()
			if err := w.Watch(dir); err != nil {
				t.Fatal(err)
			}
			st := w.Status(dir)
			if st.Method != m || !st.Active {
				t.Fatalf("status %+v", st)
			}

			// Created file.
			os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello"), 0o644)
			waitNames(t, w, 2*time.Second, "new.txt")
			drain(w, 200*time.Millisecond)

			// Growing an existing file (only seen by kqueue thanks to the per-file watches).
			f, err := os.OpenFile(existing, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			f.WriteString("more data")
			f.Close()
			waitNames(t, w, 2*time.Second, "existing.txt")
			drain(w, 200*time.Millisecond)

			// Rename reports both names.
			os.Rename(filepath.Join(dir, "new.txt"), filepath.Join(dir, "renamed.txt"))
			waitNames(t, w, 2*time.Second, "new.txt", "renamed.txt")
			drain(w, 200*time.Millisecond)

			// New subfolder and deletion.
			os.Mkdir(filepath.Join(dir, "sub"), 0o755)
			os.Remove(existing)
			waitNames(t, w, 2*time.Second, "sub", "existing.txt")
			drain(w, 200*time.Millisecond)

			// Changes inside a subfolder are not reported (the watch is not recursive).
			os.WriteFile(filepath.Join(dir, "sub", "deep.txt"), nil, 0o644)
			select {
			case ev := <-w.Events():
				for _, n := range ev.Names {
					if strings.Contains(n, "deep") {
						t.Fatalf("recursive change reported: %+v", ev)
					}
				}
			case <-time.After(400 * time.Millisecond):
			}

			// Deleting the watched folder itself gives Gone.
			if err := os.RemoveAll(dir); err != nil {
				t.Fatal(err)
			}
			deadline := time.After(3 * time.Second)
			for {
				select {
				case ev := <-w.Events():
					if ev.Kind == Gone {
						if ev.Path != dir {
							t.Fatalf("gone for %q", ev.Path)
						}
						if st := w.Status(dir); st.Active {
							t.Fatalf("still active after gone: %+v", st)
						}
						return
					}
				case <-deadline:
					t.Fatal("no Gone event after deleting the watched folder")
				}
			}
		})
	}
}

func TestPollSoftErrorAndRecovery(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("permission bits do not block reading here")
	}
	dir := t.TempDir()
	w := New(Options{Debounce: 30 * time.Millisecond, PollInterval: 50 * time.Millisecond, Local: MethodPoll})
	defer w.Close()
	if err := w.Watch(dir); err != nil {
		t.Fatal(err)
	}
	os.Chmod(dir, 0o000)
	defer os.Chmod(dir, 0o755)
	select {
	case ev := <-w.Events():
		if ev.Kind != Error {
			t.Fatalf("want Error, got %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no Error event for an unreadable folder")
	}
	os.Chmod(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "back.txt"), nil, 0o644)
	waitNames(t, w, 2*time.Second, "back.txt")
}

func TestKqueueFileWatchLimit(t *testing.T) {
	if runtime.GOOS != "darwin" && !strings.HasSuffix(runtime.GOOS, "bsd") {
		t.Skip("kqueue only")
	}
	dir := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		os.WriteFile(filepath.Join(dir, n), nil, 0o644)
	}
	be, err := startNativeBackend(dir, Options{FileWatchLimit: 2}, func(raw) {})
	if err != nil {
		t.Fatal(err)
	}
	defer be.stop()
	kb := be.(interface{ watchedFiles() int })
	if n := kb.watchedFiles(); n != 2 {
		t.Fatalf("watched %d files, want 2", n)
	}
	be2, err := startNativeBackend(dir, Options{FileWatchLimit: -1}, func(raw) {})
	if err != nil {
		t.Fatal(err)
	}
	defer be2.stop()
	if n := be2.(interface{ watchedFiles() int }).watchedFiles(); n != 0 {
		t.Fatalf("watched %d files, want 0", n)
	}
}

func TestDiffSnapshots(t *testing.T) {
	a := map[string]pollStat{"same": {1, 1, 0}, "grow": {1, 1, 0}, "gone": {1, 1, 0}, "touch": {1, 1, 0}}
	b := map[string]pollStat{"same": {1, 1, 0}, "grow": {2, 1, 0}, "new": {0, 0, 0}, "touch": {1, 2, 0}}
	got := diffSnapshots(a, b)
	sort.Strings(got)
	if strings.Join(got, ",") != "gone,grow,new,touch" {
		t.Fatalf("got %v", got)
	}
	if len(diffSnapshots(a, a)) != 0 {
		t.Fatal("identical snapshots must not differ")
	}
}

func TestStopIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	opts := Options{PollInterval: time.Second, FileWatchLimit: 10}.withDefaults()
	for name, start := range map[string]startFunc{"native": startNativeBackend, "poll": startPollBackend} {
		be, err := start(dir, opts, func(raw) {})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		be.stop()
		be.stop()
	}
}
