// Tests for the background Job: results stream through the channel, Stop ends a
// search early, errors and progress are kept, and Wait never deadlocks.

package find

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestJobStreams(t *testing.T) {
	root := mkTree(t, map[string]string{"a.txt": "", "b.txt": "", "c/d.txt": ""})
	o := DefaultOptions()
	o.SearchIn = []string{root, filepath.Join(root, "missing")}
	o.SearchFor = "*.txt"
	j, err := Start(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for r := range j.Results() {
		got = append(got, r.Rel)
	}
	st, err := j.Wait()
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "job results", got, []string{"a.txt", "b.txt", "c/d.txt"})
	if st.Matches != 3 || j.Progress().Matches != 3 {
		t.Errorf("stats %+v progress %+v", st, j.Progress())
	}
	if e := j.Errors(); len(e) != 1 || e[0].Path != filepath.Join(root, "missing") {
		t.Errorf("errors %+v", e)
	}
	select {
	case <-j.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done not closed")
	}
}

func TestJobStop(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 600; i++ {
		files[fmt.Sprintf("d%02d/f%03d.txt", i%20, i)] = "x"
	}
	root := mkTree(t, files)
	o := DefaultOptions()
	o.SearchIn = []string{root}
	j, err := Start(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	<-j.Results() // at least one result arrived
	j.Stop()
	j.Stop() // twice is fine
	_, err = j.Wait()
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Wait after Stop: %v", err)
	}
	// The channel ends.
	for range j.Results() {
	}
}

func TestJobWaitWithoutReading(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 400; i++ { // more than the channel buffer
		files[fmt.Sprintf("f%03d", i)] = ""
	}
	root := mkTree(t, files)
	o := DefaultOptions()
	o.SearchIn = []string{root}
	j, err := Start(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		st, err := j.Wait()
		if err != nil || st.Matches != 400 {
			t.Errorf("Wait: %+v %v", st, err)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Wait deadlocked")
	}
}

func TestJobCompileError(t *testing.T) {
	if _, err := Start(context.Background(), Options{}); err == nil {
		t.Error("want error")
	}
}
