package ai

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestClaudeArgs(t *testing.T) {
	got := ClaudeArgs("/tmp/m.json", "", "SYS")
	want := []string{"-p", "--output-format", "stream-json", "--verbose", "--mcp-config", "/tmp/m.json", "--strict-mcp-config",
		"--tools", "", "--allowedTools", "mcp__mormi__*", "--append-system-prompt", "SYS"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("without a model:\n got %q\nwant %q", got, want)
	}
	got = ClaudeArgs("/tmp/m.json", " sonnet ", "SYS")
	if i := indexOf(got, "--model"); i < 0 || got[i+1] != "sonnet" || got[len(got)-2] != "--append-system-prompt" {
		t.Errorf("with a model: %q", got)
	}
}

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return -1
}

func TestMCPConfig(t *testing.T) {
	var c struct {
		McpServers map[string]struct {
			Type    string            `json:"type"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(MCPConfig("http://127.0.0.1:5000/mcp/3", "tok"), &c); err != nil {
		t.Fatal(err)
	}
	m := c.McpServers["mormi"]
	if m.Type != "http" || m.URL != "http://127.0.0.1:5000/mcp/3" || m.Headers["Authorization"] != "Bearer tok" {
		t.Errorf("config %+v", c)
	}
}

func TestSystemPrompt(t *testing.T) {
	p := SystemPrompt("The user is in the Find files dialog.")
	if !strings.HasPrefix(p, "The user is in the Find files dialog.\n\n") {
		t.Errorf("the context does not come first: %q", p)
	}
	for _, w := range []string{"only act through the mormi tools", "find_files", "one well-built search", "language"} {
		if !strings.Contains(p, w) {
			t.Errorf("the rules lack %q", w)
		}
	}
	if SystemPrompt("  ") != systemRules {
		t.Error("an empty context should leave only the rules")
	}
}

// fakeFinder is a Finder over a made-up home folder, where files exist only when made with touch.
func fakeFinder(t *testing.T, goos string) (Finder, string, func(string) string) {
	t.Helper()
	home := t.TempDir()
	touch := func(rel string) string {
		p := filepath.Join(home, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// Only files under the made-up home exist, so the real /usr/local/bin and /opt/homebrew/bin are never looked at.
	isFile := func(p string) bool {
		if !strings.HasPrefix(p, home) {
			return false
		}
		fi, err := os.Stat(p)
		return err == nil && !fi.IsDir()
	}
	f := Finder{GOOS: goos, Home: home, Env: func(string) string { return "" },
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
		LoginShell: func() string { return "" }, IsFile: isFile}
	return f, home, touch
}

func TestFinderOrder(t *testing.T) {
	f, home, touch := fakeFinder(t, "darwin")
	if p, ok := f.Find(""); ok || p != "" {
		t.Fatalf("found %q in an empty home", p)
	}
	// The usual places, in their order: ~/.claude/local after ~/.local/bin.
	local := touch(".claude/local/claude")
	if p, ok := f.Find(""); !ok || p != local {
		t.Errorf("place: %q %v", p, ok)
	}
	dotLocal := touch(".local/bin/claude")
	if p, _ := f.Find(""); p != dotLocal {
		t.Errorf("~/.local/bin should come first: %q", p)
	}
	// The login shell's answer comes before the places.
	shell := touch("shellbin/claude")
	f.LoginShell = func() string { return shell }
	if p, _ := f.Find(""); p != shell {
		t.Errorf("login shell should come before the places: %q", p)
	}
	// PATH comes before the login shell.
	inPath := touch("pathbin/claude")
	f.LookPath = func(n string) (string, error) {
		if n == "claude" {
			return inPath, nil
		}
		return "", errors.New("not found")
	}
	if p, _ := f.Find(""); p != inPath {
		t.Errorf("PATH should come first: %q", p)
	}
	// The option beats them all, with ~ expanded.
	opt := touch("custom/claude")
	if p, ok := f.Find("~/custom/claude"); !ok || p != opt {
		t.Errorf("option: %q %v", p, ok)
	}
	// A wrong option is reported, not replaced by another copy.
	if p, ok := f.Find(filepath.Join(home, "nothing", "claude")); ok || p != filepath.Join(home, "nothing", "claude") {
		t.Errorf("wrong option: %q %v", p, ok)
	}
	// A bare name in the option is looked up in PATH.
	if p, ok := f.Find("claude"); !ok || p != inPath {
		t.Errorf("bare option: %q %v", p, ok)
	}
}

func TestFinderNpmPrefix(t *testing.T) {
	f, home, touch := fakeFinder(t, "linux")
	p := touch("npmprefix/bin/claude")
	f.Env = func(k string) string {
		if k == "NPM_CONFIG_PREFIX" {
			return filepath.Join(home, "npmprefix")
		}
		return ""
	}
	if got, ok := f.Find(""); !ok || got != p {
		t.Errorf("npm prefix: %q %v", got, ok)
	}
}

func TestFinderWindows(t *testing.T) {
	f, home, touch := fakeFinder(t, "windows")
	f.LoginShell = func() string { t.Error("Windows has no login shell to ask"); return "" }
	if got := f.Names(); !reflect.DeepEqual(got, []string{"claude.exe", "claude.cmd"}) {
		t.Errorf("names %q", got)
	}
	f.Env = func(k string) string {
		if k == "APPDATA" {
			return filepath.Join(home, "AppData", "Roaming")
		}
		return ""
	}
	cmd := touch("AppData/Roaming/npm/claude.cmd")
	if p, ok := f.Find(""); !ok || p != cmd {
		t.Errorf("npm claude.cmd: %q %v", p, ok)
	}
	exe := touch(".local/bin/claude.exe")
	if p, _ := f.Find(""); p != exe {
		t.Errorf("native claude.exe should come first: %q", p)
	}
	var asked []string
	f.LookPath = func(n string) (string, error) { asked = append(asked, n); return "", errors.New("no") }
	f.Find("")
	if !reflect.DeepEqual(asked, []string{"claude.exe", "claude.cmd"}) {
		t.Errorf("PATH lookups %q", asked)
	}
}

func TestLastPathLine(t *testing.T) {
	for in, want := range map[string]string{
		"/Users/x/.local/bin/claude\n":                        "/Users/x/.local/bin/claude",
		"Welcome!\nnvm is loaded\n/opt/homebrew/bin/claude\n": "/opt/homebrew/bin/claude",
		"alias claude='npx claude'\n":                         "",
		"":                                                    "",
	} {
		if got := LastPathLine(in); got != want {
			t.Errorf("LastPathLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVersionText(t *testing.T) {
	for in, want := range map[string]string{"2.1.322 (Claude Code)\n": "2.1.322", "1.0.0": "1.0.0", "": ""} {
		if got := VersionText(in); got != want {
			t.Errorf("VersionText(%q) = %q, want %q", in, got, want)
		}
	}
}
