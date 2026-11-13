// Total Commander parameter placeholders. Buttons, start menu items, internal associations and the external viewer
// and editor all describe their parameters as one line of text in which %-placeholders stand for the panels' state:
// %P is the source folder, %N the name under the cursor, %S the selected names, %L a temporary list file of them and
// so on. ExpandParams replaces them so the same configuration lines a Total Commander user already has keep working.
//
// The placeholders, as Total Commander documents them:
//
//	%P   source path, with a separator at the end      %T   target path, with a separator at the end
//	%N   name under the cursor (source panel)          %M   name under the cursor in the target panel
//	%O   that name without its extension               %E   its extension, without the dot
//	%S   selected names, separated by spaces (the name under the cursor when nothing is selected)
//	%R   selected names in the target panel
//	%P%S and %T%R   selected names, each with its full path
//	%L   list file with the full paths of the selected items, one per line; its path is inserted
//	%F   list file with names only                     %D   like %L, with short (8.3) names on Windows
//	%UL, %UF  UTF-8 list files; %WL, %WF  UTF-16 list files ("wide"); %Z  UTF-8 list file (as %UL)
//	%Y   anywhere: list files stay empty when nothing is selected, instead of naming the file under the cursor
//	%1   the file being opened (viewer, editor, associations)
//	%%   a single %;  %NAME%  the environment variable NAME, when it exists
//	?    at the very start: ask the user to confirm or edit the parameters before running (Expanded.Prompt)
//
// Lowercase letters (%p, %n, …) give short 8.3 names on Windows and are the same as the uppercase ones elsewhere.
// Inserted values are escaped so that they stay one argument (see EscapeArg) on macOS and Linux; on Windows they are
// inserted as they are and the user quotes them ("%P%N"), exactly as in Total Commander.
package launch

import (
	"bytes"
	"os"
	"strings"
	"unicode/utf16"
)

// Panel is the state of one file panel that placeholders can refer to.
type Panel struct {
	Dir      string   // the folder shown in the panel
	Current  string   // the name under the cursor ("" or ".." when the cursor is on the parent entry)
	Selected []string // the marked names, in panel order
}

// ParamContext is everything ExpandParams needs.
type ParamContext struct {
	Source Panel
	Target Panel
	File   string // the file being opened, for %1; when empty, %1 is the source path + current name
}

// Expanded is the result of ExpandParams.
type Expanded struct {
	Text      string   // the parameters with every placeholder replaced
	Prompt    bool     // the parameters started with '?': show them to the user for editing before running
	ListFiles []string // temporary list files written for %L and friends; delete them when the program has quit
	HasFile   bool     // the parameters contained %1, %N, %S or a list placeholder, so no file needs to be appended
}

// selection returns the names a %S or %L placeholder stands for.
func (p Panel) selection(emptyIfNone bool) []string {
	if len(p.Selected) > 0 {
		return p.Selected
	}
	if emptyIfNone || p.Current == "" || p.Current == ".." {
		return nil
	}
	return []string{p.Current}
}

// ExpandParams replaces the placeholders in params. List files are written to the Launcher's TempDir; when the
// expansion fails, any list file already written is removed again.
func (l *Launcher) ExpandParams(params string, ctx ParamContext) (Expanded, error) {
	var res Expanded
	if strings.HasPrefix(params, "?") {
		res.Prompt = true
		params = params[1:]
	}
	emptyLists := strings.Contains(params, "%Y") || strings.Contains(params, "%y")
	var b strings.Builder
	for i := 0; i < len(params); i++ {
		ch := params[i]
		if ch != '%' || i+1 >= len(params) {
			b.WriteByte(ch)
			continue
		}
		rest := params[i+1:]
		// %NAME% environment variables first, when such a variable exists.
		if name, ok := envName(rest); ok {
			if v, found := l.lookupEnv(name); found {
				b.WriteString(v)
				i += len(name) + 1
				continue
			}
		}
		up := strings.ToUpper(rest)
		short := rest[0] >= 'a' && rest[0] <= 'z'
		value := func(s string) {
			if short && l.windows() {
				s = shortName(s)
			}
			b.WriteString(l.insertValue(s))
		}
		switch {
		case rest[0] == '%':
			b.WriteByte('%')
			i++
		case strings.HasPrefix(up, "P%S"), strings.HasPrefix(up, "T%R"):
			panel := ctx.Source
			if up[0] == 'T' {
				panel = ctx.Target
			}
			names := panel.selection(false)
			parts := make([]string, len(names))
			for k, n := range names {
				parts[k] = l.quoteListItem(joinPath(l.OS, panel.Dir, n))
			}
			b.WriteString(strings.Join(parts, " "))
			res.HasFile = true
			i += 3
		case strings.HasPrefix(up, "WL"), strings.HasPrefix(up, "WF"), strings.HasPrefix(up, "UL"), strings.HasPrefix(up, "UF"):
			p, err := l.writeList(ctx.Source, up[1] == 'L', up[0] == 'W', false, emptyLists)
			if err != nil {
				removeAll(res.ListFiles)
				return Expanded{}, err
			}
			res.ListFiles = append(res.ListFiles, p)
			res.HasFile = true
			b.WriteString(l.insertValue(p))
			i += 2
		default:
			switch up[0] {
			case 'P':
				value(withTrailingSep(l.OS, ctx.Source.Dir))
			case 'T':
				value(withTrailingSep(l.OS, ctx.Target.Dir))
			case 'N':
				value(ctx.Source.Current)
				res.HasFile = true
			case 'M':
				value(ctx.Target.Current)
			case 'O':
				stem, _ := splitExt(ctx.Source.Current)
				value(stem)
			case 'E':
				_, ext := splitExt(ctx.Source.Current)
				value(ext)
			case 'S', 'R':
				panel := ctx.Source
				if up[0] == 'R' {
					panel = ctx.Target
				}
				names := panel.selection(false)
				parts := make([]string, len(names))
				for k, n := range names {
					if short && l.windows() {
						n = baseName(l.OS, shortName(joinPath(l.OS, panel.Dir, n)))
					}
					parts[k] = l.quoteListItem(n)
				}
				b.WriteString(strings.Join(parts, " "))
				res.HasFile = true
			case 'L', 'F', 'D', 'Z':
				full := up[0] != 'F'
				p, err := l.writeList(ctx.Source, full, false, up[0] == 'D', emptyLists)
				if err != nil {
					removeAll(res.ListFiles)
					return Expanded{}, err
				}
				res.ListFiles = append(res.ListFiles, p)
				res.HasFile = true
				b.WriteString(l.insertValue(p))
			case 'Y':
				// Only a switch; it inserts nothing.
			case '1':
				f := ctx.File
				if f == "" && ctx.Source.Current != "" {
					f = joinPath(l.OS, ctx.Source.Dir, ctx.Source.Current)
				}
				b.WriteString(l.insertValue(f))
				res.HasFile = true
			default:
				// Not a placeholder: keep the % as it is.
				b.WriteByte('%')
				continue
			}
			i++
		}
	}
	res.Text = b.String()
	return res, nil
}

func (l *Launcher) lookupEnv(name string) (string, bool) {
	if l.LookupEnv == nil {
		return "", false
	}
	return l.LookupEnv(name)
}

// envName reports whether s starts with "NAME%" where NAME has at least two characters that can make up an
// environment variable name. One-letter names are always placeholders.
func envName(s string) (string, bool) {
	end := strings.IndexByte(s, '%')
	if end < 2 {
		return "", false
	}
	name := s[:end]
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(isLetter(c) || c >= '0' && c <= '9' || c == '_' || c == '(' || c == ')') {
			return "", false
		}
	}
	return name, true
}

// insertValue prepares one inserted value: on Windows it goes in as it is; elsewhere it is escaped so splitting the
// parameters later gives it back as one piece.
func (l *Launcher) insertValue(s string) string {
	if l.windows() {
		return s
	}
	return EscapeArg(s)
}

// quoteListItem prepares one name of %S: Windows quotes names with spaces, elsewhere the name is escaped.
func (l *Launcher) quoteListItem(s string) string {
	if l.windows() {
		if strings.ContainsAny(s, " \t") {
			return `"` + s + `"`
		}
		return s
	}
	return EscapeArg(s)
}

// writeList writes a list file of the panel's selection and returns its path. full chooses full paths over names,
// wide chooses UTF-16 (with a byte order mark) over UTF-8, short asks for 8.3 names on Windows.
func (l *Launcher) writeList(p Panel, full, wide, short, emptyIfNone bool) (string, error) {
	eol := "\n"
	if l.windows() {
		eol = "\r\n"
	}
	var text strings.Builder
	for _, n := range p.selection(emptyIfNone) {
		item := n
		if full || short {
			item = joinPath(l.OS, p.Dir, n)
			if short && l.windows() {
				item = shortName(item)
			}
			if !full {
				item = baseName(l.OS, item)
			}
		}
		text.WriteString(item)
		text.WriteString(eol)
	}
	var data []byte
	if wide {
		u := utf16.Encode([]rune(text.String()))
		var buf bytes.Buffer
		buf.Write([]byte{0xFF, 0xFE})
		for _, c := range u {
			buf.WriteByte(byte(c))
			buf.WriteByte(byte(c >> 8))
		}
		data = buf.Bytes()
	} else {
		data = []byte(text.String())
	}
	f, err := os.CreateTemp(l.tempDir(), "mormi*.lst")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// CommandFor builds the Command that runs program with already expanded parameter text in dir. Environment
// variables written as %NAME% in the program path are expanded first (for example %COMMANDER_PATH%\tool.exe).
func (l *Launcher) CommandFor(program, params, dir string) (Command, error) {
	program = l.ExpandEnv(program)
	c := Command{Path: program, Dir: dir}
	if l.windows() {
		c.Params = params
		return c, nil
	}
	args, err := SplitArgs(params)
	if err != nil {
		return Command{}, err
	}
	c.Args = args
	return c, nil
}
