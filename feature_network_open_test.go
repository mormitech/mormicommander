package main

// Opening servers and shares in the background (feature_network_open.go): a listing that is cancelled (the user went
// elsewhere) doesn't stop the open, a later listing of the same folder waits for the same open instead of starting a
// second one, and a server's list of shares is kept for a minute.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"mormicommander/internal/netbrowse"
)

func TestNetworkOpenGoesOnWhenTheListingStops(t *testing.T) {
	n, _, _ := fakeNetwork(t)
	release := make(chan struct{})
	var mounts atomic.Int32
	n.mount = func(ctx context.Context, tg netbrowse.Target, _ netbrowse.Creds, _ netbrowse.MountOptions) (netbrowse.Mounted, error) {
		mounts.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
			return netbrowse.Mounted{}, ctx.Err()
		}
		return netbrowse.Mounted{Path: "/Volumes/" + tg.Share, Made: true}, nil
	}
	tg := netbrowse.Target{Kind: "smb", Host: "nas.local", Share: "media"}
	key := "network://NAS/media"

	// the panel goes elsewhere: its wait ends, the mount doesn't
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	if _, err := n.mountOf(ctx, key, tg, netbrowse.Creds{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("a stopped listing: %v, want context.Canceled", err)
	}
	if got := n.openingNow(); len(got) != 1 || got[0] != key {
		t.Fatalf("opening now %v, want [%s]", got, key)
	}
	if s := n.Status(); len(s.Opening) != 1 {
		t.Fatalf("Status().Opening = %v", s.Opening)
	}

	// coming back while it is still mounting waits for the same mount
	done := make(chan netbrowse.Mounted)
	go func() {
		m, _ := n.mountOf(context.Background(), key, tg, netbrowse.Creds{})
		done <- m
	}()
	time.Sleep(50 * time.Millisecond)
	close(release)
	select {
	case m := <-done:
		if m.Path != "/Volumes/media" {
			t.Fatalf("mounted %+v", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second listing never got the mount")
	}
	if mounts.Load() != 1 {
		t.Errorf("mounted %d times, want once", mounts.Load())
	}
	for i := 0; i < 100 && len(n.openingNow()) > 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if got := n.openingNow(); len(got) != 0 {
		t.Errorf("still opening %v after the mount", got)
	}
	n.mu.Lock()
	temps := append([]string(nil), n.temps...)
	n.mu.Unlock()
	if len(temps) != 1 || temps[0] != "/Volumes/media" {
		t.Errorf("temps %v: a share mounted in the background must be disconnected at the end too", temps)
	}
}

func TestNetworkSharesAreCachedForAMinute(t *testing.T) {
	n, _, _ := fakeNetwork(t)
	var calls atomic.Int32
	n.shares = func(context.Context, netbrowse.Target, netbrowse.Creds) ([]netbrowse.Share, error) {
		calls.Add(1)
		return []netbrowse.Share{{Name: "media"}}, nil
	}
	tg := netbrowse.Target{Kind: "smb", Host: "nas.local"}
	for i := 0; i < 3; i++ {
		if l, err := n.sharesOf(context.Background(), "network://NAS", tg, netbrowse.Creds{User: "pat"}); err != nil || len(l) != 1 {
			t.Fatalf("shares: %v %v", l, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("asked the server %d times, want once", calls.Load())
	}
	// another user is another list; an old entry is read again
	n.sharesOf(context.Background(), "network://NAS", tg, netbrowse.Creds{User: "kim"})
	o := n.opens()
	o.mu.Lock()
	for k, e := range o.shares {
		e.at = time.Now().Add(-2 * netShareCacheTTL)
		o.shares[k] = e
	}
	o.mu.Unlock()
	n.sharesOf(context.Background(), "network://NAS", tg, netbrowse.Creds{User: "pat"})
	if calls.Load() != 3 {
		t.Fatalf("asked the server %d times, want 3", calls.Load())
	}
	// a failure is not kept
	n.shares = func(context.Context, netbrowse.Target, netbrowse.Creds) ([]netbrowse.Share, error) {
		return nil, netbrowse.ErrAuth
	}
	if _, err := n.sharesOf(context.Background(), "network://NAS", netbrowse.Target{Kind: "smb", Host: "other.local"}, netbrowse.Creds{}); !errors.Is(err, netbrowse.ErrAuth) {
		t.Fatalf("a failing server: %v", err)
	}
	o.mu.Lock()
	_, kept := o.shares["smb\x00other.local\x00"]
	o.mu.Unlock()
	if kept {
		t.Error("a failed list was cached")
	}
}
