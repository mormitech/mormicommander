// Shared test helpers: a Launcher that records commands instead of running them, pretending to be any OS.
package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

type recorder struct {
	cmds []Command
}

// fakeLauncher returns a Launcher for osName whose Start records commands. env is its whole environment; programs
// in path are the only ones LookPath finds.
func fakeLauncher(t *testing.T, osName string, env map[string]string, path ...string) (*Launcher, *recorder) {
	t.Helper()
	rec := &recorder{}
	found := map[string]bool{}
	for _, p := range path {
		found[p] = true
	}
	l := &Launcher{
		OS:      osName,
		TempDir: t.TempDir(),
		Start: func(c Command) error {
			rec.cmds = append(rec.cmds, c)
			return nil
		},
		LookPath: func(name string) (string, error) {
			if found[name] {
				return "/usr/bin/" + name, nil
			}
			return "", exec.ErrNotFound
		},
		LookupEnv: func(k string) (string, bool) {
			v, ok := env[k]
			return v, ok
		},
	}
	return l, rec
}

func (r *recorder) last(t *testing.T) Command {
	t.Helper()
	if len(r.cmds) == 0 {
		t.Fatal("no command was started")
	}
	return r.cmds[len(r.cmds)-1]
}

// sameCmd compares the parts of a command a test cares about.
func sameCmd(t *testing.T, got, want Command) {
	t.Helper()
	if got.Path != want.Path || got.Params != want.Params || !reflect.DeepEqual(nilIfEmpty(got.Args), nilIfEmpty(want.Args)) ||
		got.Dir != want.Dir || got.NewConsole != want.NewConsole || got.Show != want.Show || got.Verb != want.Verb {
		t.Errorf("command mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
