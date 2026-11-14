package main

// Saved passwords in the system keychain (F-634, board issue #10): the option "Keep saved passwords in the system
// keychain" of the connection manager. It is off by default, so passwords stay sealed with the master password as
// before (feature_net_sites.go). When it is on, a password saved or changed goes to the system keychain (the login
// keychain on macOS, the Credential Manager on Windows, the Secret Service on Linux; internal/keychain) under the name
// "site.<id of the connection>", and the list in ftpsites.json keeps only the reference "keychain:site.<id>", never
// the password. The keychain is unlocked by the user's login, so these passwords need no master password.
//
// Switching the option moves the passwords already saved: on, each one sealed with the master password is opened
// (the master password is asked once, as for connecting) and written to the keychain; off, each one in the keychain is
// read back and sealed with the master password (asked, or set up when there is none yet), and the keychain items are
// deleted. Either way the list is only written once every password has been moved, so a failure halfway leaves the
// passwords where they were. Which store a saved password is in is told by its own form ("enc:v1:" or "keychain:"),
// not by the option, so a list restored from a backup still works.
//
// A password the keychain no longer has (deleted in Keychain Access, another computer) is asked when connecting, as
// for a connection without a saved password; the question says why.

import (
	"errors"
	"strings"

	"mormicommander/internal/ftp"
	"mormicommander/internal/keychain"
)

// keychainNameOf is the keychain name of a saved connection's password.
func keychainNameOf(siteID string) string { return "site." + siteID }

// keychainRefName is the keychain name in a stored reference, "" when v is not one.
func keychainRefName(v string) string {
	name, ok := strings.CutPrefix(v, ftp.KeyRefPrefix)
	if !ok {
		return ""
	}
	return name
}

// isMasterSealed says whether a stored password is sealed with the master password.
func isMasterSealed(v string) bool { return v != "" && ftp.IsSealed(v) && keychainRefName(v) == "" }

// keychainBox is the SecretBox of one connection's password in the keychain: Seal writes the password to the
// keychain and returns the reference the list keeps; Open reads the password the reference names.
type keychainBox struct {
	kc   keychain.Keychain
	name string
}

func (b keychainBox) Seal(plain string) (string, error) {
	if err := b.kc.Set(b.name, plain); err != nil {
		return "", errors.New("the password can't be saved in the system keychain: " + kcMsg(err))
	}
	return ftp.KeyRefPrefix + b.name, nil
}

func (b keychainBox) Open(sealed string) (string, error) {
	name := keychainRefName(sealed)
	if name == "" {
		return "", errors.New("ftp: value is not a keychain reference")
	}
	return b.kc.Get(name)
}

// kcMsg is a keychain error without the package's prefix.
func kcMsg(err error) string { return strings.TrimPrefix(err.Error(), "keychain: ") }

// keychain returns the system keychain, opened once per run; the error says why there is none.
func (n *Net) keychain() (keychain.Keychain, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.kcOpened {
		open := n.openKeychain
		if open == nil {
			open = keychain.Open
		}
		n.kc, n.kcErr = open()
		n.kcOpened = true
	}
	return n.kc, n.kcErr
}

// boxForSite is the box a new or changed password of the site is saved with: the keychain when the option is on,
// else the master password (asked, or set up first).
func (n *Net) boxForSite(st *ftp.Store) (func(id string) ftp.SecretBox, error) {
	n.mu.Lock()
	on := st.Keychain
	n.mu.Unlock()
	if on {
		kc, err := n.keychain()
		if err != nil {
			return nil, errors.New("The password can't be saved in the system keychain: " + err.Error())
		}
		return func(id string) ftp.SecretBox { return keychainBox{kc, keychainNameOf(id)} }, nil
	}
	box, err := n.boxForSaving()
	if err != nil {
		return nil, err
	}
	return func(string) ftp.SecretBox { return box }, nil
}

// sitePassword reads a saved connection's password. One in the keychain that can't be read is "" (asked when
// connecting), with missing set; one sealed with the master password needs the master password.
func (n *Net) sitePassword(st *ftp.Store, site ftp.Site) (pw string, missing bool, err error) {
	if name := keychainRefName(site.Password); name != "" {
		kc, err := n.keychain()
		if err != nil {
			return "", true, nil
		}
		pw, err := kc.Get(name)
		if err != nil {
			return "", true, nil
		}
		return pw, false, nil
	}
	box, err := n.unlockIfNeeded()
	if err != nil {
		return "", false, err
	}
	pw, err = st.Password(site.ID, box)
	if err != nil {
		return "", false, netStoreErr(err)
	}
	return pw, false, nil
}

// forgetKeychain deletes the keychain items of stored references (a connection was deleted, or its password was
// moved or cleared). A failure leaves an item nobody refers to, which is harmless, so it is not reported.
func (n *Net) forgetKeychain(refs ...string) {
	var names []string
	for _, r := range refs {
		if name := keychainRefName(r); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return
	}
	kc, err := n.keychain()
	if err != nil {
		return
	}
	for _, name := range names {
		kc.Delete(name)
	}
}

// NetKeychainResult says what switching the keychain option did.
type NetKeychainResult struct {
	Keychain bool `json:"keychain"` // the option is now on
	Moved    int  `json:"moved"`    // passwords moved to the other store
	Missing  int  `json:"missing"`  // passwords the keychain no longer had (now asked when connecting)
}

// SetKeychain switches "Keep saved passwords in the system keychain" and moves the saved passwords.
func (n *Net) SetKeychain(on bool) (NetKeychainResult, error) {
	st, err := n.loadStore()
	if err != nil {
		return NetKeychainResult{}, err
	}
	if on {
		return n.toKeychain(st)
	}
	return n.fromKeychain(st)
}

// toKeychain moves the passwords sealed with the master password into the keychain.
func (n *Net) toKeychain(st *ftp.Store) (NetKeychainResult, error) {
	kc, err := n.keychain()
	if err != nil {
		return NetKeychainResult{}, err
	}
	n.mu.Lock()
	var ids []string
	for _, s := range st.Sites {
		if isMasterSealed(s.Password) {
			ids = append(ids, s.ID)
		}
	}
	n.mu.Unlock()
	var box ftp.SecretBox
	if len(ids) > 0 {
		if box, err = n.unlockIfNeeded(); err != nil {
			return NetKeychainResult{}, err
		}
		if box == nil {
			return NetKeychainResult{}, netStoreErr(ftp.ErrLocked)
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	refs := map[string]string{} // site ID → reference
	undo := func() {
		for id := range refs {
			kc.Delete(keychainNameOf(id))
		}
	}
	for _, id := range ids {
		i := netSiteIndex(st, id)
		if i < 0 {
			continue
		}
		plain, err := box.Open(st.Sites[i].Password)
		if err != nil {
			undo()
			return NetKeychainResult{}, netStoreErr(err)
		}
		ref, err := (keychainBox{kc, keychainNameOf(id)}).Seal(plain)
		if err != nil {
			undo()
			return NetKeychainResult{}, netStoreErr(err)
		}
		refs[id] = ref
	}
	old := map[string]string{}
	for id, ref := range refs {
		i := netSiteIndex(st, id)
		old[id] = st.Sites[i].Password
		st.Sites[i].Password = ref
	}
	st.Keychain = true
	if err := n.saveStoreLocked(); err != nil {
		for id, v := range old {
			st.Sites[netSiteIndex(st, id)].Password = v
		}
		st.Keychain = false
		undo()
		return NetKeychainResult{}, err
	}
	return NetKeychainResult{Keychain: true, Moved: len(refs)}, nil
}

// fromKeychain moves the passwords in the keychain back to the master password and deletes the keychain items.
func (n *Net) fromKeychain(st *ftp.Store) (NetKeychainResult, error) {
	n.mu.Lock()
	type moved struct{ id, ref, plain string }
	var items []moved
	for _, s := range st.Sites {
		if keychainRefName(s.Password) != "" {
			items = append(items, moved{id: s.ID, ref: s.Password})
		}
	}
	n.mu.Unlock()
	res := NetKeychainResult{}
	found := 0
	if len(items) > 0 {
		kc, err := n.keychain()
		if err != nil {
			return res, errors.New("The passwords in the system keychain can't be read: " + err.Error())
		}
		for i := range items {
			pw, err := kc.Get(keychainRefName(items[i].ref))
			switch {
			case errors.Is(err, keychain.ErrNotFound):
				res.Missing++
			case err != nil:
				return NetKeychainResult{Keychain: true}, errors.New("The passwords in the system keychain can't be read: " + kcMsg(err))
			default:
				items[i].plain = pw
				found++
			}
		}
	}
	var box ftp.SecretBox
	if found > 0 {
		var err error
		if box, err = n.boxForSaving(); err != nil {
			return NetKeychainResult{Keychain: true}, err
		}
	}
	n.mu.Lock()
	sealed := map[string]string{}
	for _, it := range items {
		if it.plain == "" {
			sealed[it.id] = "" // not in the keychain any more: asked when connecting
			continue
		}
		v, err := box.Seal(it.plain)
		if err != nil {
			n.mu.Unlock()
			return NetKeychainResult{Keychain: true}, netStoreErr(err)
		}
		sealed[it.id] = v
	}
	old := map[string]string{}
	for id, v := range sealed {
		if i := netSiteIndex(st, id); i >= 0 {
			old[id] = st.Sites[i].Password
			st.Sites[i].Password = v
		}
	}
	st.Keychain = false
	if err := n.saveStoreLocked(); err != nil {
		for id, v := range old {
			st.Sites[netSiteIndex(st, id)].Password = v
		}
		st.Keychain = true
		n.mu.Unlock()
		return NetKeychainResult{Keychain: true}, err
	}
	n.mu.Unlock()
	refs := make([]string, 0, len(items))
	for _, it := range items {
		refs = append(refs, it.ref)
	}
	n.forgetKeychain(refs...)
	res.Moved = found
	return res, nil
}
