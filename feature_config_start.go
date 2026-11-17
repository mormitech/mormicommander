package main

// The Start menu and the start-up parameters, Go side (FEATURES.md sections 18 and 41).
//
// The Start menu is TC's [user] section, read and written by internal/config; the page edits it as a flat list with a
// depth per entry (Change start menu, F-450) and runs its entries like buttons.
//
// Start-up: args.go reads TC's parameters with internal/params; StartParams gives the page what it applies after the
// panels are ready (/T new tabs, /S source and target, /S=L /S=C /S=S tools). /O (F-729) and the "Allow only one copy"
// option (Operation page, F-735) hand the command line to the window that is already open: the first window listens on
// 127.0.0.1 (a random port and a token, written to instance.json in the settings folder, readable only by the user); a
// second start with /O finds it, sends its arguments, and quits; the first window then shows the folders and comes to
// the front. /N always opens a new window.

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mormicommander/internal/config"
	"mormicommander/internal/params"
)

// ConfigStart is what the page calls for the Start menu and the start-up parameters (window.go.main.ConfigStart).
type ConfigStart struct{ app *App }

// configStartItem is one Start menu entry in the page's flat list.
type configStartItem struct {
	Kind   string `json:"kind"` // "item", "submenu" or "sep"
	Depth  int    `json:"depth"`
	Title  string `json:"title"`
	Cmd    string `json:"cmd"`
	Params string `json:"params"`
	Path   string `json:"path"`
	Key    string `json:"key"` // as the page writes keys ("Ctrl+Shift+F1"); "" = none
}

// Menu returns the Start menu as a flat list (F-448).
func (s *ConfigStart) Menu() ([]configStartItem, error) {
	cfg, err := configFeature.get()
	if err != nil {
		return nil, err
	}
	defer configFeature.mu.Unlock()
	out := []configStartItem{}
	var walk func(items []*config.MenuItem, depth int)
	walk = func(items []*config.MenuItem, depth int) {
		for _, it := range items {
			e := configStartItem{Depth: depth, Title: it.Title}
			switch it.Kind {
			case config.MenuSeparator:
				e.Kind = "sep"
			case config.MenuSubmenu:
				e.Kind = "submenu"
			default:
				e.Kind = "item"
				e.Cmd, e.Params, e.Path = it.Field("cmd"), it.Field("param"), it.Field("path")
				if k := strings.TrimSpace(it.Field("key")); k != "" {
					if h, err := config.ParseHotkey(k); err == nil {
						e.Key = configComboFromHotkey(h)
					}
				}
			}
			out = append(out, e)
			if it.Kind == config.MenuSubmenu {
				walk(it.Children, depth+1)
			}
		}
	}
	walk(cfg.StartMenu.Items, 0)
	return out, nil
}

// configStartTree turns the flat list back into the menu tree. An entry deeper than the submenu before it allows goes
// into that submenu; a depth that jumps further is pulled back.
func configStartTree(items []configStartItem) ([]*config.MenuItem, error) {
	var root []*config.MenuItem
	stack := []*[]*config.MenuItem{&root}
	for _, e := range items {
		d := e.Depth
		if d < 0 {
			d = 0
		}
		if d > len(stack)-1 {
			d = len(stack) - 1
		}
		stack = stack[:d+1]
		var it *config.MenuItem
		switch e.Kind {
		case "sep":
			it = &config.MenuItem{Kind: config.MenuSeparator, Title: "-"}
		case "submenu":
			title := strings.TrimSpace(e.Title)
			if title == "" {
				return nil, fmt.Errorf("A submenu needs a name.")
			}
			it = &config.MenuItem{Kind: config.MenuSubmenu, Title: title}
		default:
			key := ""
			if k := strings.TrimSpace(e.Key); k != "" {
				h, err := config.ParseHotkey(k)
				if err != nil {
					return nil, fmt.Errorf("%q is not a key.", k)
				}
				key = h.String()
			}
			title := strings.TrimSpace(e.Title)
			if title == "" {
				title = strings.TrimSpace(e.Cmd)
			}
			if title == "" || title == "-" || title == "--" || strings.HasPrefix(title, "-") {
				return nil, fmt.Errorf("The entry %q needs a name that does not start with -.", title)
			}
			it = config.NewStartItem(title, strings.TrimSpace(e.Cmd), e.Params, strings.TrimSpace(e.Path), key)
		}
		list := stack[len(stack)-1]
		*list = append(*list, it)
		if it.Kind == config.MenuSubmenu {
			stack = append(stack, &it.Children)
		}
	}
	return root, nil
}

// SaveMenu replaces the Start menu and saves the settings file (F-450).
func (s *ConfigStart) SaveMenu(items []configStartItem) error {
	tree, err := configStartTree(items)
	if err != nil {
		return err
	}
	cfg, err := configFeature.get()
	if err != nil {
		return err
	}
	defer configFeature.mu.Unlock()
	cfg.StartMenu.Items = tree
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("The Start menu could not be saved: %v", err)
	}
	return nil
}

// StartParams is what the command line asked for beyond the panel folders the page opens at start (F-730 to F-734,
// F-852): new tabs, source and target, a tool to start on its own.
func (s *ConfigStart) StartParams() map[string]any {
	return configStartParams(s.app.opts)
}

func configStartParams(o startOpts) map[string]any {
	return map[string]any{"left": o.TCLeft, "right": o.TCRight, "newTab": o.NewTab, "sourceTarget": o.SourceTarget,
		"active": o.Active, "tool": o.Tool, "toolOpts": o.ToolOpts, "file1": o.ToolFile1, "file2": o.ToolFile2,
		"openInRunning": o.OpenInRunning, "newInstance": o.NewInstance, "ini": o.IniPath, "ftpIni": o.FtpIniPath}
}

// ---- one window: /O and "Allow only one copy" ----

type configInstance struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

type configForward struct {
	Token string   `json:"token"`
	Args  []string `json:"args"`
	Cwd   string   `json:"cwd"`
}

var configServer net.Listener

func init() {
	if configForwardWanted(os.Args[1:]) && configForwardToRunning(os.Args[1:]) {
		os.Exit(0)
	}
	onStartup(configStartServer)
	onShutdown(func(a *App) {
		if configServer != nil {
			configServer.Close()
			if loc, err := configLocate(a.opts); err == nil {
				p := filepath.Join(loc.Dir, "instance.json")
				if b, err := os.ReadFile(p); err == nil {
					var in configInstance
					if json.Unmarshal(b, &in) == nil && in.PID == os.Getpid() {
						os.Remove(p)
					}
				}
			}
		}
	})
}

// configForwardWanted says whether this start should look for a running window: with /O, or with "Allow only one copy"
// on, never with /N, never for the app's own helper windows, demo runs, tests or tools.
func configForwardWanted(args []string) bool {
	if strings.HasSuffix(os.Args[0], ".test") {
		return false
	}
	for _, a := range args {
		if strings.HasPrefix(a, "-test.") {
			return false
		}
	}
	o := parseArgs(args)
	if o.About || o.Demo || o.Version || o.Help || o.NewInstance || o.Tool != "" || o.Control != "" {
		return false
	}
	if o.OpenInRunning {
		return true
	}
	loc, err := configLocate(o)
	if err != nil {
		return false
	}
	cfg, err := config.Open(loc)
	if err != nil {
		return false
	}
	return cfg.Settings.Operation.OnlyOnce // (Options > Operation > Allow only one copy of the program)
}

// configForwardToRunning sends the arguments to the running window; false when there is none (or it does not answer).
func configForwardToRunning(args []string) bool {
	o := parseArgs(args)
	loc, err := configLocate(o)
	if err != nil {
		return false
	}
	b, err := os.ReadFile(filepath.Join(loc.Dir, "instance.json"))
	if err != nil {
		return false
	}
	var in configInstance
	if json.Unmarshal(b, &in) != nil || in.Port <= 0 || in.Token == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", in.Port), 700*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	cwd, _ := os.Getwd()
	msg, _ := json.Marshal(configForward{Token: in.Token, Args: args, Cwd: cwd})
	if _, err := conn.Write(append(msg, '\n')); err != nil {
		return false
	}
	reply, err := bufio.NewReader(conn).ReadString('\n')
	return err == nil && strings.TrimSpace(reply) == "ok"
}

func configStartServer(a *App) {
	if a.mode != "" || a.mock != nil {
		return
	}
	loc, err := configLocate(a.opts)
	if err != nil {
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return
	}
	var tok [16]byte
	rand.Read(tok[:])
	in := configInstance{Port: ln.Addr().(*net.TCPAddr).Port, Token: hex.EncodeToString(tok[:]), PID: os.Getpid()}
	b, _ := json.Marshal(in)
	if os.MkdirAll(loc.Dir, 0o755) != nil {
		ln.Close()
		return
	}
	tmp := filepath.Join(loc.Dir, "instance.json.tmp")
	if os.WriteFile(tmp, b, 0o600) != nil || os.Rename(tmp, filepath.Join(loc.Dir, "instance.json")) != nil {
		ln.Close()
		return
	}
	configServer = ln
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go configServe(a, conn, in.Token)
		}
	}()
}

func configServe(a *App, conn net.Conn, token string) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	var m configForward
	if json.Unmarshal([]byte(line), &m) != nil || m.Token != token {
		return
	}
	o := parseArgs(m.Args)
	s := parseStartupLenient(configTCOnly(m.Args)).Absolute(m.Cwd, params.DefaultStyle)
	left, right := s.Panels(false)
	o.TCLeft, o.TCRight = left, right
	info := configStartParams(o)
	info["forwarded"] = true
	conn.Write([]byte("ok\n"))
	a.reveal()
	a.emit("config:open", info)
}

// configTCOnly drops the app's own flags, leaving TC's parameters (for a forwarded command line).
func configTCOnly(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--theme" || a == "--control-file" || a == "--control":
			if i+1 < len(args) && (a != "--control" || isPort(args[i+1])) {
				i++
			}
		case strings.HasPrefix(a, "--theme=") || strings.HasPrefix(a, "--control") || strings.HasPrefix(a, "-psn_"):
		case a == "--left" || a == "--right":
			if i+1 < len(args) {
				i++
				out = append(out, a+"="+args[i])
			}
		default:
			out = append(out, a)
		}
	}
	return out
}
