// Tests for the watcher core: debouncing, reference counting, the global and per-path switches, exclusions, the
// fallback from native to polling, pausing while inactive and the gone/restart logic. A fake backend lets the tests
// inject raw notifications directly, so they check the watcher's own logic without depending on OS timing.
package fswatch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeBackend struct {
	mu      sync.Mutex
	report  func(raw)
	stopped bool
}

func (f *fakeBackend) stop() {
	f.mu.Lock()
	f.stopped = true
	f.mu.Unlock()
}

func (f *fakeBackend) send(r raw) {
	f.mu.Lock()
	rep, stopped := f.report, f.stopped
	f.mu.Unlock()
	if !stopped {
		rep(r)
	}
}

// fakes records the fake backends started, per kind.
type fakes struct {
	mu      sync.Mutex
	native  []*fakeBackend
	poll    []*fakeBackend
	failNat error
	network bool
}

func (fk *fakes) last(poll bool) *fakeBackend {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	l := fk.native
	if poll {
		l = fk.poll
	}
	if len(l) == 0 {
		return nil
	}
	return l[len(l)-1]
}

func (fk *fakes) count() (int, int) {
	fk.mu.Lock()
	defer fk.mu.Unlock()
	return len(fk.native), len(fk.poll)
}

// installFakes swaps the backend constructors for fakes until the test ends.
func installFakes(t *testing.T) *fakes {
	t.Helper()
	fk := &fakes{}
	on, op, oi := startNative, startPoll, isNetwork
	startNative = func(path string, _ Options, report func(raw)) (backend, error) {
		fk.mu.Lock()
		defer fk.mu.Unlock()
		if fk.failNat != nil {
			return nil, fk.failNat
		}
		b := &fakeBackend{report: report}
		fk.native = append(fk.native, b)
		return b, nil
	}
	startPoll = func(path string, _ Options, report func(raw)) (backend, error) {
		fk.mu.Lock()
		defer fk.mu.Unlock()
		b := &fakeBackend{report: report}
		fk.poll = append(fk.poll, b)
		return b, nil
	}
	isNetwork = func(string) bool {
		fk.mu.Lock()
		defer fk.mu.Unlock()
		return fk.network
	}
	t.Cleanup(func() { startNative, startPoll, isNetwork = on, op, oi })
	return fk
}

func fastOpts() Options {
	return Options{Debounce: 40 * time.Millisecond, MaxDelay: 400 * time.Millisecond, PollInterval: 50 * time.Millisecond}
}

func next(t *testing.T, w *Watcher, within time.Duration) Event {
	t.Helper()
	select {
	case ev, ok := <-w.Events():
		if !ok {
			t.Fatal("events channel closed")
		}
		return ev
	case <-time.After(within):
		t.Fatalf("no event within %v", within)
	}
	return Event{}
}

func none(t *testing.T, w *Watcher, within time.Duration) {
	t.Helper()
	select {
	case ev := <-w.Events():
		t.Fatalf("unexpected event %+v", ev)
	case <-time.After(within):
	}
}

func newW(t *testing.T, o Options) *Watcher {
	w := New(o)
	t.Cleanup(func() { w.Close() })
	return w
}

func TestDebounceMergesBurst(t *testing.T) {
	fk := installFakes(t)
	dir := t.TempDir()
	w := newW(t, fastOpts())
	if err := w.Watch(dir); err != nil {
		t.Fatal(err)
	}
	b := fk.last(false)
	for i := 0; i < 50; i++ {
		b.send(raw{Name: fmt.Sprintf("f%02d", i%10)})
	}
	ev := next(t, w, time.Second)
	if ev.Kind != Changed || ev.Full || len(ev.Names) != 10 || ev.Names[0] != "f00" || ev.Path != dir {
		t.Fatalf("got %+v", ev)
	}
	none(t, w, 150*time.Millisecond)
}

func TestMaxDelayCapsContinuousStream(t *testing.T) {
	fk := installFakes(t)
	dir := t.TempDir()
	o := fastOpts()
	o.Debounce, o.MaxDelay = 100*time.Millisecond, 200*time.Millisecond
	w := newW(t, o)
	if err := w.Watch(dir); err != nil {
		t.Fatal(err)
	}
	b := fk.last(false)
	start := time.Now()
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				b.send(raw{Name: "log.txt"})
			}
		}
	}()
	defer close(stop)
	ev := next(t, w, 2*time.Second)
	if d := time.Since(start); d > 700*time.Millisecond {
		t.Fatalf("event took %v despite MaxDelay", d)
	}
	if len(ev.Names) != 1 || ev.Names[0] != "log.txt" {
		t.Fatalf("got %+v", ev)
	}
}

func TestFullEvents(t *testing.T) {
	cases := []struct {
		name string
		send []raw
	}{
		{"overflow", []raw{{Name: "a"}, {Overflow: true}, {Name: "b"}}},
		{"unknown", []raw{{}}},
		{"too many names", func() []raw {
			var r []raw
			for i := 0; i <= maxNames+1; i++ {
				r = append(r, raw{Name: fmt.Sprint(i)})
			}
			return r
		}()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fk := installFakes(t)
			w := newW(t, fastOpts())
			if err := w.Watch(t.TempDir()); err != nil {
				t.Fatal(err)
			}
			for _, r := range c.send {
				fk.last(false).send(r)
			}
			ev := next(t, w, time.Second)
			if !ev.Full || ev.Names != nil {
				t.Fatalf("want Full, got %+v", ev)
			}
		})
	}
}

func TestRefcountAndUnwatch(t *testing.T) {
	fk := installFakes(t)
	dir := t.TempDir()
	w := newW(t, fastOpts())
	for i := 0; i < 2; i++ {
		if err := w.Watch(dir + string(filepath.Separator)); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := fk.count(); n != 1 {
		t.Fatalf("want one backend for a doubly watched folder, got %d", n)
	}
	b := fk.last(false)
	w.Unwatch(dir)
	if b.stopped || !w.Status(dir).Watched {
		t.Fatal("first Unwatch must keep the watch")
	}
	b.send(raw{Name: "x"}) // pending event must be dropped by the final Unwatch
	w.Unwatch(dir)
	if !b.stopped || w.Status(dir).Watched || len(w.Watched()) != 0 {
		t.Fatal("second Unwatch must stop the watch")
	}
	none(t, w, 150*time.Millisecond)
	if err := w.Unwatch(dir); err != nil {
		t.Fatal("unwatching an unwatched path must not fail")
	}
}

func TestPathSwitchAndGlobalSwitch(t *testing.T) {
	fk := installFakes(t)
	dir := t.TempDir()
	w := newW(t, fastOpts())
	if err := w.Watch(dir); err != nil {
		t.Fatal(err)
	}
	b1 := fk.last(false)

	w.SetPathDisabled(dir, true)
	st := w.Status(dir)
	if !b1.stopped || st.Active || st.Method != MethodOff || !st.Disabled || !strings.Contains(st.Reason, "off") {
		t.Fatalf("disabled: %+v", st)
	}
	if got := w.DisabledPaths(); len(got) != 1 || got[0] != dir {
		t.Fatalf("DisabledPaths = %v", got)
	}
	w.SetPathDisabled(dir, false)
	if ev := next(t, w, time.Second); !ev.Full {
		t.Fatalf("re-enable must announce a full reread, got %+v", ev)
	}
	if st := w.Status(dir); !st.Active || st.Method != MethodNative {
		t.Fatalf("re-enabled: %+v", st)
	}

	w.SetEnabled(false)
	if st := w.Status(dir); st.Active || !strings.Contains(st.Reason, "automatic refresh is off") {
		t.Fatalf("global off: %+v", st)
	}
	w.SetEnabled(true)
	if ev := next(t, w, time.Second); !ev.Full {
		t.Fatalf("global on must announce, got %+v", ev)
	}
	if n, _ := fk.count(); n != 3 {
		t.Fatalf("want 3 native starts, got %d", n)
	}

	// The per-path switch is remembered for paths that are not watched.
	other := t.TempDir()
	w.SetPathDisabled(other, true)
	w.Watch(other)
	if st := w.Status(other); st.Active || !st.Disabled {
		t.Fatalf("pre-disabled path: %+v", st)
	}
}

func TestStartDisabled(t *testing.T) {
	fk := installFakes(t)
	o := fastOpts()
	o.Disabled = true
	w := newW(t, o)
	dir := t.TempDir()
	w.Watch(dir)
	if n, p := fk.count(); n+p != 0 || w.Status(dir).Active {
		t.Fatal("no backend may start while disabled")
	}
}

func TestExcludeOption(t *testing.T) {
	fk := installFakes(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "logs")
	os.Mkdir(sub, 0o755)
	o := fastOpts()
	o.Exclude = []string{dir}
	w := newW(t, o)
	w.Watch(sub)
	if st := w.Status(sub); st.Active || !st.Excluded {
		t.Fatalf("excluded: %+v", st)
	}
	w.SetExclude(nil)
	if ev := next(t, w, time.Second); !ev.Full || ev.Path != sub {
		t.Fatalf("got %+v", ev)
	}
	if n, _ := fk.count(); n != 1 {
		t.Fatal("watch must start after exclusion is removed")
	}
}

func TestNativeFailureFallsBackToPoll(t *testing.T) {
	fk := installFakes(t)
	fk.failNat = errors.New("too many open files")
	w := newW(t, fastOpts())
	dir := t.TempDir()
	w.Watch(dir)
	st := w.Status(dir)
	if st.Method != MethodPoll || !st.Active || !strings.Contains(st.Reason, "too many open files") {
		t.Fatalf("got %+v", st)
	}
	fk.last(true).send(raw{Name: "a"})
	if ev := next(t, w, time.Second); len(ev.Names) != 1 {
		t.Fatalf("got %+v", ev)
	}
}

func TestMethodsPerDriveKind(t *testing.T) {
	cases := []struct {
		network     bool
		local, netw Method
		want        Method
	}{
		{false, MethodDefault, MethodDefault, MethodNative},
		{true, MethodDefault, MethodDefault, MethodPoll},
		{true, MethodDefault, MethodNative, MethodNative},
		{true, MethodDefault, MethodOff, MethodOff},
		{false, MethodPoll, MethodDefault, MethodPoll},
		{false, MethodOff, MethodDefault, MethodOff},
	}
	for i, c := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			fk := installFakes(t)
			fk.network = c.network
			o := fastOpts()
			o.Local, o.Network = c.local, c.netw
			w := newW(t, o)
			dir := t.TempDir()
			w.Watch(dir)
			st := w.Status(dir)
			if st.Method != c.want || st.Network != c.network {
				t.Fatalf("got %+v want %v", st, c.want)
			}
			// Switching at runtime restarts with the new method.
			w.SetMethods(MethodPoll, MethodPoll)
			if st := w.Status(dir); st.Method != MethodPoll {
				t.Fatalf("after SetMethods: %+v", st)
			}
		})
	}
}

func TestPauseWhenInactive(t *testing.T) {
	fk := installFakes(t)
	o := fastOpts()
	o.PauseWhenInactive = true
	w := newW(t, o)
	dir := t.TempDir()
	w.Watch(dir)
	w.SetActive(false)
	fk.last(false).send(raw{Name: "a"})
	none(t, w, 200*time.Millisecond)
	fk.last(false).send(raw{Name: "b"})
	time.Sleep(100 * time.Millisecond)
	w.SetActive(true)
	ev := next(t, w, time.Second)
	if strings.Join(ev.Names, ",") != "a,b" {
		t.Fatalf("held events must merge, got %+v", ev)
	}
}

func TestTerminalRestartOrGone(t *testing.T) {
	fk := installFakes(t)
	w := newW(t, fastOpts())
	dir := t.TempDir()
	w.Watch(dir)
	b1 := fk.last(false)

	// The folder still exists: restart with a full reread.
	b1.send(raw{Terminal: true})
	if ev := next(t, w, time.Second); ev.Kind != Changed || !ev.Full {
		t.Fatalf("got %+v", ev)
	}
	b2 := fk.last(false)
	if b2 == b1 || !b1.stopped {
		t.Fatal("backend must be replaced")
	}

	// The folder is gone.
	os.Remove(dir)
	b2.send(raw{Terminal: true})
	ev := next(t, w, time.Second)
	if ev.Kind != Gone || ev.Path != dir {
		t.Fatalf("got %+v", ev)
	}
	if st := w.Status(dir); !st.Watched || st.Active {
		t.Fatalf("gone status %+v", st)
	}
	// Late reports from the dead backend are ignored.
	b2.report(raw{Name: "late"})
	none(t, w, 150*time.Millisecond)
}

func TestSoftErrorEvent(t *testing.T) {
	fk := installFakes(t)
	w := newW(t, fastOpts())
	dir := t.TempDir()
	w.Watch(dir)
	fk.last(false).send(raw{Soft: errors.New("share unreachable")})
	ev := next(t, w, time.Second)
	if ev.Kind != Error || ev.Err == nil {
		t.Fatalf("got %+v", ev)
	}
}

func TestWatchErrorsAndClose(t *testing.T) {
	installFakes(t)
	w := New(fastOpts())
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	os.WriteFile(file, nil, 0o644)
	if err := w.Watch(file); err == nil {
		t.Fatal("watching a file must fail")
	}
	if err := w.Watch(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("watching a missing folder must fail")
	}
	if err := w.Watch(""); err == nil {
		t.Fatal("empty path must fail")
	}
	w.Watch(dir)
	w.Close()
	w.Close()
	if _, ok := <-w.Events(); ok {
		t.Fatal("Events must be closed")
	}
	if err := w.Watch(dir); err != ErrClosed {
		t.Fatalf("got %v", err)
	}
}

func TestSlowReaderMergesQueue(t *testing.T) {
	fk := installFakes(t)
	w := newW(t, fastOpts())
	dir := t.TempDir()
	w.Watch(dir)
	// Fill the output channel buffer and the queue without reading.
	for i := 0; i < 40; i++ {
		fk.last(false).send(raw{Name: fmt.Sprint(i)})
		time.Sleep(60 * time.Millisecond)
	}
	w.mu.Lock()
	q := len(w.queue)
	w.mu.Unlock()
	if q > 1 {
		t.Fatalf("queue must hold at most one Event per path, has %d", q)
	}
}

func TestMethodAndKindStrings(t *testing.T) {
	if MethodNative.String() != "native" || MethodPoll.String() != "poll" || MethodOff.String() != "off" ||
		MethodDefault.String() != "default" || Changed.String() != "changed" || Gone.String() != "gone" ||
		Error.String() != "error" {
		t.Fatal("bad String output")
	}
}
