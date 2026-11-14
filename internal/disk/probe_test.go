// Tests of the time limit on volume questions: a mount that never answers (a channel that is never closed, standing in
// for statfs on a share whose server is gone) must not hold up the others.
package disk

import (
	"errors"
	"os"
	"testing"
	"time"

	"mormicommander/internal/hang"
)

func TestProbeEachDoesNotWaitForDeadMount(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	start := time.Now()
	vals, errs := probeEach([]string{"/ok", "/dead-share-for-test", "/ok2"}, func(k string) (string, error) {
		if k == "/dead-share-for-test" {
			<-block
		}
		return "answer " + k, nil
	})
	if took := time.Since(start); took > probeTimeout+time.Second {
		t.Fatalf("probeEach took %v", took)
	}
	if vals[0] != "answer /ok" || vals[2] != "answer /ok2" || errs[0] != nil || errs[2] != nil {
		t.Fatalf("live mounts: %q %v", vals, errs)
	}
	if !errors.Is(errs[1], hang.ErrNotResponding) {
		t.Fatalf("dead mount: %v", errs[1])
	}
}

func TestUsageOfQuick(t *testing.T) {
	u, err := UsageOfQuick(os.TempDir())
	if err != nil || u.Total == 0 {
		t.Fatalf("UsageOfQuick = %+v, %v", u, err)
	}
}
