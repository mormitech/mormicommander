// Package keychain keeps small secrets (saved server passwords today, an AI provider's API key later) in the operating
// system's own secret store instead of a file of the app: the login keychain on macOS, the Credential Manager on
// Windows and the Secret Service (GNOME Keyring, KWallet and friends, through libsecret's secret-tool) on Linux. The
// store protects the secret with the user's login, so the app never has to ask for a password of its own to read it
// back, and the secret never lands in a file the app writes.
//
// A secret is known by a name ("site.4f2a…", "ai.anthropic"); every secret of the app lives under one service,
// "mormiCommander", so the user finds them together in Keychain Access, the Credential Manager or Seahorse. Names are
// limited to letters, digits, '.', '_' and '-', because on macOS and Linux they end up as words of a command, and a
// name that can't carry quotes or spaces can't change what that command does.
//
// The secret itself never goes on a command line, since any user on the machine can read the command lines of
// running processes (ps): on macOS it is written to the standard input of "security -i", on Linux to the standard
// input of "secret-tool store", and on Windows it is handed to CredWriteW in memory. The platform files say how.
//
// Nothing here talks to the network: the stores are local services reached through a program or a system call.
package keychain

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
)

// Service is the name every secret of the app is filed under.
const Service = "mormiCommander"

// Keychain gets, sets and deletes named secrets.
type Keychain interface {
	// Get returns the secret of name, or ErrNotFound when there is none.
	Get(name string) (string, error)
	// Set stores secret under name, replacing what was there.
	Set(name, secret string) error
	// Delete removes the secret of name; a name with no secret is not an error.
	Delete(name string) error
}

// ErrNotFound is returned by Get for a name with no secret.
var ErrNotFound = errors.New("keychain: no such secret")

// ErrUnavailable is returned by Open when this system has no secret store the package can use; the error wrapping it
// says why in a sentence that can be shown to the user.
var ErrUnavailable = errors.New("keychain: not available")

// unavailable wraps ErrUnavailable with the reason shown to the user.
type unavailable struct{ reason string }

func (u unavailable) Error() string { return u.reason }
func (u unavailable) Unwrap() error { return ErrUnavailable }

// Unavailable makes an ErrUnavailable error with a reason for the user.
func Unavailable(reason string) error { return unavailable{reason} }

var nameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// ValidName says whether name can be used for a secret (see the package comment for why it is so narrow).
func ValidName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("keychain: %q is not a valid secret name (letters, digits, '.', '_' and '-' only)", name)
	}
	return nil
}

// Fake is a Keychain in memory, for tests of the code that uses one. Fail, when set, is returned by every call, to
// test what happens when the system store refuses.
type Fake struct {
	mu      sync.Mutex
	secrets map[string]string
	Fail    error
}

// NewFake returns an empty Fake.
func NewFake() *Fake { return &Fake{secrets: map[string]string{}} }

func (f *Fake) Get(name string) (string, error) {
	if err := ValidName(name); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return "", f.Fail
	}
	s, ok := f.secrets[name]
	if !ok {
		return "", ErrNotFound
	}
	return s, nil
}

func (f *Fake) Set(name, secret string) error {
	if err := ValidName(name); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	f.secrets[name] = secret
	return nil
}

func (f *Fake) Delete(name string) error {
	if err := ValidName(name); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	delete(f.secrets, name)
	return nil
}

// Names lists the names that have a secret, sorted (for tests).
func (f *Fake) Names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.secrets))
	for n := range f.secrets {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
