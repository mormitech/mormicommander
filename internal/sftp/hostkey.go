// This file checks the server's host key, the step that protects a user from
// talking to an impostor. The key the server presents is looked up in the
// OpenSSH known_hosts files (the same files the ssh command uses, so trust is
// shared with the terminal). A known key connects silently. An unknown key
// (first connection) or a changed key (possibly a man-in-the-middle attack)
// is handed to a HostKeyPrompt callback, which the UI implements as Total
// Commander's "the server's host key is not known / has changed, accept?"
// dialog. The user can reject, accept for this session only, or accept and
// save the key so future connections are silent. Without a prompt, unknown
// and changed keys are refused, which is the only safe unattended default.
package sftp

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// HostKeyStatus says why a host key needs the user's decision.
type HostKeyStatus int

const (
	// HostKeyUnknown: no key of any type is stored for this host.
	HostKeyUnknown HostKeyStatus = iota
	// HostKeyChanged: a key of the same type is stored for this host but it
	// differs from the one presented. This is the dangerous case.
	HostKeyChanged
	// HostKeyNewType: keys are stored for this host, but only of other key
	// types (for example the server got a new ed25519 key and known_hosts
	// only has its RSA key). Less alarming than HostKeyChanged.
	HostKeyNewType
)

func (s HostKeyStatus) String() string {
	switch s {
	case HostKeyUnknown:
		return "unknown"
	case HostKeyChanged:
		return "changed"
	case HostKeyNewType:
		return "new key type"
	}
	return fmt.Sprintf("HostKeyStatus(%d)", int(s))
}

// KnownKey is a key already stored in a known_hosts file.
type KnownKey struct {
	Type        string // e.g. "ssh-ed25519"
	Fingerprint string // SHA256:... form
	File        string
	Line        int
}

// HostKeyInfo is everything the host key dialog shows.
type HostKeyInfo struct {
	Host           string // address as dialled, host:port
	Remote         string // the actual remote IP:port
	Status         HostKeyStatus
	KeyType        string        // e.g. "ssh-ed25519"
	Fingerprint    string        // "SHA256:base64", what OpenSSH prints
	FingerprintMD5 string        // legacy "aa:bb:..." form, still shown by some tools
	Key            ssh.PublicKey // the presented key
	KnownKeys      []KnownKey    // stored keys for this host (Changed / NewType)
	SaveFile       string        // where AcceptAndSave would write the key
}

// HostKeyDecision is the user's answer to the host key dialog.
type HostKeyDecision int

const (
	// HostKeyReject aborts the connection.
	HostKeyReject HostKeyDecision = iota
	// HostKeyAcceptOnce connects but does not remember the key.
	HostKeyAcceptOnce
	// HostKeyAcceptAndSave connects and stores the key in the first
	// known_hosts file. For a changed key the old entries for this host are
	// removed from that file first.
	HostKeyAcceptAndSave
)

// HostKeyPrompt asks the user about an unknown or changed host key. It is
// called during the SSH handshake, from the dialling goroutine.
type HostKeyPrompt func(info HostKeyInfo) HostKeyDecision

// HostKeyError is returned (wrapped) by Dial when a host key was refused,
// either by the user or because there was no prompt to ask.
type HostKeyError struct {
	Info HostKeyInfo
}

func (e *HostKeyError) Error() string {
	return fmt.Sprintf("sftp: host key for %s %s (%s %s) was not accepted",
		e.Info.Host, e.Info.Status, e.Info.KeyType, e.Info.Fingerprint)
}

// ErrHostKeyRevoked is returned when the presented key is marked @revoked in
// known_hosts. It is never offered to the prompt.
var ErrHostKeyRevoked = errors.New("sftp: host key is revoked in known_hosts")

// hostKeyCheck builds the HostKeyCallback for one dial, plus the host key
// algorithms to prefer so that the server presents a key type we already
// know (otherwise a server with several keys could present one we have not
// stored and trigger a needless prompt).
func (c *Config) hostKeyCheck() (ssh.HostKeyCallback, []string, error) {
	if c.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil, nil
	}
	files := c.knownHostsFiles()
	var existing []string
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			existing = append(existing, f)
		}
	}
	var db ssh.HostKeyCallback
	if len(existing) > 0 {
		var err error
		db, err = knownhosts.New(existing...)
		if err != nil {
			return nil, nil, fmt.Errorf("sftp: reading known_hosts: %w", err)
		}
	}
	saveFile := ""
	if len(files) > 0 {
		saveFile = files[0]
	}

	addr := c.Addr()
	algos := knownAlgorithms(db, addr)

	cb := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		info := HostKeyInfo{
			Host:           hostname,
			Remote:         remote.String(),
			Status:         HostKeyUnknown,
			KeyType:        key.Type(),
			Fingerprint:    ssh.FingerprintSHA256(key),
			FingerprintMD5: ssh.FingerprintLegacyMD5(key),
			Key:            key,
			SaveFile:       saveFile,
		}
		if db != nil {
			err := db(hostname, remote, key)
			if err == nil {
				return nil
			}
			var revoked *knownhosts.RevokedError
			if errors.As(err, &revoked) {
				return ErrHostKeyRevoked
			}
			var ke *knownhosts.KeyError
			if !errors.As(err, &ke) {
				return err
			}
			for _, w := range ke.Want {
				info.KnownKeys = append(info.KnownKeys, KnownKey{
					Type:        w.Key.Type(),
					Fingerprint: ssh.FingerprintSHA256(w.Key),
					File:        w.Filename,
					Line:        w.Line,
				})
				if w.Key.Type() == key.Type() {
					info.Status = HostKeyChanged
				} else if info.Status == HostKeyUnknown {
					info.Status = HostKeyNewType
				}
			}
		}
		if c.HostKeyPrompt == nil {
			return &HostKeyError{Info: info}
		}
		switch c.HostKeyPrompt(info) {
		case HostKeyAcceptOnce:
			return nil
		case HostKeyAcceptAndSave:
			if saveFile == "" {
				return errors.New("sftp: no known_hosts file to save the key to")
			}
			if info.Status == HostKeyChanged {
				if _, err := RemoveKnownHost(saveFile, hostname); err != nil {
					return err
				}
			}
			return AddKnownHost(saveFile, hostname, key, c.HashKnownHosts)
		default:
			return &HostKeyError{Info: info}
		}
	}
	return cb, algos, nil
}

// knownAlgorithms asks the known_hosts database which key types it holds for
// addr by checking a throw-away key: the resulting KeyError lists every
// stored key for the host.
func knownAlgorithms(db ssh.HostKeyCallback, addr string) []string {
	if db == nil {
		return nil
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil
	}
	probe, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil
	}
	err = db(addr, &net.TCPAddr{IP: net.IPv4zero}, probe.PublicKey())
	var ke *knownhosts.KeyError
	if !errors.As(err, &ke) || len(ke.Want) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(a string) {
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	for _, w := range ke.Want {
		switch t := w.Key.Type(); t {
		case ssh.KeyAlgoRSA:
			add(ssh.KeyAlgoRSASHA512)
			add(ssh.KeyAlgoRSASHA256)
			add(ssh.KeyAlgoRSA)
		default:
			add(t)
		}
	}
	// Like OpenSSH, known types go first but the others stay allowed, so a
	// server that dropped its old key type can still connect (and the user
	// is asked about the new key instead of getting a negotiation error).
	for _, a := range ssh.SupportedAlgorithms().HostKeys {
		add(a)
	}
	return out
}

// AddKnownHost appends a key for host (host:port or bare host) to a
// known_hosts file, creating the file and its folder with private
// permissions when needed. With hash set the host name is stored hashed.
func AddKnownHost(file, host string, key ssh.PublicKey, hash bool) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return fmt.Errorf("sftp: creating known_hosts folder: %w", err)
	}
	name := knownhosts.Normalize(host)
	if hash {
		name = knownhosts.HashHostname(name)
	}
	line := knownhosts.Line([]string{name}, key)

	// Make sure we start on a fresh line even if the file lacks a final
	// newline (hand-edited files often do).
	prefix := ""
	if data, err := os.ReadFile(file); err == nil && len(data) > 0 && data[len(data)-1] != '\n' {
		prefix = "\n"
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("sftp: opening known_hosts: %w", err)
	}
	if _, err := f.WriteString(prefix + line + "\n"); err != nil {
		f.Close()
		return fmt.Errorf("sftp: writing known_hosts: %w", err)
	}
	return f.Close()
}

// RemoveKnownHost deletes every line of a known_hosts file whose host list
// names host exactly (plain or hashed). Lines that list several hosts lose
// only that host; wildcard patterns and markers are left alone. It returns
// how many host names were removed. A missing file removes nothing.
func RemoveKnownHost(file, host string) (int, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	want := knownhosts.Normalize(host)
	var out bytes.Buffer
	removed := 0
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "@") {
			out.WriteString(line + "\n")
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			out.WriteString(line + "\n")
			continue
		}
		hosts := strings.Split(fields[0], ",")
		kept := hosts[:0]
		for _, h := range hosts {
			if hostEntryMatches(h, want) {
				removed++
				continue
			}
			kept = append(kept, h)
		}
		switch {
		case len(kept) == len(hosts):
			out.WriteString(line + "\n")
		case len(kept) == 0:
			// whole line dropped
		default:
			out.WriteString(strings.Join(kept, ",") + " " + strings.Join(fields[1:], " ") + "\n")
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	if removed == 0 {
		return 0, nil
	}
	return removed, writeFileAtomic(file, out.Bytes(), 0o600)
}

// hostEntryMatches compares one host field of a known_hosts line with an
// already normalised host name, handling "|1|salt|hash" hashed entries.
func hostEntryMatches(entry, want string) bool {
	if strings.HasPrefix(entry, "|1|") {
		parts := strings.Split(entry[3:], "|")
		if len(parts) != 2 {
			return false
		}
		salt, err1 := base64.StdEncoding.DecodeString(parts[0])
		sum, err2 := base64.StdEncoding.DecodeString(parts[1])
		if err1 != nil || err2 != nil {
			return false
		}
		mac := hmac.New(sha1.New, salt)
		mac.Write([]byte(want))
		return hmac.Equal(mac.Sum(nil), sum)
	}
	return entry == want
}

// writeFileAtomic replaces a file by writing a sibling temp file and renaming
// it over the original, so a crash never leaves a half-written known_hosts.
func writeFileAtomic(name string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(name), ".known_hosts-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
