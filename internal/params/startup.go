// Start-up parameters, compatible with Total Commander's wincmd.exe. Users
// and scripts start TC with things like "wincmd /O /T /L=c:\a /R=c:\b" and
// expect the same from mormiCommander: up to two positional folders (left
// and right), /L= and /R= to name a side, /O to hand the folders to an
// already running window instead of opening a new one, /N to force a new
// window, /T to open them in new tabs, /S to read the two folders as source
// and target instead of left and right, /P=L or /P=R to choose the active
// panel, /I= (or /INI=) and /F= for other settings files, and /S=L to open
// only the Lister for a file (we add /S=C for compare and /S=S for
// synchronise). Switches work with "/", "-" or "--" and in any case. On Mac
// and Linux a "/" also starts every absolute path, so a "/" token only
// counts as a switch when it is exactly one of the known switches; a folder
// literally named "/O" can still be opened as "/O/". Anything starting with
// "-" that is not a TC switch (--control, --theme, --version...) is kept in
// Unknown for the app's own flag parser.
package params

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Tool is a standalone tool started instead of the main window.
type Tool int

const (
	// ToolNone starts the normal file manager window.
	ToolNone Tool = iota
	// ToolLister opens only the Lister (/S=L).
	ToolLister
	// ToolCompare opens only Compare by content (/S=C, ours).
	ToolCompare
	// ToolSync opens only Synchronize folders (/S=S, ours).
	ToolSync
)

// ListerOptions are the letters after "/S=L:" (TC syntax).
type ListerOptions struct {
	Raw string
	// Mode is the view mode: 1 text, 2 binary, 3 hex, 4 multimedia,
	// 5 HTML, 6 Unicode, 7 UTF-8 (T1..T7); 0 = automatic.
	Mode int
	// Charset is 'A' (ANSI), 'S' (ASCII/DOS) or 'V' (variable width);
	// 0 = default.
	Charset byte
	// Codepage is the number after C, 0 = none.
	Codepage int
	// AutoDetect is N (detect the encoding).
	AutoDetect bool
	// Plugin is the text after P (forces a lister plugin by name).
	Plugin string
}

// Startup is the parsed start-up command line.
type Startup struct {
	Left, Right   string   // /L= and /R=
	Paths         []string // positional folders (or files for a tool)
	OpenInRunning bool     // /O
	NewInstance   bool     // /N
	NewTab        bool     // /T
	SourceTarget  bool     // /S
	ActivePanel   byte     // 'L' or 'R' from /P=, 0 = unchanged
	IniPath       string   // /I= or /INI=
	FtpIniPath    string   // /F=
	Tool          Tool     // /S=L, /S=C, /S=S
	Lister        ListerOptions
	Unknown       []string // other "-" flags, in order, with their spelling
}

// cleanArg removes a trailing quote that Windows' argument splitting
// leaves when a path ends in a backslash ("C:\My dir\" arrives as
// C:\My dir").
func cleanArg(s string) string {
	return strings.TrimSuffix(s, `"`)
}

// ParseStartup parses the program arguments (without the program name).
func ParseStartup(args []string) (Startup, error) {
	var s Startup
	onlyPaths := false
	for _, a := range args {
		if onlyPaths {
			s.Paths = append(s.Paths, cleanArg(a))
			continue
		}
		if a == "--" {
			onlyPaths = true
			continue
		}
		body, prefix := switchBody(a)
		if prefix == "" {
			s.Paths = append(s.Paths, cleanArg(a))
			continue
		}
		known, err := s.apply(body)
		if err != nil {
			return s, fmt.Errorf("%s: %w", a, err)
		}
		if known {
			continue
		}
		if prefix == "/" {
			s.Paths = append(s.Paths, cleanArg(a))
		} else {
			s.Unknown = append(s.Unknown, a)
		}
	}
	return s, nil
}

// switchBody strips "/", "--" or "-" and returns the rest and the prefix.
func switchBody(a string) (body, prefix string) {
	switch {
	case strings.HasPrefix(a, "--") && len(a) > 2:
		return a[2:], "--"
	case strings.HasPrefix(a, "-") && len(a) > 1:
		return a[1:], "-"
	case strings.HasPrefix(a, "/") && len(a) > 1:
		return a[1:], "/"
	}
	return a, ""
}

var errMissingValue = errors.New("missing value after '='")

// apply handles one switch body; known is false for an unknown switch.
func (s *Startup) apply(body string) (known bool, err error) {
	name, val, hasVal := strings.Cut(body, "=")
	up := strings.ToUpper(name)
	if !hasVal {
		switch up {
		case "O":
			s.OpenInRunning = true
		case "N":
			s.NewInstance = true
		case "T":
			s.NewTab = true
		case "S":
			s.SourceTarget = true
		default:
			return false, nil
		}
		return true, nil
	}
	val = cleanArg(val)
	switch up {
	case "L", "LEFT", "R", "RIGHT", "I", "INI", "F":
		if val == "" {
			return true, errMissingValue
		}
	}
	switch up {
	case "L", "LEFT":
		s.Left = val
	case "R", "RIGHT":
		s.Right = val
	case "I", "INI":
		s.IniPath = val
	case "F":
		s.FtpIniPath = val
	case "P":
		switch strings.ToUpper(val) {
		case "L":
			s.ActivePanel = 'L'
		case "R":
			s.ActivePanel = 'R'
		default:
			return true, fmt.Errorf("panel must be L or R, not %q", val)
		}
	case "S":
		tool, opts, _ := strings.Cut(val, ":")
		switch strings.ToUpper(tool) {
		case "L":
			s.Tool = ToolLister
			lo, err := parseListerOptions(opts)
			if err != nil {
				return true, err
			}
			s.Lister = lo
		case "C":
			s.Tool = ToolCompare
		case "S":
			s.Tool = ToolSync
		default:
			return true, fmt.Errorf("unknown tool %q (use L, C or S)", tool)
		}
	default:
		return false, nil
	}
	return true, nil
}

// parseListerOptions parses the letters after "/S=L:", e.g. "AT1C1250".
func parseListerOptions(o string) (ListerOptions, error) {
	lo := ListerOptions{Raw: o}
	num := func(i int) (int, int) {
		j := i
		for j < len(o) && o[j] >= '0' && o[j] <= '9' {
			j++
		}
		n, _ := strconv.Atoi(o[i:j])
		return n, j
	}
	for i := 0; i < len(o); {
		c := o[i] &^ 0x20 // upper case
		i++
		switch c {
		case 'A', 'S', 'V':
			lo.Charset = c
		case 'N':
			lo.AutoDetect = true
		case 'T':
			n, j := num(i)
			if j == i || n < 1 || n > 7 {
				return lo, fmt.Errorf("lister mode must be T1..T7")
			}
			lo.Mode, i = n, j
		case 'C':
			n, j := num(i)
			if j == i {
				return lo, fmt.Errorf("codepage number missing after C")
			}
			lo.Codepage, i = n, j
		case 'P':
			lo.Plugin = o[i:]
			i = len(o)
		default:
			return lo, fmt.Errorf("unknown lister option %q", o[i-1])
		}
	}
	return lo, nil
}

// Panels works out which folder goes to which side. activeIsRight is the
// side that is (or will be) active when ActivePanel is not given. An empty
// string means "leave that side as it is".
func (s Startup) Panels(activeIsRight bool) (left, right string) {
	a, b := s.Left, s.Right
	rest := s.Paths
	if s.Tool != ToolNone {
		rest = nil
	}
	if a == "" && len(rest) > 0 {
		a, rest = rest[0], rest[1:]
	}
	if b == "" && len(rest) > 0 {
		b = rest[0]
	}
	if !s.SourceTarget {
		return a, b
	}
	switch s.ActivePanel {
	case 'L':
		activeIsRight = false
	case 'R':
		activeIsRight = true
	}
	if activeIsRight {
		return b, a
	}
	return a, b
}

// Absolute returns a copy with relative folder paths made absolute against
// cwd, so they can be sent to a running instance (/O) that has a different
// working folder. Virtual paths are left alone.
func (s Startup) Absolute(cwd string, st Style) Startup {
	abs := func(p string) string {
		if p == "" || IsAbs(p, st) {
			return p
		}
		return JoinPath(cwd, p, st)
	}
	s.Left, s.Right = abs(s.Left), abs(s.Right)
	s.IniPath, s.FtpIniPath = abs(s.IniPath), abs(s.FtpIniPath)
	paths := make([]string, len(s.Paths))
	for i, p := range s.Paths {
		paths[i] = abs(p)
	}
	s.Paths = paths
	return s
}

// Args renders the settings back into an argument list in "-" form (safe
// on every system), for forwarding to a running instance. OpenInRunning,
// NewInstance and Unknown are not included.
func (s Startup) Args() []string {
	var out []string
	if s.NewTab {
		out = append(out, "-T")
	}
	if s.SourceTarget {
		out = append(out, "-S")
	}
	if s.ActivePanel != 0 {
		out = append(out, "-P="+string(s.ActivePanel))
	}
	if s.Left != "" {
		out = append(out, "-L="+s.Left)
	}
	if s.Right != "" {
		out = append(out, "-R="+s.Right)
	}
	if s.IniPath != "" {
		out = append(out, "-I="+s.IniPath)
	}
	if s.FtpIniPath != "" {
		out = append(out, "-F="+s.FtpIniPath)
	}
	switch s.Tool {
	case ToolLister:
		t := "-S=L"
		if s.Lister.Raw != "" {
			t += ":" + s.Lister.Raw
		}
		out = append(out, t)
	case ToolCompare:
		out = append(out, "-S=C")
	case ToolSync:
		out = append(out, "-S=S")
	}
	if len(s.Paths) > 0 {
		out = append(out, "--")
		out = append(out, s.Paths...)
	}
	return out
}
