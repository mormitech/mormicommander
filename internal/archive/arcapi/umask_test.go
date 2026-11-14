// Tests that the umask read at start-up is the one the operating system
// really applies: a file created with mode 0666 must end up with exactly the
// bits ApplyUmask leaves. Runs in t.TempDir().
package arcapi

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUmaskMatchesSystem(t *testing.T) {
	if runtime.GOOS == "windows" {
		if Umask() != 0 {
			t.Fatalf("umask %o on Windows", Umask())
		}
		return
	}
	p := filepath.Join(t.TempDir(), "f")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fi.Mode().Perm(), ApplyUmask(0o666); got != want {
		t.Errorf("created file has %o, ApplyUmask(0666) = %o", got, want)
	}
	if ApplyUmask(os.ModeDir|0o777)&os.ModeDir == 0 {
		t.Error("type bits dropped")
	}
}
