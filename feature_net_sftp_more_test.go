package main

// More SFTP tests through the connection layer (board issue #10, F-632 and F-636), on the in-process SSH server of
// feature_net_sftp_test.go and isolated the same way (HOME, the settings folder and SSH_AUTH_SOCK point at temp
// places): the SSH agent, the user's own known_hosts, a changed server key, a password typed in the URL, an encrypted
// key of a saved connection, and saved passwords (master password and keychain) of a saved SFTP connection.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"mormicommander/internal/keychain"
)

// netSSHKey makes an ed25519 key pair.
func netSSHKey(t *testing.T) (ed25519.PrivateKey, ssh.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return priv, sp
}

// netSSHHostPort is "127.0.0.1:port" of the server.
func netSSHHostPort(s *netSSHServer) string {
	rest := strings.TrimPrefix(s.URL, "sftp://"+netSSHUser+"@")
	hp, _, _ := strings.Cut(rest, "/")
	return hp
}

// The keys of the SSH agent (SSH_AUTH_SOCK) log in without a password question; a saved connection with "Use the
// keys of the SSH agent" off does not use them and asks for the password.
func TestNetSFTPAgentLogin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test agent listens on a Unix socket")
	}
	netSSHIsolate(t)
	priv, pub := netSSHKey(t)
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: priv, Comment: "test"}); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "mcag") // Unix socket paths are short on macOS
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { agent.ServeAgent(keyring, c); c.Close() }()
		}
	}()
	t.Setenv("SSH_AUTH_SOCK", sock)
	s := newNetSSHServer(t, false, pub)
	os.WriteFile(filepath.Join(s.Root, "a.txt"), nil, 0o644)
	n, asked := netSSHNet(t, func(kind, _ string) (string, bool) {
		switch kind {
		case "hostkey":
			return "save", true
		case "password":
			return netSSHPass, true
		}
		return "", false
	})
	netListHas(t, n, s.URL, "a.txt")
	if strings.Join(*asked, ",") != "hostkey" {
		t.Fatalf("asked %v, want only the host key", *asked)
	}
	n.disconnectSession(s.URL, true)

	// a saved connection that does not use the agent asks for the password instead
	n.app.settings = loadSettings(filepath.Join(t.TempDir(), "settings.json")) // where "Use the agent" off is kept
	id, err := n.SaveSite(NetSite{Name: "NoAgent", Protocol: "sftp", Host: netSSHHostPort(s), User: netSSHUser, UseAgent: false})
	if err != nil {
		t.Fatal(err)
	}
	*asked = nil
	if _, err := n.ConnectSite(id); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*asked, ",") != "password" {
		t.Fatalf("asked %v, want the password (agent off)", *asked)
	}
}

// A server whose key is in the user's ~/.ssh/known_hosts is trusted without a question, and that file is not changed.
func TestNetSFTPUserKnownHostsTrusted(t *testing.T) {
	netSSHIsolate(t)
	s := newNetSSHServer(t, false)
	sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
	os.MkdirAll(sshDir, 0o700)
	line := knownhosts.Line([]string{knownhosts.Normalize(netSSHHostPort(s))}, s.HostKey.PublicKey()) + "\n"
	kh := filepath.Join(sshDir, "known_hosts")
	os.WriteFile(kh, []byte(line), 0o600)
	n, asked := netSSHNet(t, func(kind, _ string) (string, bool) { return netSSHPass, kind == "password" })
	netListHas(t, n, s.URL, "")
	if strings.Join(*asked, ",") != "password" {
		t.Fatalf("asked %v, want only the password", *asked)
	}
	if b, _ := os.ReadFile(kh); string(b) != line {
		t.Fatal("the user's known_hosts was changed")
	}
}

// A server key that differs from the saved one is shown with the warning; refusing it does not connect, and the
// error says why.
func TestNetSFTPChangedHostKeyRefused(t *testing.T) {
	cfg := netSSHIsolate(t)
	s := newNetSSHServer(t, false)
	_, other := netSSHKey(t)
	os.WriteFile(filepath.Join(cfg, "known_hosts"), []byte(knownhosts.Line([]string{knownhosts.Normalize(netSSHHostPort(s))}, other)+"\n"), 0o600)
	var warned bool
	n, asked := netSSHNet(t, func(kind, msg string) (string, bool) {
		if kind == "hostkey" {
			warned = strings.Contains(msg, "WARNING")
		}
		return "", false // refuse
	})
	_, err := n.Connect(s.URL)
	if err == nil || !strings.Contains(err.Error(), "was not accepted") {
		t.Fatalf("got %v, want the key refused", err)
	}
	if !warned || strings.Join(*asked, ",") != "hostkey" || s.loginCount() != 0 {
		t.Fatalf("warned %v asked %v logins %d", warned, *asked, s.loginCount())
	}
}

// A password typed in the URL (sftp://user:password@host/…) logs in without a question, and never shows in the
// panel's path (F-636).
func TestNetSFTPPasswordInURL(t *testing.T) {
	netSSHIsolate(t)
	s := newNetSSHServer(t, false)
	os.WriteFile(filepath.Join(s.Root, "p.txt"), nil, 0o644)
	withPw := strings.Replace(s.URL, netSSHUser+"@", netSSHUser+":"+netSSHPass+"@", 1)
	n, asked := netSSHNet(t, func(kind, _ string) (string, bool) { return "once", kind == "hostkey" })
	r, err := n.Connect(withPw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.Path, netSSHPass) {
		t.Fatalf("the password is in the panel path %q", r.Path)
	}
	netListHas(t, n, r.Path, "p.txt")
	if strings.Join(*asked, ",") != "hostkey" {
		t.Fatalf("asked %v", *asked)
	}
	// typed straight into the path bar, too
	n.disconnectSession(s.URL, true)
	if got := n.router().Clean(withPw); strings.Contains(got, netSSHPass) {
		t.Fatalf("the path bar keeps the password: %q", got)
	}
	netListHas(t, n, withPw, "p.txt")
	if strings.Join(*asked, ",") != "hostkey,hostkey" {
		t.Fatalf("path bar asked %v", *asked)
	}
}

// The private key file of a saved connection, protected by a passphrase, asks for the passphrase and logs in.
func TestNetSFTPSavedKeyFileWithPassphrase(t *testing.T) {
	netSSHIsolate(t)
	priv, pub := netSSHKey(t)
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "test", []byte("phrase"))
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "work_key")
	os.WriteFile(keyFile, pem.EncodeToMemory(block), 0o600)
	s := newNetSSHServer(t, false, pub)
	var titles []string
	n, _ := netSSHNet(t, func(kind, msg string) (string, bool) {
		switch kind {
		case "hostkey":
			return "once", true
		case "password":
			titles = append(titles, msg)
			return "phrase", true
		}
		return "", false
	})
	id, err := n.SaveSite(NetSite{Name: "Key", Protocol: "sftp", Host: netSSHHostPort(s), User: netSSHUser, KeyFile: keyFile, UseAgent: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.ConnectSite(id); err != nil {
		t.Fatal(err)
	}
	if len(titles) != 1 || !strings.Contains(titles[0], "passphrase") {
		t.Fatalf("asked %q, want the passphrase once", titles)
	}
}

// A saved SFTP connection logs in with its saved password, whether the master password or the keychain keeps it.
func TestNetSFTPSavedPassword(t *testing.T) {
	netSSHIsolate(t)
	s := newNetSSHServer(t, false)
	n, asked := netSSHNet(t, func(kind, _ string) (string, bool) {
		switch kind {
		case "hostkey":
			return "save", true
		case "master-new", "master":
			return "m", true
		}
		return "", false
	})
	kc := keychain.NewFake()
	n.openKeychain = func() (keychain.Keychain, error) { return kc, nil }
	id, err := n.SaveSite(NetSite{Name: "Pw", Protocol: "sftp", Host: netSSHHostPort(s), User: netSSHUser, Password: netSSHPass, SavePassword: true, UseAgent: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.ConnectSite(id); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*asked, ",") != "master-new,hostkey" {
		t.Fatalf("asked %v", *asked)
	}
	if _, err := n.SetKeychain(true); err != nil {
		t.Fatal(err)
	}
	// a new run: the keychain has the password, so neither the master password nor the password is asked
	n.disconnectSession(s.URL, true)
	n2, asked2 := netSSHNet(t, func(kind, _ string) (string, bool) { return "", false })
	n2.openKeychain = n.openKeychain
	if _, err := n2.ConnectSite(id); err != nil {
		t.Fatal(err)
	}
	if len(*asked2) != 0 {
		t.Fatalf("asked %v", *asked2)
	}
}
