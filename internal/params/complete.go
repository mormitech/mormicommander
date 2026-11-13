// Path completion for the command-line box (F-296). When the user types
// the start of a path, the last word of the line is completed against the
// folder it points into: relative words are resolved against the active
// panel folder, "~" is the home folder, and after "cd" only folders are
// offered. Matching ignores case, as on Windows and the default macOS file
// system. Each candidate is returned as the complete new line, already
// quoted where needed, so the UI can show it in a dropdown and put it in
// the box as is. Folder reading goes through a function so tests and
// virtual file systems can supply their own listing.
package params

import (
	"io/fs"
	"os"
	"sort"
	"strings"
)

// Completion is one completion candidate.
type Completion struct {
	Line string // the whole new command line
	Name string // the completed entry name
	Dir  bool
}

// CompleteOptions controls Complete.
type CompleteOptions struct {
	Cwd   string
	Home  string
	Style Style
	// ReadDir lists a folder; nil means os.ReadDir.
	ReadDir func(dir string) ([]fs.DirEntry, error)
	// Max limits the number of candidates (0 = 100).
	Max int
	// Hidden includes dot files even when the typed prefix has no dot.
	Hidden bool
}

// lastWord returns the start index and unquoted text of the last word.
func lastWord(line string, st Style) (int, string) {
	start, inQ := 0, byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inQ != 0:
			if c == inQ {
				inQ = 0
			}
		case c == '"' || st == StylePOSIX && c == '\'':
			inQ = c
		case st == StylePOSIX && c == '\\':
			i++
		case c == ' ' || c == '\t':
			start = i + 1
		}
	}
	w := line[start:]
	if args, err := SplitArgs(w+closeQuote(inQ), st); err == nil && len(args) == 1 {
		return start, args[0]
	}
	return start, strings.Trim(w, `"'`)
}

func closeQuote(q byte) string {
	if q == 0 {
		return ""
	}
	return string(q)
}

// Complete returns completions for the last word of line.
func Complete(line string, o CompleteOptions) ([]Completion, error) {
	st := o.Style.orDefault()
	read := o.ReadDir
	if read == nil {
		read = os.ReadDir
	}
	max := o.Max
	if max <= 0 {
		max = 100
	}
	start, word := lastWord(line, st)
	dirsOnly := false
	if head := strings.TrimSpace(line[:start]); strings.EqualFold(head, "cd") ||
		st == StyleWindows && strings.EqualFold(head, "cd /d") {
		dirsOnly = true
	}
	typedDir, prefix := "", word
	if i := strings.LastIndexAny(word, sepChars(st)); i >= 0 {
		typedDir, prefix = word[:i+1], word[i+1:]
	}
	// Cwd and Home are host paths, so they are joined with the typed rest
	// rather than glued and re-parsed as typed text.
	var dir string
	switch {
	case o.Home != "" && (typedDir == "~" || strings.HasPrefix(typedDir, "~") &&
		strings.ContainsRune(sepChars(st), rune(typedDir[1]))):
		dir = JoinPath(o.Home, strings.TrimLeft(typedDir[1:], sepChars(st)), st)
	case strings.HasPrefix(typedDir, "~") && o.Home != "":
		dir = JoinPath(o.Cwd, o.Home+typedDir[1:], st)
	default:
		dir = JoinPath(o.Cwd, typedDir, st)
	}
	entries, err := read(dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	lp := strings.ToLower(prefix)
	var out []Completion
	for _, e := range entries {
		n := e.Name()
		isDir := e.IsDir()
		if e.Type()&fs.ModeSymlink != 0 {
			if fi, err := os.Stat(JoinPath(dir, n, st)); err == nil {
				isDir = fi.IsDir()
			}
		}
		if dirsOnly && !isDir {
			continue
		}
		if strings.HasPrefix(n, ".") && !strings.HasPrefix(prefix, ".") && !o.Hidden {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(n), lp) {
			continue
		}
		w := typedDir + n
		if isDir {
			w += st.Sep()
		}
		out = append(out, Completion{Line: line[:start] + quoteWord(w, st), Name: n, Dir: isDir})
		if len(out) >= max {
			break
		}
	}
	return out, nil
}

// quoteWord quotes a completed word but keeps a leading "~" outside the
// quotes on POSIX, so the shell (and cd) still expand it.
func quoteWord(w string, st Style) string {
	if st == StylePOSIX && strings.HasPrefix(w, "~/") {
		rest := w[2:]
		if rest == "" {
			return "~/"
		}
		return "~/" + Quote(rest, st)
	}
	return Quote(w, st)
}

func sepChars(st Style) string {
	if st == StyleWindows {
		return `/\`
	}
	return "/"
}
