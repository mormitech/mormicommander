// Tests for host key verification: first contact with and without a prompt,
// accepting once versus saving, a changed key (the man-in-the-middle case),
// a new key type for a known host, hashed known_hosts entries, revoked keys,
// and the helpers that add and remove known_hosts lines.
package sftp

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestHostKeyFirstContact(t *testing.T) {
	s := newTestServer(t)
	kh := filepath.Join(t.TempDir(), "sub", "known_hosts") // folder created on save
	base := Config{Host: "127.0.0.1", Port: s.Port, User: testUser, Password: testPassword, KnownHostsFiles: []string{kh}}

	// No prompt: refused with a HostKeyError carrying the details.
	_, err := Dial(t.Context(), base)
	var hke *HostKeyError
	if !errors.As(err, &hke) {
		t.Fatalf("err = %v, want HostKeyError", err)
	}
	if hke.Info.Status != HostKeyUnknown || hke.Info.Fingerprint != ssh.FingerprintSHA256(s.HostKey.PublicKey()) {
		t.Errorf("info = %+v", hke.Info)
	}
	if !strings.Contains(hke.Error(), "unknown") {
		t.Errorf("message %q", hke.Error())
	}

	// Prompt rejects.
	cfg := base
	var seen []HostKeyInfo
	cfg.HostKeyPrompt = func(i HostKeyInfo) HostKeyDecision { seen = append(seen, i); return HostKeyReject }
	if _, err := Dial(t.Context(), cfg); !errors.As(err, &hke) {
		t.Fatalf("rejected dial err = %v", err)
	}
	if len(seen) != 1 || seen[0].KeyType != "ssh-ed25519" || seen[0].SaveFile != kh ||
		!strings.HasPrefix(seen[0].FingerprintMD5, "") || seen[0].Host != s.Addr {
		t.Errorf("prompt saw %+v", seen)
	}

	// Accept once: connects, nothing saved, asked again next time.
	cfg.HostKeyPrompt = func(HostKeyInfo) HostKeyDecision { return HostKeyAcceptOnce }
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if _, err := os.Stat(kh); !os.IsNotExist(err) {
		t.Error("AcceptOnce wrote known_hosts")
	}

	// Accept and save: written, next dial needs no prompt.
	cfg.HostKeyPrompt = func(HostKeyInfo) HostKeyDecision { return HostKeyAcceptAndSave }
	c, err = Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	data := readFile(t, kh)
	if !strings.HasPrefix(data, "[127.0.0.1]:") {
		t.Errorf("known_hosts = %q", data)
	}
	if fi, _ := os.Stat(kh); fi.Mode().Perm()&0o077 != 0 && !isWindows() {
		t.Errorf("known_hosts mode %v", fi.Mode().Perm())
	}
	cfg.HostKeyPrompt = func(HostKeyInfo) HostKeyDecision { t.Error("prompted for a saved key"); return HostKeyReject }
	c, err = Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestHostKeyChanged(t *testing.T) {
	s := newTestServer(t)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	// Store a different ed25519 key for this address, plus an unrelated host.
	other := newSigner(t)
	if err := AddKnownHost(kh, s.Addr, other.PublicKey(), false); err != nil {
		t.Fatal(err)
	}
	if err := AddKnownHost(kh, "example.org:22", other.PublicKey(), false); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Host: "127.0.0.1", Port: s.Port, User: testUser, Password: testPassword, KnownHostsFiles: []string{kh}}

	var info HostKeyInfo
	cfg.HostKeyPrompt = func(i HostKeyInfo) HostKeyDecision { info = i; return HostKeyReject }
	if _, err := Dial(t.Context(), cfg); err == nil {
		t.Fatal("changed key accepted")
	}
	if info.Status != HostKeyChanged || len(info.KnownKeys) != 1 ||
		info.KnownKeys[0].Fingerprint != ssh.FingerprintSHA256(other.PublicKey()) ||
		info.KnownKeys[0].File != kh || info.KnownKeys[0].Line != 1 {
		t.Fatalf("info = %+v", info)
	}

	// Accept and save replaces the old entry and keeps the other host.
	cfg.HostKeyPrompt = func(HostKeyInfo) HostKeyDecision { return HostKeyAcceptAndSave }
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	lines := strings.Split(strings.TrimSpace(readFile(t, kh)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "example.org ") {
		t.Fatalf("known_hosts after replace:\n%s", strings.Join(lines, "\n"))
	}
	cfg.HostKeyPrompt = nil
	c, err = Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal("replaced key not trusted:", err)
	}
	c.Close()
}

func TestHostKeyNewType(t *testing.T) {
	s := newTestServer(t)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	// An ECDSA key stored for the host; the server only has ed25519.
	ecKey, _ := newECDSAPublic(t)
	AddKnownHost(kh, s.Addr, ecKey, false)
	cfg := Config{Host: "127.0.0.1", Port: s.Port, User: testUser, Password: testPassword, KnownHostsFiles: []string{kh}}
	var info HostKeyInfo
	cfg.HostKeyPrompt = func(i HostKeyInfo) HostKeyDecision { info = i; return HostKeyAcceptOnce }
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if info.Status != HostKeyNewType || len(info.KnownKeys) != 1 || info.KnownKeys[0].Type != ecKey.Type() {
		t.Errorf("info = %+v, want new key type", info)
	}
}

// A server with both an ed25519 and an ECDSA key, where only the ECDSA key is
// known: the client must ask for the known type and connect silently, rather
// than taking the ed25519 key and prompting.
func TestHostKeyPrefersKnownAlgorithm(t *testing.T) {
	ecPub, ecSigner := newECDSAPublic(t)
	s := newTestServer(t, withExtraHostKey(ecSigner))
	kh := filepath.Join(t.TempDir(), "known_hosts")
	AddKnownHost(kh, s.Addr, ecPub, false)
	cfg := Config{Host: "127.0.0.1", Port: s.Port, User: testUser, Password: testPassword, KnownHostsFiles: []string{kh}}
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestHostKeyHashedAndMultipleFiles(t *testing.T) {
	s := newTestServer(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "known_hosts")
	second := filepath.Join(dir, "known_hosts2")
	// Key only in the second (e.g. system-wide) file, hashed.
	if err := AddKnownHost(second, s.Addr, s.HostKey.PublicKey(), true); err != nil {
		t.Fatal(err)
	}
	if data := readFile(t, second); !strings.HasPrefix(data, "|1|") {
		t.Fatalf("not hashed: %q", data)
	}
	cfg := Config{Host: "127.0.0.1", Port: s.Port, User: testUser, Password: testPassword,
		KnownHostsFiles: []string{first, second}}
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()

	// HashKnownHosts on save.
	s2 := newTestServer(t)
	cfg2 := Config{Host: "127.0.0.1", Port: s2.Port, User: testUser, Password: testPassword,
		KnownHostsFiles: []string{first}, HashKnownHosts: true,
		HostKeyPrompt: func(HostKeyInfo) HostKeyDecision { return HostKeyAcceptAndSave }}
	c, err = Dial(t.Context(), cfg2)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if data := readFile(t, first); !strings.HasPrefix(data, "|1|") || strings.Contains(data, "127.0.0.1") {
		t.Errorf("saved entry not hashed: %q", data)
	}
}

func TestHostKeyRevoked(t *testing.T) {
	s := newTestServer(t)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	line := "@revoked * " + string(ssh.MarshalAuthorizedKey(s.HostKey.PublicKey()))
	os.WriteFile(kh, []byte(line), 0o600)
	cfg := Config{Host: "127.0.0.1", Port: s.Port, User: testUser, Password: testPassword, KnownHostsFiles: []string{kh},
		HostKeyPrompt: func(HostKeyInfo) HostKeyDecision { t.Error("revoked key offered to prompt"); return HostKeyAcceptOnce }}
	if _, err := Dial(t.Context(), cfg); !errors.Is(err, ErrHostKeyRevoked) {
		t.Errorf("err = %v, want ErrHostKeyRevoked", err)
	}
}

func TestHostKeyBadFile(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	os.WriteFile(kh, []byte("host ssh-ed25519 !!!notbase64\n"), 0o600)
	cfg := Config{Host: "127.0.0.1", Port: 1, User: "u", Password: "p", KnownHostsFiles: []string{kh}}
	if _, _, err := cfg.hostKeyCheck(); err == nil {
		t.Error("corrupt known_hosts accepted")
	}
}

func TestInsecureIgnoreHostKey(t *testing.T) {
	s := newTestServer(t)
	cfg := Config{Host: "127.0.0.1", Port: s.Port, User: testUser, Password: testPassword,
		KnownHostsFiles: []string{filepath.Join(t.TempDir(), "kh")}, InsecureIgnoreHostKey: true}
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestRemoveKnownHost(t *testing.T) {
	k := newSigner(t).PublicKey()
	keyText := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(k)))
	hashed := knownhosts.HashHostname("victim.example")
	content := strings.Join([]string{
		"# comment stays",
		"victim.example " + keyText,
		"other.example,victim.example,third.example " + keyText,
		hashed + " " + keyText,
		"[victim.example]:2222 " + keyText,
		"*.example " + keyText,
		"@cert-authority victim.example " + keyText,
		"",
	}, "\n")
	file := filepath.Join(t.TempDir(), "known_hosts")
	os.WriteFile(file, []byte(content), 0o600)

	n, err := RemoveKnownHost(file, "victim.example:22")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("removed %d, want 3", n)
	}
	want := strings.Join([]string{
		"# comment stays",
		"other.example,third.example " + keyText,
		"[victim.example]:2222 " + keyText,
		"*.example " + keyText,
		"@cert-authority victim.example " + keyText,
		"",
	}, "\n")
	if got := readFile(t, file); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	// Non-standard port entries are matched in bracket form.
	if n, _ := RemoveKnownHost(file, "victim.example:2222"); n != 1 {
		t.Errorf("port entry: removed %d", n)
	}
	// Nothing to remove: file untouched; missing file is fine.
	if n, err := RemoveKnownHost(file, "absent:22"); n != 0 || err != nil {
		t.Errorf("absent: %d %v", n, err)
	}
	if n, err := RemoveKnownHost(filepath.Join(t.TempDir(), "none"), "x"); n != 0 || err != nil {
		t.Errorf("missing file: %d %v", n, err)
	}
}

func TestAddKnownHostNoTrailingNewline(t *testing.T) {
	file := filepath.Join(t.TempDir(), "known_hosts")
	os.WriteFile(file, []byte("# no newline"), 0o600)
	k := newSigner(t).PublicKey()
	if err := AddKnownHost(file, "h.example:22", k, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(readFile(t, file), "\n")
	if len(lines) != 3 || lines[0] != "# no newline" || !strings.HasPrefix(lines[1], "h.example ") {
		t.Errorf("file = %q", lines)
	}
}

func TestHostKeyStatusString(t *testing.T) {
	for s, want := range map[HostKeyStatus]string{HostKeyUnknown: "unknown", HostKeyChanged: "changed",
		HostKeyNewType: "new key type", HostKeyStatus(9): "HostKeyStatus(9)"} {
		if s.String() != want {
			t.Errorf("%d: %q", int(s), s.String())
		}
	}
}
