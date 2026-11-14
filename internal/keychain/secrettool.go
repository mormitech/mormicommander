package keychain

// The Linux store: the Secret Service (GNOME Keyring, KWallet, KeePassXC…), reached through libsecret's secret-tool.
// A secret is the item with the attributes service=mormiCommander and id=<name>. secret-tool reads the secret of
// "store" from its standard input when that is not a terminal (all of it, as it is), so the secret is never an
// argument; "lookup" prints it on standard output with nothing added, and "clear" deletes every item with the
// attributes. secret-tool talks to the store over the session's D-Bus, not the network.
//
// When secret-tool is not installed, or no Secret Service runs (a bare window manager, a server), the option is not
// offered: Open says why.

import (
	"fmt"
	"strings"
)

// secretToolKeychain is the Secret Service through the program in path.
type secretToolKeychain struct {
	service string
	path    string // the secret-tool program
	run     runner
}

func (k *secretToolKeychain) attrs(name string) []string {
	return []string{"service", k.service, "id", name}
}

// storeArgs are the arguments of "secret-tool store"; the label is what Seahorse shows for the item.
func (k *secretToolKeychain) storeArgs(name string) []string {
	return append([]string{"store", "--label=" + k.service + " " + name}, k.attrs(name)...)
}

func (k *secretToolKeychain) lookupArgs(name string) []string {
	return append([]string{"lookup"}, k.attrs(name)...)
}

func (k *secretToolKeychain) clearArgs(name string) []string {
	return append([]string{"clear"}, k.attrs(name)...)
}

func (k *secretToolKeychain) Get(name string) (string, error) {
	if err := ValidName(name); err != nil {
		return "", err
	}
	r, err := k.run(k.path, k.lookupArgs(name), "")
	if err != nil {
		return "", err
	}
	switch {
	case r.code == 0:
		return r.stdout, nil
	case r.code == 1 && strings.TrimSpace(r.stderr) == "":
		return "", ErrNotFound // nothing has the attributes
	}
	return "", stErr(r)
}

func (k *secretToolKeychain) Set(name, secret string) error {
	if err := ValidName(name); err != nil {
		return err
	}
	if err := checkSecret(secret); err != nil {
		return err
	}
	r, err := k.run(k.path, k.storeArgs(name), secret)
	if err != nil {
		return err
	}
	if r.code != 0 {
		return stErr(r)
	}
	return nil
}

func (k *secretToolKeychain) Delete(name string) error {
	if err := ValidName(name); err != nil {
		return err
	}
	r, err := k.run(k.path, k.clearArgs(name), "")
	if err != nil {
		return err
	}
	if r.code != 0 && !(r.code == 1 && strings.TrimSpace(r.stderr) == "") {
		return stErr(r)
	}
	return nil
}

func stErr(r runResult) error {
	if l := firstLine(r.stderr); l != "" {
		return fmt.Errorf("keychain: %s", strings.TrimPrefix(l, "secret-tool: "))
	}
	return fmt.Errorf("keychain: secret-tool ended with code %d", r.code)
}
