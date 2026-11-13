// The command line below the panels (FEATURES.md section 10). Total Commander's command line is more than a shell
// prompt: "cd path" changes the panel's folder instead of starting a shell, a cm_* name runs an internal command,
// environment variables are expanded, the history is kept across restarts, and names from the panel can be inserted
// with the right quoting. This file holds the parts that need no window: ParseLine decides what a typed line means,
// History keeps the previous lines, CompleteLine finds matching files and folders, and AppendName adds a name the way
// Ctrl+Enter does. Running a line is in run.go.
package launch

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// LineKind says what a command line means.
type LineKind int

const (
	LineEmpty    LineKind = iota // nothing to do
	LineCd                       // change the panel's folder to ParsedLine.Target (or back, when Back is set)
	LineInternal                 // run the internal command ParsedLine.Command with ParsedLine.Args
	LineRun                      // run ParsedLine.Text as a program or shell command
)

// ParsedLine is the meaning of one command line.
type ParsedLine struct {
	Kind    LineKind
	Text    string // the line with environment variables expanded
	Target  string // LineCd: the absolute folder to go to
	Back    bool   // LineCd: "cd -", go back to the previous folder
	Command string // LineInternal: the command name, such as "cm_Copy"
	Args    string // LineInternal: the text after the command name
}

// ParseLine decides what the line typed into the command line means when cwd is the active panel's folder.
func (l *Launcher) ParseLine(line, cwd string) ParsedLine {
	line = strings.TrimSpace(line)
	if line == "" {
		return ParsedLine{Kind: LineEmpty}
	}
	// Internal commands are recognised before anything is expanded: their arguments are theirs to interpret.
	first, rest := cutWord(line)
	if lf := strings.ToLower(first); strings.HasPrefix(lf, "cm_") || strings.HasPrefix(lf, "em_") {
		return ParsedLine{Kind: LineInternal, Text: line, Command: first, Args: strings.TrimSpace(rest)}
	}
	text := l.ExpandEnv(line)
	first, rest = cutWord(text)
	if strings.EqualFold(first, "cd") || (l.windows() && (strings.EqualFold(first, "chdir") || strings.HasPrefix(strings.ToLower(text), `cd\`) || strings.HasPrefix(strings.ToLower(text), "cd.."))) {
		if !strings.EqualFold(first, "cd") && !strings.EqualFold(first, "chdir") {
			// "cd.." and "cd\" without a space, as cmd.exe accepts them.
			rest = text[2:]
		}
		arg := unquote(strings.TrimSpace(rest))
		if l.windows() && strings.HasPrefix(strings.ToLower(arg), "/d ") {
			arg = strings.TrimSpace(arg[3:])
		}
		switch {
		case arg == "-":
			return ParsedLine{Kind: LineCd, Text: text, Back: true}
		case arg == "":
			if l.windows() {
				return ParsedLine{Kind: LineCd, Text: text, Target: cleanPath(l.OS, cwd)}
			}
			arg = "~"
		}
		return ParsedLine{Kind: LineCd, Text: text, Target: joinPath(l.OS, cwd, l.expandHome(arg))}
	}
	// "D:" alone switches drive on Windows.
	if l.windows() && len(text) == 2 && text[1] == ':' && isLetter(text[0]) {
		return ParsedLine{Kind: LineCd, Text: text, Target: strings.ToUpper(text[:1]) + `:\`}
	}
	// A line that is just an existing folder opens that folder.
	if rest == "" {
		target := joinPath(l.OS, cwd, l.expandHome(unquote(text)))
		if l.OS == runtime.GOOS { // the folder can only be checked on the system it belongs to
			if fi, err := os.Stat(target); err == nil && fi.IsDir() {
				return ParsedLine{Kind: LineCd, Text: text, Target: target}
			}
		}
	}
	return ParsedLine{Kind: LineRun, Text: text}
}

// cutWord splits off the first word (which may be quoted) of s.
func cutWord(s string) (string, string) {
	s = strings.TrimLeft(s, " \t")
	if strings.HasPrefix(s, `"`) {
		if end := strings.IndexByte(s[1:], '"'); end >= 0 {
			return s[1 : end+1], s[end+2:]
		}
	}
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

// expandHome turns a leading ~ into the home folder on macOS and Linux.
func (l *Launcher) expandHome(p string) string {
	if l.windows() || !(p == "~" || strings.HasPrefix(p, "~/")) {
		return p
	}
	home := l.getenv("HOME")
	if home == "" {
		return p
	}
	return home + p[1:]
}

// ExpandEnv expands environment variables in s: %NAME% on every platform (Total Commander's syntax), and also
// $NAME and ${NAME} on macOS and Linux. Variables that do not exist are left as they are, as cmd.exe does.
func (l *Launcher) ExpandEnv(s string) string {
	if !strings.ContainsAny(s, "%$") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '%' {
			if end := strings.IndexByte(s[i+1:], '%'); end > 0 {
				name := s[i+1 : i+1+end]
				if validEnvName(name) {
					if v, ok := l.lookupEnv(name); ok {
						b.WriteString(v)
						i += end + 1
						continue
					}
				}
			}
		}
		if ch == '$' && !l.windows() && i+1 < len(s) {
			if s[i+1] == '{' {
				if end := strings.IndexByte(s[i+2:], '}'); end > 0 {
					name := s[i+2 : i+2+end]
					if validEnvName(name) {
						if v, ok := l.lookupEnv(name); ok {
							b.WriteString(v)
							i += end + 2
							continue
						}
					}
				}
			} else {
				j := i + 1
				for j < len(s) && (isLetter(s[j]) || s[j] == '_' || j > i+1 && s[j] >= '0' && s[j] <= '9') {
					j++
				}
				if j > i+1 {
					if v, ok := l.lookupEnv(s[i+1 : j]); ok {
						b.WriteString(v)
						i = j - 1
						continue
					}
				}
			}
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(isLetter(c) || c == '_' || c == '(' || c == ')' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// AppendName adds a name to the command line the way Ctrl+Enter and Ctrl+Shift+Enter do in Total Commander: with a
// space before it when the line does not already end in one, and quoted when it needs quotes.
func (l *Launcher) AppendName(line, name string) string {
	q := QuoteName(l.OS, name)
	if line == "" || strings.HasSuffix(line, " ") {
		return line + q
	}
	return line + " " + q
}

// CompleteLine completes the last word of line as a file or folder name, relative to cwd. It returns the possible
// complete lines, folders first, each folder ending in a separator so the next Tab continues inside it.
func (l *Launcher) CompleteLine(line, cwd string) []string {
	start := lastWordStart(line)
	word := line[start:]
	quoted := strings.HasPrefix(word, `"`)
	word = strings.TrimPrefix(word, `"`)
	word = l.expandHome(word)
	sep := sepFor(l.OS)
	dirPart, prefix := "", word
	if i := strings.LastIndexAny(word, slashChars(l.OS)); i >= 0 {
		dirPart, prefix = word[:i+1], word[i+1:]
	}
	searchDir := joinPath(l.OS, cwd, dirPart)
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil
	}
	fold := l.windows() || l.darwin()
	type cand struct {
		name string
		dir  bool
	}
	var cands []cand
	for _, e := range entries {
		n := e.Name()
		match := strings.HasPrefix(n, prefix)
		if fold {
			match = strings.HasPrefix(strings.ToLower(n), strings.ToLower(prefix))
		}
		if !match || (prefix == "" && strings.HasPrefix(n, ".")) {
			continue
		}
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(filepath.Join(searchDir, n)); err == nil {
				isDir = fi.IsDir()
			}
		}
		cands = append(cands, cand{n, isDir})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].dir != cands[j].dir {
			return cands[i].dir
		}
		return strings.ToLower(cands[i].name) < strings.ToLower(cands[j].name)
	})
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		full := dirPart + c.name
		if c.dir {
			full += sep
		}
		var w string
		switch {
		case l.windows() && (quoted || strings.ContainsAny(full, " \t")):
			w = `"` + full
			if !c.dir {
				w += `"`
			}
		case l.windows():
			w = full
		default:
			w = escapeShellWord(full)
		}
		out = append(out, line[:start]+w)
	}
	return out
}

func slashChars(osName string) string {
	if osName == "windows" {
		return `\/`
	}
	return "/"
}

// lastWordStart finds where the last word of a command line starts, skipping over quoted parts.
func lastWordStart(line string) int {
	start, inQuote := 0, false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			inQuote = !inQuote
		case ' ', '\t':
			if !inQuote {
				start = i + 1
			}
		}
	}
	return start
}

// escapeShellWord escapes the characters a POSIX shell would treat specially, without quotes, so a completion can be
// continued by typing.
func escapeShellWord(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(" \t\\\"'`$&|;<>()*?[]#~!{}", s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// History is the command line history (Alt+F8, Ctrl+E/Ctrl+X): the newest line first, no duplicates, at most Max
// lines. It is stored in the [Command line history] section of the settings file, as Total Commander's
// cmdlinehistory is.
type History struct {
	Max   int
	items []string
}

// HistorySection is the INI section name History is stored under.
const HistorySection = "Command line history"

// NewHistory returns an empty history that keeps at most max lines (Total Commander keeps 50 by default).
func NewHistory(max int) *History {
	if max <= 0 {
		max = 50
	}
	return &History{Max: max}
}

// Add puts line at the top of the history, removing an older copy of it.
func (h *History) Add(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	out := []string{line}
	for _, it := range h.items {
		if it != line {
			out = append(out, it)
		}
	}
	if len(out) > h.Max {
		out = out[:h.Max]
	}
	h.items = out
}

// Items returns the lines, newest first.
func (h *History) Items() []string { return append([]string(nil), h.items...) }

// Clear empties the history.
func (h *History) Clear() { h.items = nil }

// Load replaces the history with lines read from the settings file's section.
func (h *History) Load(f *INIFile) {
	h.items = nil
	s := f.Section(HistorySection)
	if s == nil {
		return
	}
	for _, kv := range s.Keys {
		if kv.Value != "" && len(h.items) < h.Max {
			h.items = append(h.items, kv.Value)
		}
	}
}

// Store writes the history into the settings file's section, numbered from 0 as Total Commander does.
func (h *History) Store(f *INIFile) {
	s := f.EnsureSection(HistorySection)
	s.Keys = nil
	for i, it := range h.items {
		s.Set(strconv.Itoa(i), it)
	}
}
