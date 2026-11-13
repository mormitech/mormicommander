// Tests for every login method against the in-test server: password,
// private key files (plain and passphrase protected, with the passphrase
// given up front or asked through the prompt), in-memory keys,
// keyboard-interactive (password-only and with an extra one-time code), the
// SSH agent (handed in directly and reached through a Unix socket), and the
// failure cases that must produce clear errors.
package sftp

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestAuthMethods(t *testing.T) {
	plainKey, plainPub := newKeyPEM(t, "")
	encKey, encPub := newKeyPEM(t, "hunter2")
	strangerKey, _ := newKeyPEM(t, "")
	keyDir := t.TempDir()
	plainFile := filepath.Join(keyDir, "id_plain")
	encFile := filepath.Join(keyDir, "id_enc")
	strangerFile := filepath.Join(keyDir, "id_stranger")
	os.WriteFile(plainFile, plainKey, 0o600)
	os.WriteFile(encFile, encKey, 0o600)
	os.WriteFile(strangerFile, strangerKey, 0o600)

	s := newTestServer(t, withAuthorizedKey(plainPub), withAuthorizedKey(encPub))

	asked := 0
	tests := []struct {
		name    string
		mod     func(c *Config)
		wantErr bool
	}{
		{"password", func(c *Config) {}, false},
		{"wrong password", func(c *Config) { c.Password = "nope" }, true},
		{"wrong user", func(c *Config) { c.User = "mallory" }, true},
		{"key file", func(c *Config) { c.Password = ""; c.KeyFiles = []string{plainFile} }, false},
		{"unknown key then password", func(c *Config) { c.KeyFiles = []string{strangerFile} }, false},
		{"unknown key only", func(c *Config) { c.Password = ""; c.KeyFiles = []string{strangerFile} }, true},
		{"several keys, second works", func(c *Config) {
			c.Password = ""
			c.KeyFiles = []string{strangerFile, plainFile}
		}, false},
		{"key data", func(c *Config) { c.Password = ""; c.KeyData = [][]byte{plainKey} }, false},
		{"encrypted key with passphrase", func(c *Config) {
			c.Password = ""
			c.KeyFiles = []string{encFile}
			c.Passphrase = "hunter2"
		}, false},
		{"encrypted key via prompt after wrong passphrase", func(c *Config) {
			c.Password = ""
			c.KeyFiles = []string{encFile}
			c.Passphrase = "wrong"
			attempts := 0
			c.PassphraseFunc = func(name string) (string, error) {
				attempts++
				asked++
				if name != "id_enc" {
					t.Errorf("prompt for %q", name)
				}
				if attempts == 1 {
					return "still wrong", nil // re-asked
				}
				return "hunter2", nil
			}
		}, false},
		{"encrypted key, prompt declined, password fallback", func(c *Config) {
			c.KeyFiles = []string{encFile}
			c.PassphraseFunc = func(string) (string, error) { return "", errors.New("cancel") }
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := s.baseConfig(t)
			tc.mod(&cfg)
			c, err := Dial(t.Context(), cfg)
			if tc.wantErr {
				if err == nil {
					c.Close()
					t.Fatal("expected login failure")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			c.Close()
		})
	}
	if asked != 2 {
		t.Errorf("passphrase prompt asked %d times, want 2", asked)
	}
}

func TestAuthConfigErrors(t *testing.T) {
	encKey, _ := newKeyPEM(t, "pw")
	dir := t.TempDir()
	encFile := filepath.Join(dir, "enc")
	os.WriteFile(encFile, encKey, 0o600)
	garbage := filepath.Join(dir, "garbage")
	os.WriteFile(garbage, []byte("not a key"), 0o600)
	tests := []struct {
		name string
		cfg  Config
		want error
	}{
		{"missing key file", Config{KeyFiles: []string{filepath.Join(dir, "none")}}, nil},
		{"garbage key", Config{KeyFiles: []string{garbage}}, nil},
		{"encrypted, no passphrase", Config{KeyFiles: []string{encFile}}, nil},
		{"encrypted, wrong passphrase", Config{KeyFiles: []string{encFile}, Passphrase: "x"}, nil},
		{"encrypted, declined, nothing else", Config{KeyFiles: []string{encFile},
			PassphraseFunc: func(string) (string, error) { return "", errors.New("no") }}, ErrNoAuthMethod},
		{"nothing", Config{}, ErrNoAuthMethod},
		{"default keys, none present", Config{DefaultKeys: true, KeyData: nil}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cfg.DefaultKeys {
				t.Setenv("HOME", t.TempDir())
				t.Setenv("USERPROFILE", os.Getenv("HOME"))
			}
			_, cleanup, err := tc.cfg.authMethods()
			if err == nil {
				cleanup()
				t.Fatal("expected error")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDefaultKeys(t *testing.T) {
	key, pub := newKeyPEM(t, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), key, 0o600)
	s := newTestServer(t, withAuthorizedKey(pub))
	cfg := s.baseConfig(t)
	cfg.Password = ""
	cfg.DefaultKeys = true
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestKeyboardInteractive(t *testing.T) {
	t.Run("password answers the prompt", func(t *testing.T) {
		s := newTestServer(t, withKbdOnly(""))
		c, err := Dial(t.Context(), s.baseConfig(t))
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	})
	t.Run("one-time code via callback", func(t *testing.T) {
		s := newTestServer(t, withKbdOnly("424242"))
		cfg := s.baseConfig(t)
		cfg.KeyboardInteractive = func(name, instr string, qs []string, echos []bool) ([]string, error) {
			ans := make([]string, len(qs))
			for i, q := range qs {
				if looksLikePasswordPrompt(q) {
					ans[i] = testPassword
				} else {
					ans[i] = "424242"
				}
			}
			return ans, nil
		}
		c, err := Dial(t.Context(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	})
	t.Run("one-time code without callback fails", func(t *testing.T) {
		s := newTestServer(t, withKbdOnly("424242"))
		if c, err := Dial(t.Context(), s.baseConfig(t)); err == nil {
			c.Close()
			t.Fatal("expected failure")
		}
	})
}

func TestAnswerChallenge(t *testing.T) {
	cfg := Config{Password: "pw"}
	tests := []struct {
		qs      []string
		echos   []bool
		want    []string
		wantErr bool
	}{
		{nil, nil, nil, false},
		{[]string{"Password: "}, []bool{false}, []string{"pw"}, false},
		{[]string{"Jelszó:"}, []bool{false}, []string{"pw"}, false},
		{[]string{"Password: "}, []bool{true}, nil, true}, // echoed: not a password field
		{[]string{"Token:"}, []bool{false}, nil, true},
	}
	for _, tc := range tests {
		got, err := cfg.answerChallenge("", "", tc.qs, tc.echos)
		if (err != nil) != tc.wantErr {
			t.Errorf("%v: err = %v", tc.qs, err)
			continue
		}
		if len(got) != len(tc.want) || (len(got) > 0 && got[0] != tc.want[0]) {
			t.Errorf("%v: got %v want %v", tc.qs, got, tc.want)
		}
	}
}

func TestAgentAuth(t *testing.T) {
	key, pub := newKeyPEM(t, "")
	raw, err := ssh.ParseRawPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: raw, Comment: "test key"}); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t, withAuthorizedKey(pub))

	t.Run("injected agent", func(t *testing.T) {
		cfg := s.baseConfig(t)
		cfg.Password = ""
		cfg.Agent = keyring
		c, err := Dial(t.Context(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	})

	t.Run("agent socket", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("unix sockets")
		}
		// Unix socket paths are limited to ~104 bytes on macOS, and
		// t.TempDir() paths can be longer, so use a short temp dir.
		dir, err := os.MkdirTemp("", "ag")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		sock := filepath.Join(dir, "a.sock")
		ln, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go func() { agent.ServeAgent(keyring, c); c.Close() }()
			}
		}()

		keys, err := ListAgentKeys(sock)
		if err != nil || len(keys) != 1 || keys[0].Comment != "test key" {
			t.Fatalf("ListAgentKeys = %v, %v", keys, err)
		}

		cfg := s.baseConfig(t)
		cfg.Password = ""
		cfg.UseAgent = true
		cfg.AgentSocket = sock
		c, err := Dial(t.Context(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()

		// Through SSH_AUTH_SOCK as well.
		t.Setenv("SSH_AUTH_SOCK", sock)
		cfg.AgentSocket = ""
		c, err = Dial(t.Context(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	})

	t.Run("unreachable agent is not fatal", func(t *testing.T) {
		t.Setenv("SSH_AUTH_SOCK", "")
		cfg := s.baseConfig(t)
		cfg.UseAgent = true
		cfg.AgentSocket = filepath.Join(t.TempDir(), "missing.sock")
		c, err := Dial(t.Context(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
		if _, err := ListAgentKeys(cfg.AgentSocket); err == nil {
			t.Error("ListAgentKeys on missing socket succeeded")
		}
	})
}
