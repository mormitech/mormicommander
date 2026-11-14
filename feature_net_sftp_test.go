package main

// Tests for SFTP through the connection layer (board issue #10: F-632 and F-636). A small SSH server runs inside the
// test on 127.0.0.1 and serves the SFTP subsystem over a temp folder; it takes a plain password and public keys, and
// leaves keyboard-interactive out unless asked, so the "password only" servers that used to fail from the path bar are
// covered. Every test points HOME, the settings folder and SSH_AUTH_SOCK at temp places, so the user's real keys,
// agent and known_hosts are never read or written.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	pkgsftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"mormicommander/internal/keychain"
)

const netSSHUser, netSSHPass = "tester", "s3cret"

// netSSHServer is the test server: Root is the served folder, URL the sftp:// address of that folder.
type netSSHServer struct {
	Root, URL string
	HostKey   ssh.Signer
	mu        sync.Mutex
	logins    int
}

// newNetSSHServer starts the server. authKeys are public keys allowed to log in; kbd also offers keyboard-interactive.
func newNetSSHServer(t *testing.T, kbd bool, authKeys ...ssh.PublicKey) *netSSHServer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hk, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	s := &netSSHServer{Root: t.TempDir(), HostKey: hk}
	ok := func() (*ssh.Permissions, error) {
		s.mu.Lock()
		s.logins++
		s.mu.Unlock()
		return nil, nil
	}
	conf := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if c.User() == netSSHUser && string(pw) == netSSHPass {
				return ok()
			}
			return nil, errors.New("denied")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			for _, k := range authKeys {
				if string(k.Marshal()) == string(key.Marshal()) && c.User() == netSSHUser {
					return ok()
				}
			}
			return nil, errors.New("unknown key")
		},
	}
	if kbd {
		conf.KeyboardInteractiveCallback = func(c ssh.ConnMetadata, client ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			ans, err := client(c.User(), "login", []string{"Password: "}, []bool{false})
			if err != nil || len(ans) != 1 || ans[0] != netSSHPass || c.User() != netSSHUser {
				return nil, errors.New("denied")
			}
			return ok()
		}
	}
	conf.AddHostKey(hk)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var conns []net.Conn
	var cmu sync.Mutex
	t.Cleanup(func() {
		ln.Close()
		cmu.Lock()
		for _, c := range conns {
			c.Close()
		}
		cmu.Unlock()
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			cmu.Lock()
			conns = append(conns, c)
			cmu.Unlock()
			go netSSHHandle(c, conf, s.Root)
		}
	}()
	root := filepath.ToSlash(s.Root)
	if runtime.GOOS == "windows" {
		root = "/" + root
	}
	s.URL = "sftp://" + netSSHUser + "@127.0.0.1:" + strconv.Itoa(ln.Addr().(*net.TCPAddr).Port) + root
	return s
}

func (s *netSSHServer) loginCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logins
}

// netSSHHandle serves one connection: session channels with the "sftp" subsystem, nothing else.
func netSSHHandle(c net.Conn, conf *ssh.ServerConfig, root string) {
	sc, chans, reqs, err := ssh.NewServerConn(c, conf)
	if err != nil {
		c.Close()
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, creqs, err := nc.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer ch.Close()
			for r := range creqs {
				var p struct{ Name string }
				if r.Type == "subsystem" && ssh.Unmarshal(r.Payload, &p) == nil && p.Name == "sftp" {
					r.Reply(true, nil)
					srv, err := pkgsftp.NewServer(ch, pkgsftp.WithServerWorkingDirectory(root))
					if err != nil {
						return
					}
					srv.Serve()
					srv.Close()
					return
				}
				if r.WantReply {
					r.Reply(false, nil)
				}
			}
		}()
	}
}

// netSSHIsolate points HOME, the agent socket and the settings folder at temp places and returns the settings folder.
func netSSHIsolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	dir := t.TempDir()
	t.Setenv("MORMICOMMANDER_CONFIG_DIR", dir)
	return dir
}

// netSSHNet is a Net whose questions are answered by answer (kind, message) → (value, ok); it records the kinds asked.
func netSSHNet(t *testing.T, answer func(kind, message string) (string, bool)) (*Net, *[]string) {
	t.Helper()
	n := newNet(&App{})
	n.openKeychain = func() (keychain.Keychain, error) { return keychain.NewFake(), nil } // never the real keychain
	var mu sync.Mutex
	var asked []string
	n.asker = func(kind string, data map[string]any) (map[string]any, bool) {
		mu.Lock()
		asked = append(asked, kind)
		mu.Unlock()
		msg, _ := data["message"].(string)
		v, ok := answer(kind, msg)
		if !ok {
			return nil, false
		}
		return map[string]any{"ok": true, "value": v, "choice": v}, true
	}
	t.Cleanup(func() {
		// the vfs router is shared by the whole test binary: drop the test servers' sessions so other tests see none
		for _, ss := range n.Sessions() {
			if strings.HasPrefix(ss.URL, "sftp://"+netSSHUser+"@127.0.0.1:") {
				n.disconnectSession(ss.URL, true)
			}
		}
		n.close()
	})
	return n, &asked
}

// netListHas lists p the way a panel does when a URL is typed in its path bar (FS.List → vfs router) and checks that
// name is in it ("" only checks that the folder could be read).
func netListHas(t *testing.T, n *Net, p, name string) {
	t.Helper()
	l, err := n.router().List(context.Background(), p)
	if err != nil {
		t.Fatalf("listing %s: %v", p, err)
	}
	if name == "" {
		return
	}
	for _, e := range l.Entries {
		if e.Name == name {
			return
		}
	}
	t.Fatalf("%s not in the listing of %s", name, p)
}

// Typing an sftp:// URL without a password in the path bar of a server that only takes a plain password: the server's
// key is shown first (and saved in the app's own known_hosts), then the password is asked, and the folder is shown.
// The password is remembered for the session; a new run trusts the saved key and only asks for the password.
func TestNetSFTPPathBarAsksHostKeyThenPassword(t *testing.T) {
	cfg := netSSHIsolate(t)
	s := newNetSSHServer(t, false)
	os.WriteFile(filepath.Join(s.Root, "hello.txt"), []byte("hi"), 0o644)
	answers := map[string]string{"hostkey": "save", "password": netSSHPass}
	n, asked := netSSHNet(t, func(kind, _ string) (string, bool) { v, ok := answers[kind]; return v, ok })

	netListHas(t, n, s.URL, "hello.txt")
	if strings.Join(*asked, ",") != "hostkey,password" {
		t.Fatalf("asked %v, want the host key then the password", *asked)
	}
	kh, err := os.ReadFile(filepath.Join(cfg, "known_hosts"))
	if err != nil || !strings.Contains(string(kh), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(s.HostKey.PublicKey())))) {
		t.Fatalf("host key not saved in the app's known_hosts: %v %q", err, kh)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")); err == nil {
		t.Fatal("the user's ~/.ssh/known_hosts was written")
	}

	// a reconnect of the same session uses the remembered password and the saved key
	n.disconnectSession(s.URL, false)
	netListHas(t, n, s.URL, "hello.txt")
	if len(*asked) != 2 {
		t.Fatalf("reconnect asked again: %v", *asked)
	}

	// a new run (no open session, nothing remembered): the key is known, only the password is asked
	n.disconnectSession(s.URL, true)
	n2, asked2 := netSSHNet(t, func(kind, _ string) (string, bool) { v, ok := answers[kind]; return v, ok })
	netListHas(t, n2, s.URL, "hello.txt")
	if strings.Join(*asked2, ",") != "password" {
		t.Fatalf("new run asked %v, want only the password", *asked2)
	}
}

// A mistyped password is asked again, with the "did not accept" text only after the refusal.
func TestNetSFTPPathBarWrongPasswordAsksAgain(t *testing.T) {
	netSSHIsolate(t)
	s := newNetSSHServer(t, false)
	typed := []string{"wrong", netSSHPass}
	var messages []string
	n, _ := netSSHNet(t, func(kind, msg string) (string, bool) {
		switch kind {
		case "hostkey":
			return "once", true
		case "password":
			messages = append(messages, msg)
			v := typed[0]
			typed = typed[1:]
			return v, true
		}
		return "", false
	})
	netListHas(t, n, s.URL, "")
	if len(messages) != 2 || messages[0] != "" || !strings.Contains(messages[1], "did not accept") {
		t.Fatalf("password messages %q", messages)
	}
}

// Cancelling the password question stops the connection with the plain "cancelled" error, and nothing is remembered.
func TestNetSFTPPathBarPasswordCancelled(t *testing.T) {
	netSSHIsolate(t)
	s := newNetSSHServer(t, false)
	n, _ := netSSHNet(t, func(kind, _ string) (string, bool) { return "once", kind == "hostkey" })
	if _, err := n.router().List(context.Background(), s.URL); !errors.Is(err, errNetCancelled) {
		t.Fatalf("got %v, want the cancelled error", err)
	}
	if len(n.passwords) != 0 {
		t.Fatalf("a password was remembered: %v", n.passwords)
	}
	if s.loginCount() != 0 {
		t.Fatal("logged in without a password")
	}
}

// A key in ~/.ssh logs in without any password question (key auth), and quick connect (Net.Connect, also the
// command line's "cd sftp://...") reaches the same folder.
func TestNetSFTPKeyLoginNeedsNoPassword(t *testing.T) {
	netSSHIsolate(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "test")
	if err != nil {
		t.Fatal(err)
	}
	sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
	os.MkdirAll(sshDir, 0o700)
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	s := newNetSSHServer(t, false, sp)
	os.WriteFile(filepath.Join(s.Root, "k.txt"), nil, 0o644)
	n, asked := netSSHNet(t, func(kind, _ string) (string, bool) { return "once", kind == "hostkey" })
	r, err := n.Connect(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	netListHas(t, n, r.Path, "k.txt")
	if strings.Join(*asked, ",") != "hostkey" {
		t.Fatalf("asked %v, want only the host key", *asked)
	}
}

// A server that also offers keyboard-interactive keeps asking through that question, as before: the new password
// question is only for servers that take nothing but a plain password.
func TestNetSFTPKeyboardInteractiveUnchanged(t *testing.T) {
	netSSHIsolate(t)
	s := newNetSSHServer(t, true)
	n, asked := netSSHNet(t, func(kind, _ string) (string, bool) {
		switch kind {
		case "hostkey":
			return "once", true
		case "interactive":
			return netSSHPass, true
		}
		return "", false
	})
	netListHas(t, n, s.URL, "")
	if strings.Join(*asked, ",") != "hostkey,interactive" {
		t.Fatalf("asked %v", *asked)
	}
}
