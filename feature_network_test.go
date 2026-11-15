package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/keychain"
	"mormicommander/internal/netbrowse"
	"mormicommander/internal/vfs"
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

// The network folder (#148): the background search fills the root as servers answer (an AFP twin of an SMB server is
// left out), a server lists its shares, a share is mounted and shown with ".." back to the shares, and a server that
// wants a password makes the page ask.
func TestNetworkFolder(t *testing.T) {
	n, _, _ := fakeNetwork(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "movie.mkv"), []byte("x"), 0o644)
	n.app.fs = vfs.Local{}
	emitted := make(chan func(netbrowse.Event), 1)
	n.watch = func(ctx context.Context, emit func(netbrowse.Event)) string {
		emitted <- emit
		<-ctx.Done()
		return ""
	}
	n.mount = func(_ context.Context, tg netbrowse.Target, c netbrowse.Creds, _ netbrowse.MountOptions) (netbrowse.Mounted, error) {
		if c.Password != "pw" {
			return netbrowse.Mounted{}, netbrowse.ErrAuth
		}
		return netbrowse.Mounted{Path: dir, Made: true}, nil
	}
	defer n.shutdown()
	ctx := context.Background()
	l, err := n.listNetwork(ctx, "network://")
	if err != nil || len(l.Entries) != 0 {
		t.Fatalf("empty root: %+v %v", l, err)
	}
	emit := <-emitted
	emit(netbrowse.Event{Server: netbrowse.Server{Name: "NAS", Host: "nas.local", Kind: "smb"}})
	emit(netbrowse.Event{Server: netbrowse.Server{Name: "NAS", Host: "nas.local", Kind: "afp"}})
	emit(netbrowse.Event{Server: netbrowse.Server{Name: "Gone", Host: "gone.local", Kind: "smb"}})
	emit(netbrowse.Event{Gone: true, Server: netbrowse.Server{Name: "Gone", Kind: "smb"}})
	l, _ = n.listNetwork(ctx, "network://")
	if len(l.Entries) != 1 || l.Entries[0].Name != "NAS" || l.Entries[0].Flags&vfs.FlagDir == 0 || l.Entries[0].Flags&vfs.FlagNoAttr == 0 {
		t.Fatalf("root: %+v", l.Entries)
	}
	if s := n.Status(); !s.Searching || s.Count != 2 {
		t.Fatalf("status %+v", s)
	}
	// the shares want a password: the page is asked
	if _, err := n.listNetwork(ctx, "network://NAS"); !errors.As(err, new(errNetAuth)) {
		t.Fatalf("shares without a password: %v", err)
	}
	if err := n.SetCreds(NetRequest{Address: "smb://nas.local", User: "pat", Password: "pw"}); err != nil {
		t.Fatal(err)
	}
	l, err = n.listNetwork(ctx, "network://NAS/")
	if err != nil || len(l.Entries) != 1 || l.Entries[0].Name != "media" || l.Parent != "network://" || l.Entries[0].Flags&vfs.FlagNoAttr == 0 {
		t.Fatalf("shares: %+v %v", l, err)
	}
	l, err = n.listNetwork(ctx, "network://NAS/media")
	if err != nil || l.Path != dir || l.Parent != "network://NAS" || len(l.Entries) != 1 {
		t.Fatalf("share: %+v %v", l, err)
	}
	// typed in the path bar
	if l, err := n.listNetwork(ctx, "smb://nas.local"); err != nil || len(l.Entries) != 1 {
		t.Fatalf("smb://nas.local: %+v %v", l, err)
	}
	fsys := networkFS{}
	for in, want := range map[string]string{"network://NAS/media": "network://NAS", "network://NAS": "network://", "network://": "", "smb://nas": "network://", "smb://nas/media": "smb://nas"} {
		if got := fsys.Parent(in); got != want {
			t.Errorf("Parent(%q) = %q, want %q", in, got, want)
		}
	}
	if got := fsys.Join("network://", "NAS"); got != "network://NAS" {
		t.Errorf("Join = %q", got)
	}
}

// A server name used before the search has found it (a button bar "cd network://NAS", a bookmark, a typed path right
// after the start) waits for the search instead of failing at once; a name nobody answers to still fails.
func TestNetworkServerBeforeSearch(t *testing.T) {
	n, _, _ := fakeNetwork(t)
	n.watch = func(ctx context.Context, emit func(netbrowse.Event)) string {
		time.Sleep(300 * time.Millisecond)
		emit(netbrowse.Event{Server: netbrowse.Server{Name: "NAS", Host: "nas.local", Kind: "smb"}})
		<-ctx.Done()
		return ""
	}
	defer n.shutdown()
	old := serverWait
	serverWait = 3 * time.Second
	defer func() { serverWait = old }()
	ctx := context.Background()
	if _, err := n.listNetwork(ctx, "network://NAS/"); errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a server found a moment later: %v", err)
	}
	serverWait = 500 * time.Millisecond
	if _, err := n.listNetwork(ctx, "network://Nobody"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a server nobody answers to: %v", err)
	}
}
