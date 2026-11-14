package main

// Tests of "Keep saved passwords in the system keychain" (F-634, board issue #10) with the fake keychain of
// internal/keychain: the option is off by default and changes nothing then; on, the list keeps only a reference;
// switching moves the saved passwords both ways (asking for the master password when needed); a password the
// keychain lost is asked when connecting; copies and deletions keep the keychain tidy. The real keychain is never used.

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mormicommander/internal/ftp"
	"mormicommander/internal/keychain"
)

// kcNet is a testNet with a fake keychain the test can look into.
func kcNet(t *testing.T, answers map[string]string) (*Net, *keychain.Fake, *[]string) {
	t.Helper()
	n, asked := testNet(t, answers)
	kc := keychain.NewFake()
	n.openKeychain = func() (keychain.Keychain, error) { return kc, nil }
	return n, kc, asked
}

func storedPassword(t *testing.T, n *Net, id string) string {
	t.Helper()
	st, err := ftp.LoadStore(n.storePath)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := st.Get(id)
	if !ok {
		t.Fatalf("site %s not in the file", id)
	}
	return s.Password
}

func fileHas(t *testing.T, n *Net, s string) bool {
	t.Helper()
	b, _ := os.ReadFile(n.storePath)
	return bytes.Contains(b, []byte(s))
}

func TestNetKeychainOffByDefault(t *testing.T) {
	n, kc, _ := kcNet(t, map[string]string{"master-new": "m"})
	id, err := n.SaveSite(NetSite{Name: "A", Protocol: "ftp", Host: "h", User: "u", Password: "pw-a", SavePassword: true})
	if err != nil {
		t.Fatal(err)
	}
	if v := storedPassword(t, n, id); !strings.HasPrefix(v, "enc:v1:") {
		t.Fatalf("stored %q, want sealed with the master password", v)
	}
	if len(kc.Names()) != 0 {
		t.Fatalf("the keychain was used: %v", kc.Names())
	}
	l, _ := n.Sites()
	if l.Keychain || !l.KeychainAvailable || l.Sites[0].InKeychain {
		t.Fatalf("list %+v", l)
	}
}

// Switching on moves the master-password passwords into the keychain; the file keeps references only. New passwords
// go to the keychain without the master password; switching off moves them back.
func TestNetKeychainMigrationBothWays(t *testing.T) {
	n, kc, asked := kcNet(t, map[string]string{"master-new": "m", "master": "m"})
	a, _ := n.SaveSite(NetSite{Name: "A", Protocol: "ftp", Host: "h", User: "u", Password: "pw-a", SavePassword: true})
	b, _ := n.SaveSite(NetSite{Name: "B", Protocol: "sftp", Host: "h2", User: "u", Password: "pw-b", SavePassword: true})
	c, _ := n.SaveSite(NetSite{Name: "C", Protocol: "ftp", Host: "h3", User: "u"}) // no saved password

	// a new run: the master password is not typed yet, so switching on asks for it once
	n2 := newNet(&App{})
	n2.storePath, n2.asker = n.storePath, n.asker
	n2.openKeychain = func() (keychain.Keychain, error) { return kc, nil }
	*asked = nil
	r, err := n2.SetKeychain(true)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Keychain || r.Moved != 2 || strings.Join(*asked, ",") != "master" {
		t.Fatalf("result %+v asked %v", r, *asked)
	}
	for id, pw := range map[string]string{a: "pw-a", b: "pw-b"} {
		if v := storedPassword(t, n2, id); v != "keychain:site."+id {
			t.Fatalf("stored %q", v)
		}
		if got, _ := kc.Get("site." + id); got != pw {
			t.Fatalf("keychain has %q for %s", got, id)
		}
	}
	if storedPassword(t, n2, c) != "" || fileHas(t, n2, "pw-a") || fileHas(t, n2, "pw-b") {
		t.Fatal("the file has a password, or C got one")
	}
	l, _ := n2.Sites()
	if !l.Keychain || !l.Sites[0].InKeychain || !l.Sites[0].HasPassword {
		t.Fatalf("list %+v", l)
	}

	// a password saved now goes to the keychain only, without the master password (a run that never typed it)
	n3 := newNet(&App{})
	n3.storePath, n3.asker = n.storePath, n.asker
	n3.openKeychain = func() (keychain.Keychain, error) { return kc, nil }
	*asked = nil
	d, err := n3.SaveSite(NetSite{Name: "D", Protocol: "ftp", Host: "h4", User: "u", Password: "pw-d", SavePassword: true})
	if err != nil {
		t.Fatal(err)
	}
	if storedPassword(t, n3, d) != "keychain:site."+d || fileHas(t, n3, "pw-d") || len(*asked) != 0 {
		t.Fatalf("new password: %q asked %v", storedPassword(t, n3, d), *asked)
	}
	// reading it back needs no master password either
	st, _ := n3.loadStore()
	site, _ := st.Get(d)
	if pw, missing, err := n3.sitePassword(st, site); err != nil || missing || pw != "pw-d" || len(*asked) != 0 {
		t.Fatalf("read back %q %v %v asked %v", pw, missing, err, *asked)
	}

	// one item is gone from the keychain: switching off moves the others and counts it as missing
	kc.Delete("site." + b)
	r, err = n3.SetKeychain(false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Keychain || r.Moved != 2 || r.Missing != 1 || strings.Join(*asked, ",") != "master" {
		t.Fatalf("result %+v asked %v", r, *asked)
	}
	if len(kc.Names()) != 0 {
		t.Fatalf("keychain items left: %v", kc.Names())
	}
	for _, id := range []string{a, d} {
		if v := storedPassword(t, n3, id); !strings.HasPrefix(v, "enc:v1:") {
			t.Fatalf("stored %q after switching off", v)
		}
	}
	if storedPassword(t, n3, b) != "" {
		t.Fatal("the missing password was not cleared")
	}
	box, _ := n3.unlockIfNeeded()
	st, _ = n3.loadStore()
	if pw, _ := st.Password(d, box); pw != "pw-d" {
		t.Fatalf("moved back %q", pw)
	}
	if l, _ := n3.Sites(); l.Keychain {
		t.Fatal("still on")
	}
}

// Switching off when every password was saved in keychain mode sets up a master password first.
func TestNetKeychainOffSetsUpMasterPassword(t *testing.T) {
	n, kc, asked := kcNet(t, map[string]string{"master-new": "new"})
	if _, err := n.SetKeychain(true); err != nil || len(*asked) != 0 {
		t.Fatalf("switching on with nothing to move: %v asked %v", err, *asked)
	}
	id, _ := n.SaveSite(NetSite{Name: "A", Protocol: "ftp", Host: "h", User: "u", Password: "pw", SavePassword: true})
	if len(*asked) != 0 || len(kc.Names()) != 1 {
		t.Fatalf("asked %v keychain %v", *asked, kc.Names())
	}
	if _, err := n.SetKeychain(false); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*asked, ",") != "master-new" || !strings.HasPrefix(storedPassword(t, n, id), "enc:v1:") || len(kc.Names()) != 0 {
		t.Fatalf("asked %v stored %q keychain %v", *asked, storedPassword(t, n, id), kc.Names())
	}
}

// A keychain that refuses halfway leaves everything as it was.
func TestNetKeychainFailureChangesNothing(t *testing.T) {
	n, kc, _ := kcNet(t, map[string]string{"master-new": "m", "master": "m"})
	id, _ := n.SaveSite(NetSite{Name: "A", Protocol: "ftp", Host: "h", User: "u", Password: "pw", SavePassword: true})
	before := storedPassword(t, n, id)
	kc.Fail = errors.New("keychain: User interaction is not allowed.")
	if _, err := n.SetKeychain(true); err == nil || !strings.Contains(err.Error(), "User interaction") {
		t.Fatalf("got %v", err)
	}
	if storedPassword(t, n, id) != before {
		t.Fatal("the stored password changed")
	}
	if l, _ := n.Sites(); l.Keychain {
		t.Fatal("the option is on after a failure")
	}
	// on, a password that can't be written to the keychain is not saved at all (and never in plain text)
	kc.Fail = nil
	n.SetKeychain(true)
	kc.Fail = errors.New("keychain: no")
	if _, err := n.SaveSite(NetSite{Name: "B", Protocol: "ftp", Host: "h2", User: "u", Password: "pw-b", SavePassword: true}); err == nil {
		t.Fatal("saving worked with a broken keychain")
	}
	if fileHas(t, n, "pw-b") {
		t.Fatal("the password is in the file")
	}
}

// Without a system keychain the option is not offered, and says why.
func TestNetKeychainUnavailable(t *testing.T) {
	n, _ := testNet(t, nil)
	n.openKeychain = func() (keychain.Keychain, error) { return nil, keychain.Unavailable("Install secret-tool.") }
	l, err := n.Sites()
	if err != nil || l.KeychainAvailable || l.KeychainNote != "Install secret-tool." {
		t.Fatalf("%+v %v", l, err)
	}
	if _, err := n.SetKeychain(true); err == nil || err.Error() != "Install secret-tool." {
		t.Fatalf("switching on: %v", err)
	}
}

// A password missing from the keychain is asked when connecting, and the question says why.
func TestNetKeychainMissingPasswordIsAsked(t *testing.T) {
	host, _ := davServer(t, "bob", "right")
	var messages []string
	n, kc, _ := kcNet(t, nil)
	n.asker = func(kind string, data map[string]any) (map[string]any, bool) {
		msg, _ := data["message"].(string)
		messages = append(messages, kind+": "+msg)
		return map[string]any{"ok": true, "value": "right"}, true
	}
	n.SetKeychain(true)
	id, err := n.SaveSite(NetSite{Name: "Dav", Protocol: "webdav", Host: host, User: "bob", Password: "right", SavePassword: true})
	if err != nil {
		t.Fatal(err)
	}
	// saved: connecting asks nothing
	if _, err := n.ConnectSite(id); err != nil || len(messages) != 0 {
		t.Fatalf("connect with the saved password: %v asked %v", err, messages)
	}
	n.disconnectSession("webdav://bob@"+host, true)
	kc.Delete("site." + id)
	if _, err := n.ConnectSite(id); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0] != "password: The saved password is not in the system keychain any more." {
		t.Fatalf("asked %q", messages)
	}
	if storedPassword(t, n, id) != "keychain:site."+id {
		t.Fatal("the reference changed")
	}
}

// Copies get their own keychain item; unticking "save", deleting a connection or a folder deletes the items.
func TestNetKeychainCopyAndDelete(t *testing.T) {
	n, kc, _ := kcNet(t, nil)
	n.SetKeychain(true)
	a, _ := n.SaveSite(NetSite{Name: "A", Folder: "Work", Protocol: "ftp", Host: "h", User: "u", Password: "pw", SavePassword: true})
	cp, err := n.CopySite(a)
	if err != nil {
		t.Fatal(err)
	}
	if storedPassword(t, n, cp) != "keychain:site."+cp {
		t.Fatalf("copy refers to %q", storedPassword(t, n, cp))
	}
	if got, _ := kc.Get("site." + cp); got != "pw" {
		t.Fatalf("copy's item %q", got)
	}
	// unticking "save the password" deletes its item
	l, _ := n.Sites()
	var s NetSite
	for _, x := range l.Sites {
		if x.ID == cp {
			s = x
		}
	}
	s.SavePassword = false
	if _, err := n.SaveSite(s); err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Get("site." + cp); !errors.Is(err, keychain.ErrNotFound) {
		t.Fatal("unticked password still in the keychain")
	}
	// a changed password replaces the item of the same name
	s2 := NetSite{ID: a, Name: "A", Folder: "Work", Protocol: "ftp", Host: "h", User: "u", Password: "pw2", SavePassword: true}
	if _, err := n.SaveSite(s2); err != nil {
		t.Fatal(err)
	}
	if got, _ := kc.Get("site." + a); got != "pw2" || len(kc.Names()) != 1 {
		t.Fatalf("changed: %q %v", got, kc.Names())
	}
	b, _ := n.SaveSite(NetSite{Name: "B", Protocol: "ftp", Host: "h2", User: "u", Password: "pb", SavePassword: true})
	if err := n.DeleteSite(b); err != nil {
		t.Fatal(err)
	}
	if err := n.DeleteFolder("Work"); err != nil {
		t.Fatal(err)
	}
	if len(kc.Names()) != 0 {
		t.Fatalf("items left: %v", kc.Names())
	}
}
