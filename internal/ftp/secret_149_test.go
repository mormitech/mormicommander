package ftp

import (
	"testing"
	"time"
)

// A tampered settings file with a huge key-derivation count is refused at once instead of hanging the app (#149).
func TestOpenMasterKeyBoundsIterations(t *testing.T) {
	_, h, err := NewMasterKey("pw", 1000)
	if err != nil {
		t.Fatal(err)
	}
	h.Iterations = 2_000_000_000
	start := time.Now()
	if _, err := OpenMasterKey("pw", h); err == nil {
		t.Fatal("a 2-billion iteration header was accepted")
	}
	if time.Since(start) > time.Second {
		t.Fatal("the refusal took too long")
	}
}
