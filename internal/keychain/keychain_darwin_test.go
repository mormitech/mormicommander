package keychain

// The one test that runs /usr/bin/security for real. It never touches the user's login keychain: it makes a keychain
// file of its own in a temp folder (create-keychain does not add it to the search list), points every command at
// that file, and deletes it at the end. It still changes the Mac's keychain state while it runs, and a locked or
// missing keychain file can make macOS show a dialog or fall back to the default keychain, so it only runs when asked
// for: MC_KEYCHAIN_INTEGRATION=1 go test ./internal/keychain -run Security.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSecurityThrowawayKeychain(t *testing.T) {
	if os.Getenv("MC_KEYCHAIN_INTEGRATION") != "1" {
		t.Skip("set MC_KEYCHAIN_INTEGRATION=1 to run security against a throwaway keychain file")
	}
	path := filepath.Join(t.TempDir(), "mc-test.keychain-db")
	pw := make([]byte, 16)
	rand.Read(pw)
	if out, err := exec.Command(macSecurity, "create-keychain", "-p", hex.EncodeToString(pw), path).CombinedOutput(); err != nil {
		t.Fatalf("create-keychain: %v %s", err, out)
	}
	t.Cleanup(func() { exec.Command(macSecurity, "delete-keychain", path).Run() })
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the throwaway keychain is not there, so nothing is run: %v", err)
	}
	k := &macKeychain{service: "mormiCommander-test", path: path, run: execRunner}
	name := "site.test" + hex.EncodeToString(pw[:4])
	if _, err := k.Get(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("new keychain: %v", err)
	}
	for _, secret := range []string{"plain", `pä ss"w\ord with spaces`, "line\nbreak"} {
		if err := k.Set(name, secret); err != nil {
			t.Fatal(err)
		}
		if got, err := k.Get(name); err != nil || got != secret {
			t.Fatalf("read back %q %v, want %q", got, err, secret)
		}
	}
	if err := k.Delete(name); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Get(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := k.Delete(name); err != nil {
		t.Fatalf("deleting again: %v", err)
	}
}
