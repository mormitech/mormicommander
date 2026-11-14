package fileops

// Tests for the Trash job's route around Finder (issue #36): when the Finder call fails, the error question offers
// "without Finder" for one item or for the rest of the job, and a cancelled job stops a trash call that hangs. The
// real trash is never touched: trashMove, trashDirect and trashDirectOK are replaced.

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// fakeTrash replaces the trash hooks for one test; finder is the Finder route, direct records the direct route.
func fakeTrash(t *testing.T, finder func(context.Context, string) error) *[]string {
	t.Helper()
	var direct []string
	om, od, ok := trashMove, trashDirect, trashDirectOK
	trashMove = finder
	trashDirect = func(_ context.Context, p string) error { direct = append(direct, filepath.Base(p)); return nil }
	trashDirectOK = func() bool { return true }
	t.Cleanup(func() { trashMove, trashDirect, trashDirectOK = om, od, ok })
	return &direct
}

func TestTrashDirectOne(t *testing.T) {
	finderCalls := 0
	direct := fakeTrash(t, func(context.Context, string) error { finderCalls++; return errors.New("Finder did not answer") })
	nowhere := filepath.Join(t.TempDir(), "nowhere")
	q := NewQueue(nil)
	asks := 0
	q.AskFunc = func(p Progress) Answer {
		asks++
		if !hasChoice(p.Ask.Choices, ChoiceTrashDirect) || !hasChoice(p.Ask.Choices, ChoiceTrashDirectAll) || !hasChoice(p.Ask.Choices, ChoiceDelete) {
			t.Errorf("choices %v", p.Ask.Choices)
		}
		return Answer{Choice: ChoiceTrashDirect}
	}
	p := start(t, q, Request{Kind: Trash, Sources: []string{filepath.Join(nowhere, "a"), filepath.Join(nowhere, "b")}, OnError: ErrorsAsk})
	if p.State != StateDone || p.FilesDone != 2 || asks != 2 || finderCalls != 2 || len(*direct) != 2 {
		t.Fatalf("%+v asks=%d finder=%d direct=%v", p, asks, finderCalls, *direct)
	}
}

func TestTrashDirectAll(t *testing.T) {
	finderCalls := 0
	direct := fakeTrash(t, func(context.Context, string) error { finderCalls++; return errors.New("not allowed") })
	nowhere := filepath.Join(t.TempDir(), "nowhere")
	q := NewQueue(nil)
	asks := 0
	q.AskFunc = func(Progress) Answer { asks++; return Answer{Choice: ChoiceTrashDirectAll} }
	srcs := []string{filepath.Join(nowhere, "a"), filepath.Join(nowhere, "b"), filepath.Join(nowhere, "c")}
	p := start(t, q, Request{Kind: Trash, Sources: srcs, OnError: ErrorsAsk})
	if p.State != StateDone || p.FilesDone != 3 || asks != 1 || finderCalls != 1 || len(*direct) != 3 {
		t.Fatalf("%+v asks=%d finder=%d direct=%v", p, asks, finderCalls, *direct)
	}
}

func TestTrashNoDirectRouteOffered(t *testing.T) {
	fakeTrash(t, func(context.Context, string) error { return errors.New("no trash here") })
	trashDirectOK = func() bool { return false }
	q := NewQueue(nil)
	var got Conflict
	q.AskFunc = func(p Progress) Answer { got = *p.Ask; return Answer{Choice: ChoiceSkip} }
	start(t, q, Request{Kind: Trash, Sources: []string{filepath.Join(t.TempDir(), "x")}, OnError: ErrorsAsk})
	if hasChoice(got.Choices, ChoiceTrashDirect) || !hasChoice(got.Choices, ChoiceDelete) {
		t.Errorf("without a direct route only the permanent delete is offered: %v", got.Choices)
	}
}

func TestTrashCancelStopsHangingCall(t *testing.T) {
	entered := make(chan struct{}, 1)
	fakeTrash(t, func(ctx context.Context, _ string) error {
		entered <- struct{}{}
		<-ctx.Done() // a Finder call that never answers until it is stopped
		return ctx.Err()
	})
	q := NewQueue(nil)
	id, err := q.Start(Request{Kind: Trash, Sources: []string{filepath.Join(t.TempDir(), "x")}, OnError: ErrorsAsk})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the trash call was never made")
	}
	q.Cancel(id)
	deadline := time.Now().Add(5 * time.Second)
	for {
		p, _ := q.Get(id)
		if p.State == StateCancelled {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the job did not stop: %+v", p)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
