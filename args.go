package main

import (
	"strings"
)

// startOpts is what the command line asks for. Total Commander's own parameters work too (F-728): /L=path and /R=path
// (or -L= -R=) for the left and right folder, /P=L or /P=R for the active panel, and up to two plain paths (the first
// for the left panel, the second for the right).
type startOpts struct {
	About       bool   // show the About window (the app starts itself with this; one window per process)
	Demo        bool   // --demo / --mock: fictional files, settings of its own, control mode on
	Theme       string // --theme light|dark, instead of the system's setting (for screenshots)
	Control     string // --control PORT ("0" = a free port); "" = off
	ControlFile string // --control-file PATH: where the address and token go
	Version     bool   // --version: print the version and quit
	Help        bool   // --help
	Left, Right string // folders for the panels
	Active      string // "L" or "R"
}

// parseArgs reads the arguments. Unknown ones are ignored (macOS adds its own, such as -psn_0_12345, when it launches an
// app); no argument ever stops the app.
func parseArgs(args []string) startOpts {
	var o startOpts
	var plain []string
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
		low := strings.ToLower(a)
		// TC style: /L=, -L=, --left=
		if len(a) > 3 && (a[0] == '/' || a[0] == '-') && a[2] == '=' && (a[1] == 'L' || a[1] == 'l' || a[1] == 'R' || a[1] == 'r' || a[1] == 'P' || a[1] == 'p') {
			v := a[3:]
			switch low[1] {
			case 'l':
				o.Left = v
			case 'r':
				o.Right = v
			case 'p':
				o.Active = strings.ToUpper(v)
			}
			continue
		}
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
		case strings.HasPrefix(a, "--left"):
			if v, ok := value(&i, a, "--left"); ok {
				o.Left = v
			}
		case strings.HasPrefix(a, "--right"):
			if v, ok := value(&i, a, "--right"); ok {
				o.Right = v
			}
		case strings.HasPrefix(a, "-psn_"): // macOS process serial number
		case strings.HasPrefix(a, "-"):
		default:
			plain = append(plain, a)
		}
	}
	if len(plain) > 0 && o.Left == "" {
		o.Left = plain[0]
	}
	if len(plain) > 1 && o.Right == "" {
		o.Right = plain[1]
	}
	if o.Active != "L" && o.Active != "R" {
		o.Active = ""
	}
	if o.Demo && o.Control == "" {
		o.Control = "0" // a demo run is driven from outside (screenshots), like mormiView's
	}
	return o
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
  --demo                       show fictional files (for screenshots); your settings are not used or changed
  --control PORT               let a script drive the app (0 picks a free port); see docs/control.md
  --control-file PATH          where control mode writes its address and token
  --theme light|dark           use this theme instead of the system's
  --version                    print the version
`
