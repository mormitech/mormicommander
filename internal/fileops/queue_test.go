package fileops

// Tests of the queue as a transfer manager: pausing and resuming, the queue lane (one job after another), cancelling
// a job that is still waiting, starting a waiting job at once, the speed limit, the final statistics and the error
// policy (retry, stop, automatic retries).

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPauseResume(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "f"), strings.Repeat("x", 100000))
	q := NewQueue(nil)
	ids := make(chan int, 1)
	hit := make(chan struct{})
	var once sync.Once
	q.beforeFile = func(string) { once.Do(func() { q.Pause(<-ids); close(hit) }) }
	id, _ := q.Start(Request{Kind: Copy, Sources: []string{filepath.Join(src, "f")}, Target: dst})
	ids <- id
	<-hit
	p, _ := q.Get(id)
	if p.State != StatePaused {
		t.Fatalf("state %s, want paused", p.State)
	}
	time.Sleep(50 * time.Millisecond)
	if p, _ := q.Get(id); p.BytesDone != 0 {
		t.Fatalf("a paused job copied %d bytes", p.BytesDone)
	}
	if !q.Resume(id) {
		t.Fatal("resume refused")
	}
	p = q.Wait(id)
	if p.State != StateDone || p.BytesDone != 100000 {
		t.Fatalf("%+v", p)
	}
	if q.Pause(id) || q.Resume(id) {
		t.Error("pause/resume of a finished job accepted")
	}
}

func TestQueueLane(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "a"), "a")
	write(t, filepath.Join(src, "b"), "b")
	write(t, filepath.Join(src, "c"), "c")
	q := NewQueue(nil)
	release := make(chan struct{})
	var mu sync.Mutex
	var order []string
	q.beforeFile = func(p string) {
		mu.Lock()
		order = append(order, filepath.Base(p))
		mu.Unlock()
		if filepath.Base(p) == "a" {
			<-release
		}
	}
	id1, _ := q.Start(Request{Kind: Copy, Sources: []string{filepath.Join(src, "a")}, Target: dst, Queued: true})
	id2, _ := q.Start(Request{Kind: Copy, Sources: []string{filepath.Join(src, "b")}, Target: dst, Queued: true})
	id3, _ := q.Start(Request{Kind: Copy, Sources: []string{filepath.Join(src, "c")}, Target: dst, Queued: true})
	waitFor(t, "first job", func() bool { mu.Lock(); defer mu.Unlock(); return len(order) == 1 })
	if p, _ := q.Get(id2); p.State != StateQueued || !p.Queued {
		t.Fatalf("second job %+v", p)
	}
	if !q.Cancel(id3) {
		t.Fatal("cancel of a waiting job refused")
	}
	if p := q.Wait(id3); p.State != StateCancelled {
		t.Fatalf("waiting job after cancel: %+v", p)
	}
	close(release)
	if p := q.Wait(id1); p.State != StateDone {
		t.Fatalf("%+v", p)
	}
	if p := q.Wait(id2); p.State != StateDone {
		t.Fatalf("%+v", p)
	}
	if strings.Join(order, "") != "ab" || exists(filepath.Join(dst, "c")) {
		t.Errorf("order %v", order)
	}
	q.Forget()
	if len(q.List()) != 0 {
		t.Error("Forget kept finished jobs")
	}
}

func TestRunNow(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "a"), "a")
	write(t, filepath.Join(src, "b"), "b")
	q := NewQueue(nil)
	release := make(chan struct{})
	q.beforeFile = func(p string) {
		if filepath.Base(p) == "a" {
			<-release
		}
	}
	id1, _ := q.Start(Request{Kind: Copy, Sources: []string{filepath.Join(src, "a")}, Target: dst, Queued: true})
	id2, _ := q.Start(Request{Kind: Copy, Sources: []string{filepath.Join(src, "b")}, Target: dst, Queued: true})
	if !q.RunNow(id2) {
		t.Fatal("RunNow refused")
	}
	if p := q.Wait(id2); p.State != StateDone || p.Queued {
		t.Fatalf("%+v", p)
	}
	if q.RunNow(id1) {
		t.Error("RunNow of a running job accepted")
	}
	close(release)
	q.Wait(id1)
}

func TestSpeedLimit(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "f"), strings.Repeat("x", 200<<10))
	q := NewQueue(nil)
	begin := time.Now()
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "f")}, Target: dst, SpeedLimit: 500 << 10})
	if took := time.Since(begin); took < 300*time.Millisecond {
		t.Errorf("200 KB at 500 KB/s took only %v", took)
	}
	if p.State != StateDone || p.SpeedLimit != 500<<10 || read(t, filepath.Join(dst, "f")) != strings.Repeat("x", 200<<10) {
		t.Fatalf("%+v", p)
	}
}

func TestSetSpeedLimitWhileRunning(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "f"), strings.Repeat("x", 1<<20))
	q := NewQueue(nil)
	id, _ := q.Start(Request{Kind: Copy, Sources: []string{filepath.Join(src, "f")}, Target: dst, SpeedLimit: 64 << 10})
	waitFor(t, "first bytes", func() bool { p, _ := q.Get(id); return p.BytesDone > 0 })
	if !q.SetSpeedLimit(id, 0) {
		t.Fatal("SetSpeedLimit refused")
	}
	done := make(chan Progress)
	go func() { done <- q.Wait(id) }()
	select {
	case p := <-done:
		if p.State != StateDone || p.SpeedLimit != 0 {
			t.Fatalf("%+v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lifting the limit didn't speed the job up")
	}
}

func TestSummaryAndTiming(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "f"), "hello")
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "f")}, Target: dst})
	if !strings.HasPrefix(p.Summary, "Copied 1 of 1 files (5 bytes)") || p.ETA != 0 || p.Elapsed < 0 {
		t.Fatalf("%+v", p)
	}
}

func TestErrorAskRetry(t *testing.T) {
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
		os.Chmod(f, 0o644)
		return Answer{Choice: ChoiceRetry}
	}
	p := start(t, q, Request{Kind: Copy, Sources: []string{f}, Target: dst, OnError: ErrorsAsk})
	os.Chmod(filepath.Join(dst, "f"), 0o644) // the copy got the source's mode from before the fix
	if p.State != StateDone || len(asked) != 1 || asked[0].Type != AskError || read(t, filepath.Join(dst, "f")) != "data" {
		t.Fatalf("%+v %+v", p, asked)
	}
}

func TestErrorStopAndSkipAll(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs an unreadable file")
	}
	src := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		write(t, filepath.Join(src, n), n)
		os.Chmod(filepath.Join(src, n), 0)
		defer os.Chmod(filepath.Join(src, n), 0o644)
	}
	srcs := []string{filepath.Join(src, "a"), filepath.Join(src, "b"), filepath.Join(src, "c")}
	q := NewQueue(nil)
	dst := t.TempDir()
	p := start(t, q, Request{Kind: Copy, Sources: srcs, Target: dst, OnError: ErrorsStop})
	if p.State != StateCancelled || len(p.Errors) != 1 {
		t.Fatalf("stop: %+v", p)
	}
	asks := 0
	q.AskFunc = func(p Progress) Answer { asks++; return Answer{Choice: ChoiceSkipAll} }
	p = start(t, q, Request{Kind: Copy, Sources: srcs, Target: t.TempDir(), OnError: ErrorsAsk})
	if p.State != StateDone || asks != 1 || p.Skipped != 3 {
		t.Fatalf("skip all: asks %d, %+v", asks, p)
	}
}

func TestTryRetriesAutomatically(t *testing.T) {
	q := NewQueue(nil)
	id, _ := q.Start(Request{Kind: Delete, Sources: []string{filepath.Join(t.TempDir(), "missing")}, Retries: 2})
	q.Wait(id)
	j := q.get(id)
	calls := 0
	err, _ := j.try("x", "", "", nil, func() error {
		calls++
		if calls < 3 {
			return errors.New("busy")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err %v after %d calls", err, calls)
	}
}

// The summary counts only bytes really written (#124): skipped and failed files still fill the bar, but a stopped
// job reports what it copied, both in the summary and in BytesDone.
func TestSummaryLeavesOutSkippedAndFailedBytes(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "a"), "new a")
	write(t, filepath.Join(src, "b"), "new b")
	write(t, filepath.Join(dst, "a"), "old")
	write(t, filepath.Join(dst, "b"), "old")
	q := NewQueue(nil)
	srcs := []string{filepath.Join(src, "a"), filepath.Join(src, "b")}
	p := start(t, q, Request{Kind: Copy, Sources: srcs, Target: dst, Overwrite: Skip})
	if p.Skipped != 2 || p.BytesDone != p.Bytes || !strings.HasPrefix(p.Summary, "Copied 0 of 2 files (0 bytes)") {
		t.Fatalf("conflict skip: %+v", p)
	}

	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		ro := t.TempDir()
		os.Chmod(ro, 0o555)
		defer os.Chmod(ro, 0o755)
		q.AskFunc = func(p Progress) Answer { return Answer{Choice: ChoiceSkipAll} }
		p = start(t, q, Request{Kind: Copy, Sources: srcs, Target: ro, OnError: ErrorsAsk})
		if p.Skipped != 2 || !strings.HasPrefix(p.Summary, "Copied 0 of 2 files (0 bytes)") {
			t.Fatalf("read-only target: %+v", p)
		}
	}

	big := filepath.Join(src, "big")
	if err := os.WriteFile(big, make([]byte, 4<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := q.Start(Request{Kind: Copy, Sources: []string{big}, Target: t.TempDir(), SpeedLimit: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	q.Cancel(id)
	p = q.Wait(id)
	if p.State != StateCancelled || p.BytesDone != 0 || !strings.HasPrefix(p.Summary, "Copied 0 of 1 files (0 bytes)") {
		t.Fatalf("cancelled: %+v", p)
	}
}
