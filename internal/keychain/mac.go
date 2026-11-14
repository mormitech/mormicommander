package keychain

// The macOS store: the user's login keychain, reached through /usr/bin/security (generic password items, service
// "mormiCommander", account = the secret's name). Going through the program instead of the Security framework keeps
// cgo out of the package, and the items it makes trust /usr/bin/security, so reading them back never brings up the
// "mormiCommander wants to use your keychain" question.
//
// Keeping the secret off the command line: "security add-generic-password -w secret" would put the secret in the
// process list, and "-w" given as the last option without a value (the documented way to be prompted) reads it with
// readpassphrase, which wants a terminal, asks twice, and leaves no room for a keychain file argument after it. So
// the command is not given as arguments at all: "security -i" reads commands from its standard input, one per line,
// and the add command is written there, with the secret as "-X <hex>" (the password data as hexadecimal). Hex needs
// no quoting, so no character of the secret can end the word or start another option, and the process list only ever
// shows "security -i". Reading back uses "find-generic-password -g", which prints the password to standard error,
// either as "password: "text"" or, when it has bytes that are not printable ASCII, as "password: 0x<hex>  "…"";
// both forms are decoded ("-w" prints the hex form without the marker, so a password that looks like hex could not be
// told apart). Deleting needs no secret, so it runs as a plain command.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// macSecurity is the program the macOS store runs.
const macSecurity = "/usr/bin/security"

// macNotFound is the exit code of security for "The specified item could not be found in the keychain."
const macNotFound = 44

// MaxSecretLen is the longest secret (in bytes) the package stores: the Windows Credential Manager takes 2560 bytes of
// UTF-16, and one limit for every system keeps a secret saved on one machine valid on another.
const MaxSecretLen = 1024

// macKeychain is the login keychain (or, for tests only, the keychain file in path).
type macKeychain struct {
	service string
	path    string // "" = the default (login) keychain
	run     runner
}

// keychainArg is the keychain file argument at the end of a command ("" for the default keychain).
func (k *macKeychain) keychainArg() []string {
	if k.path == "" {
		return nil
	}
	return []string{k.path}
}

// macAddScript is the line written to "security -i" to store secret: "-U" replaces an item that is already there.
func macAddScript(service, name, secret, path string) (string, error) {
	if err := ValidName(name); err != nil {
		return "", err
	}
	if err := ValidName(service); err != nil {
		return "", err
	}
	if strings.ContainsAny(path, "\"\n\r\\") {
		return "", errors.New("keychain: the keychain path can't have quotes, backslashes or line breaks")
	}
	line := fmt.Sprintf("add-generic-password -U -s %s -a %s -X %s", service, name, hex.EncodeToString([]byte(secret)))
	if path != "" {
		line += ` "` + path + `"`
	}
	return line + "\n", nil
}

// macFindArgs are the arguments that print the item of name (its password on standard error).
func (k *macKeychain) macFindArgs(name string) []string {
	return append([]string{"find-generic-password", "-s", k.service, "-a", name, "-g"}, k.keychainArg()...)
}

// macDeleteArgs are the arguments that delete the item of name.
func (k *macKeychain) macDeleteArgs(name string) []string {
	return append([]string{"delete-generic-password", "-s", k.service, "-a", name}, k.keychainArg()...)
}

// macParsePassword reads the password out of what "find-generic-password -g" printed on standard error.
func macParsePassword(stderr string) (string, error) {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimRight(line, "\r")
		v, ok := strings.CutPrefix(line, "password: ")
		if !ok {
			if line == "password:" {
				return "", nil
			}
			continue
		}
		switch {
		case v == "":
			return "", nil
		case strings.HasPrefix(v, "0x"):
			h, _, _ := strings.Cut(v[2:], " ")
			b, err := hex.DecodeString(h)
			if err != nil {
				return "", fmt.Errorf("keychain: can't read the password security printed: %v", err)
			}
			return string(b), nil
		case len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"':
			return v[1 : len(v)-1], nil
		}
		return "", errors.New("keychain: can't read the password security printed")
	}
	return "", errors.New("keychain: security printed no password")
}

func checkSecret(secret string) error {
	if secret == "" {
		return errors.New("keychain: an empty secret can't be stored (delete it instead)")
	}
	if len(secret) > MaxSecretLen {
		return fmt.Errorf("keychain: the secret is longer than %d bytes", MaxSecretLen)
	}
	return nil
}

func (k *macKeychain) Get(name string) (string, error) {
	if err := ValidName(name); err != nil {
		return "", err
	}
	r, err := k.run(macSecurity, k.macFindArgs(name), "")
	if err != nil {
		return "", err
	}
	switch r.code {
	case 0:
		return macParsePassword(r.stderr)
	case macNotFound:
		return "", ErrNotFound
	}
	return "", fmt.Errorf("keychain: %s", macErr(r))
}

func (k *macKeychain) Set(name, secret string) error {
	if err := checkSecret(secret); err != nil {
		return err
	}
	script, err := macAddScript(k.service, name, secret, k.path)
	if err != nil {
		return err
	}
	r, err := k.run(macSecurity, []string{"-i"}, script)
	if err != nil {
		return err
	}
	// In interactive mode security does not always pass a failed command's status on as its own exit code, but a
	// command that worked prints nothing, so any message means it failed.
	if r.code != 0 || strings.TrimSpace(r.stderr) != "" {
		return fmt.Errorf("keychain: %s", macErr(r))
	}
	return nil
}

func (k *macKeychain) Delete(name string) error {
	if err := ValidName(name); err != nil {
		return err
	}
	r, err := k.run(macSecurity, k.macDeleteArgs(name), "")
	if err != nil {
		return err
	}
	if r.code != 0 && r.code != macNotFound {
		return fmt.Errorf("keychain: %s", macErr(r))
	}
	return nil
}

// macErr is the message of a failed security command, without the usage text it prints after a bad option.
func macErr(r runResult) string {
	if l := firstLine(r.stderr); l != "" {
		return strings.TrimPrefix(l, "security: ")
	}
	return fmt.Sprintf("security ended with code %d", r.code)
}
