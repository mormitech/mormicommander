package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// The Claude Code CLI as the step-1 provider: the command line that makes it answer one prompt through the app's
// tools only, the MCP config that points it at the app, and finding the program.

// ClaudeArgs is the CLI's argument list for one run (the prompt goes on stdin, so it can be of any length and never
// meets a shell). --tools "" turns off every built-in tool (shell, file edits, web), --strict-mcp-config ignores the
// MCP servers the user set up for their own work, and --allowedTools lets the mormi tools run without asking, which
// the CLI could not do anyway: in -p mode there is nobody to ask.
func ClaudeArgs(mcpConfigPath, model, systemPrompt string) []string {
	args := []string{"-p", "--output-format", "stream-json", "--verbose",
		"--mcp-config", mcpConfigPath, "--strict-mcp-config",
		"--tools", "", "--allowedTools", ToolPrefix + "*"}
	if m := strings.TrimSpace(model); m != "" {
		args = append(args, "--model", m)
	}
	return append(args, "--append-system-prompt", systemPrompt)
}

// MCPConfig is the content of the --mcp-config file: the app's endpoint for this run, with the token in a header.
func MCPConfig(url, token string) []byte {
	b, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"mormi": map[string]any{
		"type": "http", "url": url, "headers": map[string]string{"Authorization": "Bearer " + token},
	}}}, "", "  ")
	return b
}

// systemRules are the rules every run gets. The model can't do anything but call the tools (the CLI's own tools are
// off), and saying so stops it from offering shell commands; one good search is faster for the user than many small
// ones, each of which walks the disk.
const systemRules = `You are the assistant inside mormiCommander, a two-panel file manager. You can only act through the mormi tools; you have no shell and cannot read or change files any other way. To search for files, use find_files; prefer one well-built search (folders, name patterns, text, size and date filters together) over many small ones. Answer briefly, in the language the user wrote in.`

// SystemPrompt is the text appended to the CLI's system prompt: what the page said about where the user is, then the
// rules.
func SystemPrompt(context string) string {
	if c := strings.TrimSpace(context); c != "" {
		return c + "\n\n" + systemRules
	}
	return systemRules
}

// Finder finds the claude program. Every way it looks is a field, so the order can be tested with made-up folders.
type Finder struct {
	GOOS string
	Home string
	// Env reads an environment variable (APPDATA on Windows).
	Env func(string) string
	// LookPath finds a program in this process's PATH, like exec.LookPath.
	LookPath func(string) (string, error)
	// LoginShell returns where the user's login shell finds claude (`$SHELL -l -c 'command -v claude'`), or "". An app
	// started from the Dock or Finder gets a minimal PATH, without the folders the user's shell profile adds.
	LoginShell func() string
	// IsFile says whether a path is an existing file (not a folder).
	IsFile func(string) bool
}

// Names are the file names the program can have on this system: the npm package installs claude.cmd on Windows, the
// native installer claude.exe.
func (f Finder) Names() []string {
	if f.GOOS == "windows" {
		return []string{"claude.exe", "claude.cmd"}
	}
	return []string{"claude"}
}

// Places are the folders the installers put claude in, looked at when neither PATH knows it.
func (f Finder) Places() []string {
	h := f.Home
	env := func(k string) string {
		if f.Env == nil {
			return ""
		}
		return f.Env(k)
	}
	if f.GOOS == "windows" {
		out := []string{filepath.Join(h, ".local", "bin"), filepath.Join(h, ".claude", "local")}
		if ad := env("APPDATA"); ad != "" {
			out = append(out, filepath.Join(ad, "npm"))
		}
		return out
	}
	out := []string{filepath.Join(h, ".local", "bin"), filepath.Join(h, ".claude", "local"),
		"/opt/homebrew/bin", "/usr/local/bin"}
	// npm's global folder: the prefix the user set (npm's own guide suggests ~/.npm-global), else the usual ones.
	if p := env("NPM_CONFIG_PREFIX"); p != "" {
		out = append(out, filepath.Join(p, "bin"))
	}
	return append(out, filepath.Join(h, ".npm-global", "bin"), filepath.Join(h, ".volta", "bin"))
}

// Find returns the path of claude: configured when it is set (and then only it, so a wrong setting shows as an
// error instead of silently using another copy), else from PATH, the login shell, and the usual places, in that order.
// ok is false when it is not found; the path is then the configured one, or "".
func (f Finder) Find(configured string) (path string, ok bool) {
	if c := strings.TrimSpace(configured); c != "" {
		c = expandHome(c, f.Home)
		if f.isFile(c) {
			return c, true
		}
		if !strings.ContainsAny(c, `/\`) && f.LookPath != nil {
			if p, err := f.LookPath(c); err == nil {
				return p, true
			}
		}
		return c, false
	}
	if f.LookPath != nil {
		for _, n := range f.Names() {
			if p, err := f.LookPath(n); err == nil {
				return p, true
			}
		}
	}
	if f.GOOS != "windows" && f.LoginShell != nil {
		if p := f.LoginShell(); p != "" && f.isFile(p) {
			return p, true
		}
	}
	for _, d := range f.Places() {
		for _, n := range f.Names() {
			if p := filepath.Join(d, n); f.isFile(p) {
				return p, true
			}
		}
	}
	return "", false
}

func (f Finder) isFile(p string) bool {
	if f.IsFile != nil {
		return f.IsFile(p)
	}
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// expandHome turns a leading "~" into the home folder, as users write it in the option.
func expandHome(p, home string) string {
	if home != "" && (p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`)) {
		return filepath.Join(home, p[1:])
	}
	return p
}

// LastPathLine picks the program path out of a login shell's output: the last line that is an absolute path. A
// shell profile may print a greeting or a warning before `command -v` answers, and an alias answer
// ("alias claude=...") is no path at all.
func LastPathLine(out string) string {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); strings.HasPrefix(l, "/") {
			return l
		}
	}
	return ""
}

// VersionText takes the version number out of `claude --version` ("2.1.322 (Claude Code)" → "2.1.322").
func VersionText(out string) string {
	s := strings.TrimSpace(out)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if i := strings.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	return s
}
