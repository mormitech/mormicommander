// Where the settings files live. By default the main settings file sits in
// the per-user config folder of the platform (os.UserConfigDir()/mormicommander,
// the configdir.go convention of Péter's other apps). Like Total Commander we
// also support a portable install: if a writable settings file is found next
// to the program it is used instead, so the whole app can live on a USB stick
// (F-444). And like TC's /i= and /f= switches, the command line can name any
// settings file or FTP settings file explicitly (F-445). Locate applies these
// rules in that order: command line first, then portable, then the user
// folder.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// File and folder names.
const (
	AppDirName = "mormicommander"
	IniName    = "mormicommander.ini"
	FtpIniName = "mormicommander_ftp.ini"
)

// LocateOptions are the inputs of Locate. Empty fields are filled from the
// running program (ExeDir) and the operating system (UserConfigDir).
type LocateOptions struct {
	ExeDir        string // folder of the program, for portable mode
	UserConfigDir string // os.UserConfigDir() or a test folder
	IniOverride   string // from -i= / /i= (file or folder)
	FtpOverride   string // from -f= / /f= (file or folder)
}

// Locations is the result of Locate.
type Locations struct {
	Dir      string // folder that holds Ini (and button bars, tab files...)
	Ini      string // main settings file
	FtpIni   string // FTP connection settings file
	Portable bool   // Ini is next to the program
	Override bool   // Ini came from the command line
}

// Locate decides which settings files to use (see the file comment).
func Locate(o LocateOptions) (Locations, error) {
	var loc Locations
	if o.ExeDir == "" {
		if exe, err := os.Executable(); err == nil {
			o.ExeDir = filepath.Dir(exe)
		}
	}
	if o.UserConfigDir == "" {
		d, err := os.UserConfigDir()
		if err != nil && o.IniOverride == "" {
			return loc, err
		}
		o.UserConfigDir = d
	}
	switch {
	case o.IniOverride != "":
		p, err := resolveOverride(o.IniOverride, IniName)
		if err != nil {
			return loc, err
		}
		loc.Ini, loc.Override = p, true
	case o.ExeDir != "" && isWritableFile(filepath.Join(o.ExeDir, IniName)):
		loc.Ini, loc.Portable = filepath.Join(o.ExeDir, IniName), true
	default:
		loc.Ini = filepath.Join(o.UserConfigDir, AppDirName, IniName)
	}
	loc.Dir = filepath.Dir(loc.Ini)
	if o.FtpOverride != "" {
		p, err := resolveOverride(o.FtpOverride, FtpIniName)
		if err != nil {
			return loc, err
		}
		loc.FtpIni = p
	} else {
		loc.FtpIni = filepath.Join(loc.Dir, FtpIniName)
	}
	return loc, nil
}

func resolveOverride(p, name string) (string, error) {
	p = strings.Trim(p, `"`)
	p = Vars{}.Expand(p)
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err == nil && st.IsDir() {
		abs = filepath.Join(abs, name)
	}
	return abs, nil
}

// isWritableFile reports whether path is an existing regular file that we
// may write and whose folder accepts the temporary file of an atomic save.
func isWritableFile(path string) bool {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return false
	}
	f.Close()
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mc-probe-*")
	if err != nil {
		return false
	}
	tmp.Close()
	os.Remove(tmp.Name())
	return true
}

// ParseConfigArgs takes the settings switches out of a command line: "-i=",
// "/i=", "--ini=" name the settings file, "-f=", "/f=", "--ftpini=" the FTP
// file (TC parameters, F-445). Other arguments are returned unchanged.
func ParseConfigArgs(args []string) (ini, ftp string, rest []string) {
	for _, a := range args {
		low := strings.ToLower(a)
		switch {
		case hasAnyPrefix(low, "-i=", "/i=", "--ini="):
			ini = a[strings.Index(a, "=")+1:]
		case hasAnyPrefix(low, "-f=", "/f=", "--ftpini="):
			ftp = a[strings.Index(a, "=")+1:]
		default:
			rest = append(rest, a)
		}
	}
	return ini, ftp, rest
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// ErrNoConfig is returned by helpers that need an existing settings file.
var ErrNoConfig = errors.New("config: settings file not found")
