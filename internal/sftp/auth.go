// This file turns the authentication settings of a Config into the ordered
// list of SSH authentication methods offered to the server. The order mirrors
// OpenSSH and Total Commander's SFTP plugin: first the SSH agent (no secrets
// leave the agent), then explicit private key files (asking for a passphrase
// only when a key is encrypted), then the password, and finally
// keyboard-interactive, which many PAM-based servers use to ask for the very
// same password. Offering them all in one handshake means the user is not
// asked to pick a method; the server simply accepts the first that works.
package sftp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// ErrNoAuthMethod is returned by Dial when the configuration offers no way at
// all to log in (no password, no usable key and no agent).
var ErrNoAuthMethod = errors.New("sftp: no authentication method available")

// authMethods builds the method list. The returned cleanup closes the agent
// connection, if one was opened, once the handshake is over.
func (c *Config) authMethods() ([]ssh.AuthMethod, func(), error) {
	var methods []ssh.AuthMethod
	cleanup := func() {}

	// 1. SSH agent.
	if c.Agent != nil || c.UseAgent {
		ag := c.Agent
		if ag == nil {
			conn, err := dialAgent(c.AgentSocket)
			if err == nil {
				ag = agent.NewClient(conn)
				cleanup = func() { conn.Close() }
			}
			// An unreachable agent is not fatal: other methods may work.
		}
		if ag != nil {
			methods = append(methods, ssh.PublicKeysCallback(ag.Signers))
		}
	}

	// 2. Private keys. Each key is its own method so that one encrypted key
	// the user declines to unlock does not block the others.
	var signers []ssh.Signer
	keyFiles := c.KeyFiles
	if len(keyFiles) == 0 && len(c.KeyData) == 0 && c.DefaultKeys {
		keyFiles = defaultKeyFiles()
	}
	for _, f := range keyFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			if c.DefaultKeys && len(c.KeyFiles) == 0 {
				continue // default keys are optional
			}
			cleanup()
			return nil, nil, fmt.Errorf("sftp: reading key %s: %w", f, err)
		}
		s, err := c.parseKey(filepath.Base(f), data)
		if err != nil {
			if errors.Is(err, errKeySkipped) {
				continue
			}
			cleanup()
			return nil, nil, fmt.Errorf("sftp: key %s: %w", f, err)
		}
		signers = append(signers, s)
	}
	for i, data := range c.KeyData {
		s, err := c.parseKey(fmt.Sprintf("key #%d", i+1), data)
		if err != nil {
			if errors.Is(err, errKeySkipped) {
				continue
			}
			cleanup()
			return nil, nil, fmt.Errorf("sftp: key #%d: %w", i+1, err)
		}
		signers = append(signers, s)
	}
	if len(signers) > 0 {
		methods = append(methods, ssh.PublicKeys(signers...))
	}

	// 3. Password, then 4. keyboard-interactive.
	if c.Password != "" {
		methods = append(methods, ssh.Password(c.Password))
	}
	if c.Password != "" || c.KeyboardInteractive != nil {
		methods = append(methods, ssh.KeyboardInteractive(c.answerChallenge))
	}

	if len(methods) == 0 {
		cleanup()
		return nil, nil, ErrNoAuthMethod
	}
	return methods, cleanup, nil
}

// errKeySkipped marks an encrypted key whose passphrase the user declined to
// give; the key is silently left out.
var errKeySkipped = errors.New("key skipped")

// parseKey parses a private key, decrypting it with Passphrase or, failing
// that, with whatever PassphraseFunc returns. A wrong passphrase from the
// prompt is asked again up to three times, like ssh does.
func (c *Config) parseKey(name string, data []byte) (ssh.Signer, error) {
	s, err := ssh.ParsePrivateKey(data)
	if err == nil {
		return s, nil
	}
	var missing *ssh.PassphraseMissingError
	if !errors.As(err, &missing) {
		return nil, err
	}
	if c.Passphrase != "" {
		s, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(c.Passphrase))
		if err == nil {
			return s, nil
		}
		if c.PassphraseFunc == nil {
			return nil, err
		}
	}
	if c.PassphraseFunc == nil {
		return nil, errors.New("key is encrypted and no passphrase was given")
	}
	for attempt := 0; attempt < 3; attempt++ {
		pass, perr := c.PassphraseFunc(name)
		if perr != nil {
			return nil, errKeySkipped
		}
		s, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(pass))
		if err == nil {
			return s, nil
		}
	}
	return nil, err
}

// answerChallenge handles keyboard-interactive rounds. Prompts that look like
// a password request are answered with Password; everything else goes to the
// KeyboardInteractive callback. A round with no questions (servers send those
// to show a banner) is answered with an empty list.
func (c *Config) answerChallenge(name, instruction string, questions []string, echos []bool) ([]string, error) {
	if len(questions) == 0 {
		return nil, nil
	}
	allPassword := c.Password != ""
	for i, q := range questions {
		if echos[i] || !looksLikePasswordPrompt(q) {
			allPassword = false
		}
	}
	if allPassword {
		answers := make([]string, len(questions))
		for i := range answers {
			answers[i] = c.Password
		}
		return answers, nil
	}
	if c.KeyboardInteractive != nil {
		return c.KeyboardInteractive(name, instruction, questions, echos)
	}
	return nil, errors.New("sftp: server asked an interactive question that cannot be answered")
}

func looksLikePasswordPrompt(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "password") || strings.Contains(q, "passphrase") ||
		strings.Contains(q, "passwort") || strings.Contains(q, "jelszó")
}

func defaultKeyFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var out []string
	for _, n := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		p := filepath.Join(home, ".ssh", n)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// agentConn is what dialAgent returns: a byte stream to the agent.
type agentConn interface {
	io.ReadWriteCloser
}

// ListAgentKeys connects to the SSH agent (socket as in Config.AgentSocket)
// and returns the keys it holds, so a connection dialog can show which keys
// will be offered.
func ListAgentKeys(socket string) ([]*agent.Key, error) {
	conn, err := dialAgent(socket)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return agent.NewClient(conn).List()
}
