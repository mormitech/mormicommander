// Environment variable expansion. Total Commander expands %NAME% in button
// commands, parameters, start paths and the command-line box, and adds its
// own variables (%COMMANDER_PATH%, %COMMANDER_INI%, %COMMANDER_EXE%) plus
// special folders written as %$DESKTOP% or %$APPDATA%. Mac and Linux users
// expect $HOME and ${HOME} as well, so both forms are supported. Unknown
// names are left exactly as typed, which keeps literal percent signs and
// dollar signs in commands intact. One-letter %X% names are never treated as
// variables, because "%P%N" must stay two placeholders.
package params

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvOptions controls ExpandEnv.
type EnvOptions struct {
	// Lookup finds an environment variable; nil means os.LookupEnv.
	Lookup func(name string) (string, bool)
	// Vars are extra variables checked before the environment, compared
	// case-insensitively (COMMANDER_PATH and the like).
	Vars map[string]string
	// Percent enables %NAME% (and %$SPECIAL%).
	Percent bool
	// Dollar enables $NAME and ${NAME}.
	Dollar bool
}

// DefaultEnvOptions enables %NAME% everywhere and $NAME on POSIX systems.
func DefaultEnvOptions(st Style) EnvOptions {
	return EnvOptions{Percent: true, Dollar: st.orDefault() == StylePOSIX}
}

// lookup resolves a variable name (without $ prefix for special folders).
func (o EnvOptions) lookup(name string) (string, bool) {
	if strings.HasPrefix(name, "$") {
		return SpecialFolder(name[1:])
	}
	for k, v := range o.Vars {
		if strings.EqualFold(k, name) {
			return v, true
		}
	}
	if o.Lookup != nil {
		return o.Lookup(name)
	}
	return os.LookupEnv(name)
}

func isNameChar(c byte, percent bool) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		percent && (c == '(' || c == ')')
}

// percentVar tries to read %NAME% or %$NAME% at s[i] (s[i] == '%'). It
// returns the value and the index after the closing '%'.
func (o EnvOptions) percentVar(s string, i int) (string, int, bool) {
	j := i + 1
	if j < len(s) && s[j] == '$' {
		j++
	}
	k := j
	for k < len(s) && isNameChar(s[k], true) {
		k++
	}
	if k >= len(s) || s[k] != '%' || k-j < 2 && s[i+1] != '$' || k == j {
		return "", 0, false
	}
	v, ok := o.lookup(s[i+1 : k])
	if !ok {
		return "", 0, false
	}
	return v, k + 1, true
}

// dollarVar tries to read $NAME or ${NAME} at s[i] (s[i] == '$').
func (o EnvOptions) dollarVar(s string, i int) (string, int, bool) {
	if i+1 < len(s) && s[i+1] == '{' {
		end := strings.IndexByte(s[i+2:], '}')
		if end <= 0 {
			return "", 0, false
		}
		v, ok := o.lookup(s[i+2 : i+2+end])
		return v, i + 3 + end, ok
	}
	k := i + 1
	for k < len(s) && isNameChar(s[k], false) {
		k++
	}
	if k == i+1 || s[i+1] >= '0' && s[i+1] <= '9' {
		return "", 0, false
	}
	v, ok := o.lookup(s[i+1 : k])
	return v, k, ok
}

// ExpandEnv replaces known variables in s and leaves everything else.
func ExpandEnv(s string, o EnvOptions) string {
	if !strings.ContainsAny(s, "%$") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		if c == '%' && o.Percent {
			if v, next, ok := o.percentVar(s, i); ok {
				b.WriteString(v)
				i = next
				continue
			}
		}
		if c == '$' && o.Dollar {
			if v, next, ok := o.dollarVar(s, i); ok {
				b.WriteString(v)
				i = next
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// SpecialFolder returns the folder for a Total Commander special-folder
// name as used in %$NAME% (DESKTOP, PERSONAL, APPDATA, ...). The names are
// the Windows CSIDL names TC accepts; on macOS and Linux they map to the
// matching user folders.
func SpecialFolder(name string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	switch strings.ToUpper(name) {
	case "DESKTOP", "DESKTOPDIRECTORY":
		return filepath.Join(home, "Desktop"), true
	case "PERSONAL", "MYDOCUMENTS", "DOCUMENTS":
		return filepath.Join(home, "Documents"), true
	case "DOWNLOADS":
		return filepath.Join(home, "Downloads"), true
	case "MYMUSIC", "MUSIC":
		return filepath.Join(home, "Music"), true
	case "MYPICTURES", "PICTURES":
		return filepath.Join(home, "Pictures"), true
	case "MYVIDEO", "VIDEOS":
		return filepath.Join(home, "Videos"), true
	case "PROFILE", "HOME":
		return home, true
	case "APPDATA":
		d, err := os.UserConfigDir()
		return d, err == nil
	case "LOCAL_APPDATA":
		d, err := os.UserCacheDir()
		return d, err == nil
	case "TEMP", "TMP":
		return os.TempDir(), true
	}
	return "", false
}
