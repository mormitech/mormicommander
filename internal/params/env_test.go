// Tests for environment variable expansion: %NAME%, $NAME, ${NAME}, extra
// TC variables, special folders, and leaving unknown names untouched.
package params

import (
	"path/filepath"
	"testing"
)

func fakeEnv(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestExpandEnv(t *testing.T) {
	o := EnvOptions{
		Lookup:  fakeEnv(map[string]string{"HOME": "/home/p", "EDITOR": "vim", "ProgramFiles(x86)": `C:\PF86`, "P": "nope"}),
		Vars:    map[string]string{"COMMANDER_PATH": "/opt/mc"},
		Percent: true, Dollar: true,
	}
	cases := []struct{ in, want string }{
		{"%HOME%/x", "/home/p/x"},
		{"$HOME/x", "/home/p/x"},
		{"${HOME}x", "/home/px"},
		{"%commander_path%/bin", "/opt/mc/bin"},
		{"%ProgramFiles(x86)%", `C:\PF86`},
		{"%P%N", "%P%N"}, // one-letter names stay placeholders
		{"%UNKNOWN% $NOPE ${NOPE}", "%UNKNOWN% $NOPE ${NOPE}"},
		{"100% sure", "100% sure"},
		{"$1 $ ${", "$1 $ ${"},
		{"%EDITOR% %HOME%", "vim /home/p"},
		{"no vars", "no vars"},
	}
	for _, c := range cases {
		if got := ExpandEnv(c.in, o); got != c.want {
			t.Errorf("ExpandEnv(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	o.Dollar = false
	if got := ExpandEnv("$HOME %HOME%", o); got != "$HOME /home/p" {
		t.Errorf("dollar disabled: %q", got)
	}
	o.Percent, o.Dollar = false, true
	if got := ExpandEnv("$HOME %HOME%", o); got != "/home/p %HOME%" {
		t.Errorf("percent disabled: %q", got)
	}
}

func TestSpecialFolders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	o := EnvOptions{Percent: true, Lookup: fakeEnv(nil)}
	if got := ExpandEnv("%$DESKTOP%", o); got != filepath.Join(home, "Desktop") {
		t.Errorf("DESKTOP = %q", got)
	}
	if got := ExpandEnv("%$personal%/x", o); got != filepath.Join(home, "Documents")+"/x" {
		t.Errorf("PERSONAL = %q", got)
	}
	if got := ExpandEnv("%$NOSUCH%", o); got != "%$NOSUCH%" {
		t.Errorf("unknown special = %q", got)
	}
	for _, n := range []string{"APPDATA", "LOCAL_APPDATA", "TEMP", "PROFILE", "DOWNLOADS", "MYMUSIC", "MYPICTURES", "MYVIDEO"} {
		if v, ok := SpecialFolder(n); !ok || v == "" {
			t.Errorf("SpecialFolder(%s) missing", n)
		}
	}
}

func TestDefaultEnvOptions(t *testing.T) {
	if o := DefaultEnvOptions(StyleWindows); !o.Percent || o.Dollar {
		t.Error("windows defaults")
	}
	if o := DefaultEnvOptions(StylePOSIX); !o.Percent || !o.Dollar {
		t.Error("posix defaults")
	}
}
