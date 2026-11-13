// Variable expansion in configured paths and commands. Total Commander lets
// hotlist entries, buttons, start menu items and settings use %COMMANDER_PATH%
// (the program folder), %COMMANDER_INI% (the settings file) and any
// environment variable written as %NAME%, so that a portable install on a USB
// stick works whatever drive letter it gets. Mac and Linux users write $HOME
// or ${HOME} and ~ instead, and a hotlist shared between machines should work
// on every platform (F-849), so all of these forms are understood everywhere.
// Unknown variables are left in the text unchanged, exactly like TC does, so
// "\\server\c$\" or a literal 100% survive. Contract does the reverse when
// saving: it turns an absolute path below the program folder or home folder
// back into the variable form so the stored setting stays portable.

package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Well-known variable names.
const (
	VarCommanderPath = "COMMANDER_PATH"
	VarCommanderIni  = "COMMANDER_INI"
	VarCommanderExe  = "COMMANDER_EXE"
)

// Vars holds the values of the commander variables. Lookups are
// case-insensitive. Names not found here fall back to the environment.
type Vars map[string]string

// Get looks a variable up: first in v (case-insensitive), then in the
// environment. HOME also works on Windows (it maps to the user profile).
func (v Vars) Get(name string) (string, bool) {
	for k, val := range v {
		if strings.EqualFold(k, name) {
			return val, true
		}
	}
	if val, ok := os.LookupEnv(name); ok {
		return val, true
	}
	if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
		if h, err := os.UserHomeDir(); err == nil {
			return h, true
		}
	}
	return "", false
}

// Expand replaces %NAME%, $NAME, ${NAME} and a leading ~ in s. Unknown
// variables stay as written.
func (v Vars) Expand(s string) string {
	if s == "" {
		return s
	}
	if s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) {
		if h, ok := v.Get("HOME"); ok {
			s = h + s[1:]
		}
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		switch c {
		case '%':
			if j := strings.IndexByte(s[i+1:], '%'); j > 0 {
				name := s[i+1 : i+1+j]
				if isVarName(name) {
					if val, ok := v.Get(name); ok {
						b.WriteString(val)
						i += j + 2
						continue
					}
				}
			}
		case '$':
			if i+1 < len(s) && s[i+1] == '{' {
				if j := strings.IndexByte(s[i+2:], '}'); j > 0 {
					name := s[i+2 : i+2+j]
					if val, ok := v.Get(name); ok && isVarName(name) {
						b.WriteString(val)
						i += j + 3
						continue
					}
				}
			} else {
				j := i + 1
				for j < len(s) && isVarChar(s[j], j == i+1) {
					j++
				}
				if j > i+1 {
					if val, ok := v.Get(s[i+1 : j]); ok {
						b.WriteString(val)
						i = j
						continue
					}
				}
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

func isVarName(n string) bool {
	if n == "" {
		return false
	}
	for i := 0; i < len(n); i++ {
		c := n[i]
		if !(isVarChar(c, i == 0) || c == '(' || c == ')' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

func isVarChar(c byte, first bool) bool {
	if c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
		return true
	}
	return !first && c >= '0' && c <= '9'
}

// Contract rewrites an absolute path that lies below COMMANDER_PATH as
// %COMMANDER_PATH%..., or below the home folder as $HOME... (on Windows
// %USERPROFILE%...). Other text is returned unchanged.
func (v Vars) Contract(p string) string {
	if p == "" {
		return p
	}
	if cp, ok := v[VarCommanderPath]; ok && cp != "" {
		if rest, ok := cutPathPrefix(p, cp); ok {
			return "%" + VarCommanderPath + "%" + rest
		}
	}
	if h, ok := v.Get("HOME"); ok && h != "" {
		if rest, ok := cutPathPrefix(p, h); ok {
			return homeVarSpelling + rest
		}
	}
	return p
}

// cutPathPrefix reports whether p is prefix or a path below it and returns
// the remainder (starting with a separator, or empty).
func cutPathPrefix(p, prefix string) (string, bool) {
	prefix = strings.TrimRight(prefix, `/\`)
	if prefix == "" || len(p) < len(prefix) || !pathsEqual(p[:len(prefix)], prefix) {
		return "", false
	}
	rest := p[len(prefix):]
	if rest == "" || rest[0] == '/' || rest[0] == '\\' {
		return rest, true
	}
	return "", false
}

// DefaultVars builds the commander variables for a program folder and ini
// path; empty arguments are skipped.
func DefaultVars(programDir, iniPath string) Vars {
	v := Vars{}
	if programDir != "" {
		v[VarCommanderPath] = filepath.Clean(programDir)
	}
	if iniPath != "" {
		v[VarCommanderIni] = iniPath
	}
	if exe, err := os.Executable(); err == nil {
		v[VarCommanderExe] = exe
	}
	return v
}
