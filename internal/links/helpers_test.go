// Shared test helpers: a temp dir with its symlinks resolved (macOS puts
// t.TempDir under /var, which is itself a link to /private/var, and resolved
// paths come back in the /private form), small file writers, and a skip for
// systems where the test account may not create symbolic links (Windows
// without Developer Mode).

package links

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func realTemp(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if errors.Is(err, ErrPrivilege) || isPrivilegeErr(err) {
			t.Skip("symlinks not permitted for this account")
		}
		t.Fatal(err)
	}
}
