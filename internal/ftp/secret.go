// This file protects saved FTP passwords. The checklist demands that
// passwords are never written to disk in plain text, so every password in
// the connection manager goes through a SecretBox before it is stored. The
// box built in here is Total Commander's master password: a key is derived
// from the user's master password with PBKDF2-SHA256 and a random salt, and
// each password is sealed with AES-256-GCM, which also detects tampering. A
// small known value sealed with the same key lets the UI tell a wrong master
// password from a right one without trying to decrypt real passwords. The UI
// may plug in an OS-keychain box instead; it only has to implement Seal and
// Open.
package ftp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// SecretBox seals and opens stored passwords.
type SecretBox interface {
	Seal(plain string) (string, error)
	Open(sealed string) (string, error)
}

// sealedPrefix marks values produced by MasterKey.Seal. Other boxes (such as
// a keychain reference) use their own prefixes; Store.Save refuses values
// with no known prefix so a plain password can never slip through.
const sealedPrefix = "enc:v1:"

// KeyRefPrefix is the prefix a keychain-backed SecretBox should use for
// the references it returns, so Store.Save accepts them.
const KeyRefPrefix = "keychain:"

// DefaultKDFIterations is the PBKDF2 work factor for new master passwords.
const DefaultKDFIterations = 210000

const checkPlain = "mormiCommander master password check"

// MasterKey is a SecretBox derived from a master password.
type MasterKey struct {
	aead cipher.AEAD
}

// CryptoHeader is stored with the connection list; it holds everything
// needed to re-derive the key except the master password itself.
type CryptoHeader struct {
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	Iterations int    `json:"iterations"`
	Check      string `json:"check"`
}

// NewMasterKey creates a fresh salt and verification value for password.
func NewMasterKey(password string, iterations int) (*MasterKey, CryptoHeader, error) {
	if password == "" {
		return nil, CryptoHeader{}, errors.New("ftp: empty master password")
	}
	if iterations <= 0 {
		iterations = DefaultKDFIterations
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, CryptoHeader{}, err
	}
	h := CryptoHeader{KDF: "pbkdf2-sha256", Salt: base64.StdEncoding.EncodeToString(salt), Iterations: iterations}
	k, err := deriveKey(password, salt, iterations)
	if err != nil {
		return nil, CryptoHeader{}, err
	}
	if h.Check, err = k.Seal(checkPlain); err != nil {
		return nil, CryptoHeader{}, err
	}
	return k, h, nil
}

// OpenMasterKey re-derives the key from password and checks it against
// the header; a wrong password gives ErrWrongMasterPass.
func OpenMasterKey(password string, h CryptoHeader) (*MasterKey, error) {
	if h.KDF != "pbkdf2-sha256" {
		return nil, fmt.Errorf("ftp: unknown key derivation %q", h.KDF)
	}
	salt, err := base64.StdEncoding.DecodeString(h.Salt)
	if err != nil {
		return nil, fmt.Errorf("ftp: bad salt: %w", err)
	}
	k, err := deriveKey(password, salt, h.Iterations)
	if err != nil {
		return nil, err
	}
	if got, err := k.Open(h.Check); err != nil || got != checkPlain {
		return nil, ErrWrongMasterPass
	}
	return k, nil
}

func deriveKey(password string, salt []byte, iterations int) (*MasterKey, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &MasterKey{aead: aead}, nil
}

// Seal encrypts plain with a fresh random nonce.
func (k *MasterKey) Seal(plain string) (string, error) {
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := k.aead.Seal(nonce, nonce, []byte(plain), nil)
	return sealedPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// Open decrypts a value produced by Seal.
func (k *MasterKey) Open(sealed string) (string, error) {
	if !strings.HasPrefix(sealed, sealedPrefix) {
		return "", errors.New("ftp: value is not sealed by a master key")
	}
	raw, err := base64.StdEncoding.DecodeString(sealed[len(sealedPrefix):])
	if err != nil {
		return "", err
	}
	ns := k.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ftp: sealed value too short")
	}
	pt, err := k.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", ErrWrongMasterPass
	}
	return string(pt), nil
}

// IsSealed reports whether a stored value is in a protected form (sealed
// by a master key or a keychain reference) rather than plain text.
func IsSealed(v string) bool {
	return strings.HasPrefix(v, sealedPrefix) || strings.HasPrefix(v, KeyRefPrefix)
}
