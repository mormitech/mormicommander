package main

import (
	"os"
	"strings"

	"mormicommander/internal/params"
)

// startOpts is what the command line asks for. The app's own flags (--demo, --control, --theme...) are read here; Total
// Commander's start-up parameters (F-728 to F-734, F-852) are read by internal/params: /L=path and /R=path (or -L= -R=,
// --left --right) for the left and right folder, up to two plain paths, /P=L or /P=R for the active panel, /T to open
// the folders in new tabs, /S to take them as source and target, /O to hand them to a window that is already open, /N for
// a new window anyway, /I= and /F= for other settings files, and /S=L (Lister), /S=C (compare) and /S=S (synchronize)
// to start a tool on its own.
//
// Left and Right are what the page opens at start. When the folders need the session first (/T: the tabs of the last
// session are there before the new ones; /S: which side is active decides which folder goes where) they stay empty and
// TCLeft/TCRight carry them; the configuration feature applies them once the page is ready (feature_config_start.go).
type startOpts struct {
	About       bool   // --about: show just the About window (the app opens it as a window of its own otherwise)
	Demo        bool   // --demo / --mock: fictional files, settings of its own, control mode on
	Theme       string // --theme light|dark, instead of the system's setting (for screenshots)
	Control     string // --control PORT ("0" = a free port); "" = off
	ControlFile string // --control-file PATH: where the address and token go
	Version     bool   // --version: print the version and quit
	Help        bool   // --help
	Left, Right string // folders for the panels
	Active      string // "L" or "R"

	TCLeft, TCRight string // the folders of /T and /S, applied by the page after start-up
	NewTab          bool   // /T
	SourceTarget    bool   // /S
	OpenInRunning   bool   // /O
	NewInstance     bool   // /N
	IniPath         string // /I= (or /INI=)
	FtpIniPath      string // /F=
	Tool            string // "lister", "compare" or "sync" (/S=L, /S=C, /S=S); "" = the file manager
	ToolOpts        string // the letters after /S=L:
	ToolFile1       string // the file(s) the tool starts with
	ToolFile2       string
}

// parseArgs reads the arguments. Unknown ones are ignored (macOS adds its own, such as -psn_0_12345, when it launches an
// app); no argument ever stops the app.
func parseArgs(args []string) startOpts {
	var o startOpts
	var rest []string // what is left for the TC parameters
	value := func(i *int, a, name string) (string, bool) { // --name=v or --name v
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v, true
		}
		if a == name && *i+1 < len(args) {
			*i++
			return args[*i], true
		}
		return "", false
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--about" || a == "-about":
			o.About = true
		case a == "--demo" || a == "-demo" || a == "--mock" || a == "-mock" || strings.HasPrefix(a, "--mock=") || strings.HasPrefix(a, "--demo="):
			o.Demo = true
		case a == "--version" || a == "-version" || a == "-v":
			o.Version = true
		case a == "--help" || a == "-help" || a == "-h":
			o.Help = true
		case strings.HasPrefix(a, "--theme"):
			if v, ok := value(&i, a, "--theme"); ok {
				o.Theme = v
			}
		case strings.HasPrefix(a, "--control-file"):
			if v, ok := value(&i, a, "--control-file"); ok {
				o.ControlFile = v
			}
		case a == "--control" || a == "-control":
			o.Control = "0"
			if i+1 < len(args) && args[i+1] != "" && isPort(args[i+1]) {
				i++
				o.Control = args[i]
			}
		case strings.HasPrefix(a, "--control="):
			o.Control = strings.TrimPrefix(a, "--control=")
		case a == "--left" || a == "--right": // the "--left FOLDER" spelling; "--left=FOLDER" is a TC parameter
			if i+1 < len(args) {
				i++
				rest = append(rest, a+"="+args[i])
			}
		case strings.HasPrefix(a, "-psn_"): // macOS process serial number
		default:
			rest = append(rest, a)
		}
	}
	s := parseStartupLenient(rest)
	o.NewTab, o.SourceTarget, o.OpenInRunning, o.NewInstance = s.NewTab, s.SourceTarget, s.OpenInRunning, s.NewInstance
	o.IniPath, o.FtpIniPath = s.IniPath, s.FtpIniPath
	if s.ActivePanel != 0 {
		o.Active = string(s.ActivePanel)
	}
	switch s.Tool {
	case params.ToolLister:
		o.Tool, o.ToolOpts = "lister", s.Lister.Raw
	case params.ToolCompare:
		o.Tool = "compare"
	case params.ToolSync:
		o.Tool = "sync"
	}
	if o.Tool != "" {
		if len(s.Paths) > 0 {
			o.ToolFile1 = s.Paths[0]
		}
		if len(s.Paths) > 1 {
			o.ToolFile2 = s.Paths[1]
		}
	}
	left, right := s.Panels(false)
	if o.Tool != "" {
		left, right = s.Left, s.Right
	}
	if o.NewTab || o.SourceTarget || isFile(left) || isFile(right) {
		o.TCLeft, o.TCRight = left, right // a file opens its folder with the cursor on it, after start-up
	} else {
		o.Left, o.Right = left, right
	}
	if o.Demo && o.Control == "" {
		o.Control = "0" // a demo run is driven from outside (screenshots), like mormiView's
	}
	return o
}

// parseStartupLenient reads the TC parameters; a parameter with a mistake (/P=X, /S=Q) is left out instead of stopping
// the app, and flags that are neither ours nor TC's are ignored.
func parseStartupLenient(args []string) params.Startup {
	s, err := params.ParseStartup(args)
	if err == nil {
		return s
	}
	var ok []string
	for _, a := range args {
		if _, err := params.ParseStartup([]string{a}); err == nil {
			ok = append(ok, a)
		}
	}
	s, _ = params.ParseStartup(ok)
	return s
}

// isFile says whether a start-up path names an existing file (not a folder).
func isFile(p string) bool {
	if p == "" || strings.Contains(p, "://") {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func isPort(s string) bool {
	if len(s) == 0 || len(s) > 5 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

const usage = `mormiCommander %s – a two-panel file manager.

Usage: mormicommander [options] [left folder] [right folder]

  -L=FOLDER, --left FOLDER     folder of the left panel
  -R=FOLDER, --right FOLDER    folder of the right panel
  -P=L, -P=R                   which panel is active
  -T                           open the folders in new tabs
  -S                           the folders are source and target, not left and right
  -O                           open the folders in the window that is already open
  -N                           always open a new window
  -I=FILE, -F=FILE             use this settings file / this FTP settings file
  -S=L FILE                    open only the viewer for FILE (-S=C: compare two files, -S=S: synchronize two folders)
  --demo                       show fictional files (for screenshots); your settings are not used or changed
  --control PORT               let a script drive the app (0 picks a free port); see docs/control.md
  --control-file PATH          where control mode writes its address and token
  --theme light|dark           use this theme instead of the system's
  --version                    print the version
`
