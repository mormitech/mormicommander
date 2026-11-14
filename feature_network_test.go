package main

import (
	"context"
	"strings"
	"testing"

	"mormicommander/internal/keychain"
	"mormicommander/internal/netbrowse"
)

// fakeNetwork is a Network with a fake network: one server "nas" whose shares need user "pat" with password "pw".
func fakeNetwork(t *testing.T) (*Network, *keychain.Fake, *[]string) {
	dir := t.TempDir()
	kc := keychain.NewFake()
	var unmounted []string
	n := newNetwork(&App{})
	n.dir = func() string { return dir }
	n.openKeychain = func() (keychain.Keychain, error) { return kc, nil }
	n.discover = func(context.Context) ([]netbrowse.Server, string) {
		return []netbrowse.Server{{Name: "NAS", Host: "nas.local", Kind: "smb", Source: "bonjour"}}, ""
	}
	n.shares = func(_ context.Context, tg netbrowse.Target, c netbrowse.Creds) ([]netbrowse.Share, error) {
		if c.User != "pat" || c.Password != "pw" {
			return nil, netbrowse.ErrAuth
		}
		return []netbrowse.Share{{Name: "media"}}, nil
	}
	n.mount = func(_ context.Context, tg netbrowse.Target, c netbrowse.Creds, o netbrowse.MountOptions) (netbrowse.Mounted, error) {
		if c.Password != "pw" {
			return netbrowse.Mounted{}, netbrowse.ErrAuth
		}
		return netbrowse.Mounted{Path: "/Volumes/" + tg.Share, Made: true}, nil
	}
	n.unmount = func(_ context.Context, p string) error { unmounted = append(unmounted, p); return nil }
	return n, kc, &unmounted
}

func TestNetworkSharesAskThenRemember(t *testing.T) {
	n, kc, _ := fakeNetwork(t)
	ctx := context.Background()
	r, err := n.Shares(ctx, NetRequest{Address: "smb://nas.local"})
	if err != nil || !r.NeedAuth {
		t.Fatalf("without a password: %+v %v, want NeedAuth", r, err)
	}
	r, err = n.Shares(ctx, NetRequest{Address: "smb://nas.local", User: "pat", Password: "pw", Save: true})
	if err != nil || r.NeedAuth || len(r.Shares) != 1 {
		t.Fatalf("with the password: %+v %v", r, err)
	}
	if pw, _ := kc.Get(netKeyName(netbrowse.Target{Kind: "smb", Host: "nas.local", User: "pat"})); pw != "pw" {
		t.Fatalf("saved password %q", pw)
	}
	// the next time the saved password is used, and the server comes first with its user
	r, err = n.Shares(ctx, NetRequest{Address: "smb://nas.local", User: "pat"})
	if err != nil || r.NeedAuth {
		t.Fatalf("with the saved password: %+v %v", r, err)
	}
	d := n.Discover(ctx)
	if len(d.Servers) != 1 || d.Servers[0].User != "pat" || !d.Servers[0].Saved || d.Servers[0].Address != "smb://nas.local" {
		t.Fatalf("discover: %+v", d.Servers)
	}
	if err := n.Forget("smb://nas.local"); err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Get(netKeyName(netbrowse.Target{Kind: "smb", Host: "nas.local", User: "pat"})); err == nil {
		t.Fatal("Forget kept the saved password")
	}
	if d := n.Discover(ctx); d.Servers[0].User != "" {
		t.Fatalf("Forget kept the server: %+v", d.Servers)
	}
}

func TestNetKeyNameIsValid(t *testing.T) {
	name := netKeyName(netbrowse.Target{Kind: "smb", Host: "NAS.local", Port: 4455, User: `DOM\pat`})
	if err := keychain.ValidName(name); err != nil || !strings.HasPrefix(name, "net.smb.") {
		t.Fatalf("%q: %v", name, err)
	}
	if name == netKeyName(netbrowse.Target{Kind: "smb", Host: "nas.local", User: "pat"}) {
		t.Fatal("two servers share a keychain name")
	}
}

func TestNetworkPasswordNotSavedUnlessAsked(t *testing.T) {
	n, kc, _ := fakeNetwork(t)
	if _, err := n.Shares(context.Background(), NetRequest{Address: "nas.local", User: "pat", Password: "pw"}); err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Get(netKeyName(netbrowse.Target{Kind: "smb", Host: "nas.local", User: "pat"})); err == nil {
		t.Fatal("a password was saved without the box ticked")
	}
}

// Open's own mounts are disconnected when the app ends; mapped shares stay.
func TestNetworkOpenMountsAreTemporary(t *testing.T) {
	n, _, unmounted := fakeNetwork(t)
	ctx := context.Background()
	if m, err := n.Open(ctx, NetRequest{Address: "smb://nas.local", Share: "media"}); err != nil || !m.NeedAuth {
		t.Fatalf("open without a password: %+v %v", m, err)
	}
	m, err := n.Open(ctx, NetRequest{Address: "smb://nas.local", Share: "media", User: "pat", Password: "pw"})
	if err != nil || m.Path != "/Volumes/media" {
		t.Fatalf("open: %+v %v", m, err)
	}
	if _, err := n.Map(ctx, NetRequest{Address: "smb://nas.local/kept", User: "pat", Password: "pw"}); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Open(ctx, NetRequest{Address: "smb://nas.local"}); err == nil {
		t.Fatal("open without a share was accepted")
	}
	n.shutdown()
	if len(*unmounted) != 1 || (*unmounted)[0] != "/Volumes/media" {
		t.Fatalf("disconnected at the end: %v, want only the opened share", *unmounted)
	}
}
