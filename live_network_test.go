//go:build live

package main

// Live tests of the network neighbourhood (board issue #148) against the Samba server and the NFS export that
// scripts/live_servers.sh starts: the shares are listed through the Network service as the page calls it, a wrong
// password and a guest are refused and asked again, the NFS export is listed by showmount.

import (
	"context"
	"os"
	"slices"
	"testing"

	"mormicommander/internal/keychain"
	"mormicommander/internal/netbrowse"
)

func TestLiveNetworkSMBShares(t *testing.T) {
	host, user, pass, share := os.Getenv("MC_LIVE_SMB_HOST"), os.Getenv("MC_LIVE_SMB_USER"), os.Getenv("MC_LIVE_SMB_PASS"), os.Getenv("MC_LIVE_SMB_SHARE")
	if host == "" {
		liveMissing(t, "smb", "MC_LIVE_SMB_HOST is not set")
	}
	n := newNetwork(&App{})
	dir := t.TempDir()
	kc := keychain.NewFake()
	n.dir = func() string { return dir }
	n.openKeychain = func() (keychain.Keychain, error) { return kc, nil }
	ctx := context.Background()

	// a guest is refused: the page is told to ask
	r, err := n.Shares(ctx, NetRequest{Address: "smb://" + host})
	if err != nil || !r.NeedAuth {
		t.Fatalf("as guest: %+v %v, want NeedAuth", r, err)
	}
	// a wrong password too
	r, err = n.Shares(ctx, NetRequest{Address: "smb://" + host, User: user, Password: "wrong"})
	if err != nil || !r.NeedAuth {
		t.Fatalf("wrong password: %+v %v, want NeedAuth", r, err)
	}
	// the right one lists the share, without the hidden IPC$
	r, err = n.Shares(ctx, NetRequest{Address: `\\` + host, User: user, Password: pass, Save: true})
	if err != nil || r.NeedAuth {
		t.Fatalf("with the password: %+v %v", r, err)
	}
	var names []string
	for _, s := range r.Shares {
		names = append(names, s.Name)
	}
	if !slices.Contains(names, share) || slices.Contains(names, "IPC$") {
		t.Fatalf("shares %v, want %s and no IPC$", names, share)
	}
	// the saved password is used the next time
	if r, err := n.Shares(ctx, NetRequest{Address: "smb://" + host, User: user}); err != nil || r.NeedAuth {
		t.Fatalf("with the saved password: %+v %v", r, err)
	}
}

func TestLiveNetworkNFSExports(t *testing.T) {
	host, export := os.Getenv("MC_LIVE_NFS_HOST"), os.Getenv("MC_LIVE_NFS_EXPORT")
	if host == "" {
		liveMissing(t, "nfs", "MC_LIVE_NFS_HOST is not set")
	}
	list, err := netbrowse.Shares(context.Background(), netbrowse.Target{Kind: "nfs", Host: host}, netbrowse.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(list, func(s netbrowse.Share) bool { return s.Name == export }) {
		t.Fatalf("exports %+v, want %s", list, export)
	}
}
