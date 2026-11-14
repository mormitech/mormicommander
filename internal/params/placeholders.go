// Expansion of button-bar and start-menu parameters. In Total Commander the
// "Parameters" field of a button or user-menu entry is a template: %P is the
// source folder, %N the name under the cursor, %S all selected names, %L a
// list file, and so on (the full table is in README.md). Names that contain
// spaces are quoted automatically unless %Q appears anywhere, and a
// placeholder glued to text forms one argument, so "%P%N" or "%O.zip" is
// quoted as a whole and "%P%S" repeats the folder in front of every
// selected name. A leading "?" means "show the expanded text to the user
// before running", and %[caption;default] asks the user for a value.
// %X, %Y and %Z switch the following placeholders to the left panel, the
// right panel, and back to source/target, so a button can refer to fixed
// sides regardless of which panel is active.
package params

import (
	"os"
	"strings"
)

// Entry is one item in a panel.
type Entry struct {
	Name string
	Dir  bool
}

// valid reports whether the entry is a real item (not empty or "..").
func (e Entry) valid() bool { return e.Name != "" && e.Name != ".." }

// Panel is the state of one file panel as far as placeholders care.
type Panel struct {
	// Path is the folder shown (local, UNC or virtual like ftp://h/dir).
	Path string
	// Cursor is the item under the cursor; Name "" or ".." means none.
	Cursor Entry
	// Selected are the marked items, in panel order.
	Selected []Entry
}

// items returns the selection, or the cursor item when nothing is marked.
func (p Panel) items() []Entry {
	if len(p.Selected) > 0 {
		return p.Selected
	}
	if p.Cursor.valid() {
		return []Entry{p.Cursor}
	}
	return nil
}

// Context is everything Expand needs to know about the app's state.
type Context struct {
	// Source is the active panel, Target the other one.
	Source, Target Panel
	// SourceIsRight tells which side is active, for %X (left) and %Y
	// (right).
	SourceIsRight bool
	// Style selects quoting and path rules; zero means DefaultStyle.
	Style Style
	// TempDir is where list files go; "" means os.TempDir().
	TempDir string
	// Env controls %NAME% expansion. Percent is always on in parameters.
	Env EnvOptions
	// Ask is called for %[caption;default]; it returns the value and false
	// when the user cancels. Nil means the default is used.
	Ask func(caption, def string) (string, bool)
	// Dropped are files dropped onto the button (F-307); they are appended
	// to the parameters as full paths, quoted as needed.
	Dropped []string
	// Batch says the program is a Windows batch file (.bat, .cmd), which cmd.exe reads: there a name such as
	// "a&b.txt" would end the command and start another, so names with & | < > ^ ( ) are quoted as well (#149).
	// Total Commander quotes only for blanks, which stays the rule for other programs.
	Batch bool
}

func (c Context) style() Style { return c.Style.orDefault() }

// batchMeta are the characters cmd.exe acts on outside quotes.
const batchMeta = "&|<>^()"

// needsQuote is NeedsQuote plus, for a batch file on Windows, cmd.exe's own operators.
func (e *expander) needsQuote(s string) bool {
	return NeedsQuote(s, e.st) || e.ctx.Batch && e.st == StyleWindows && strings.ContainsAny(s, batchMeta)
}

// quoteName quotes s when needsQuote says so.
func (e *expander) quoteName(s string) string {
	if !e.needsQuote(s) {
		return s
	}
	return QuoteAlways(s, e.st)
}

// Result is an expanded parameter string.
type Result struct {
	// Params is the final text, ready for SplitArgs.
	Params string
	// Prompt is true when the template started with "?": the UI shows
	// Params in an edit box and runs nothing on Cancel (F-814).
	Prompt bool
	// Canceled is true when the user canceled a %[...] question.
	Canceled bool
	// ListFiles are the temporary files created; delete them with Cleanup
	// after the started program has exited.
	ListFiles []string
}

// Cleanup deletes the list files and returns the first error.
func (r *Result) Cleanup() error {
	var first error
	for _, f := range r.ListFiles {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}
	r.ListFiles = nil
	return first
}

// part is one piece of an argument word being built.
type part struct {
	text        string
	sub         bool     // substituted value (subject to quoting)
	raw         bool     // substituted but never auto-quoted (env, %[...])
	quote       byte     // user quote context at this point: 0, '"' or '\''
	beforeClose bool     // Windows: the template's next char closes the quote
	multi       []string // %S / %R items when isMulti
	isMulti     bool
}

// Expand expands a parameter template.
func Expand(params string, ctx Context) (Result, error) {
	return expand(params, ctx, false)
}

// ExpandPath expands a "start path" field: env variables and %P %T %N %M
// %O %E %S %R are replaced without any quoting, list placeholders are left
// alone. An empty result means "use the source folder".
func ExpandPath(field string, ctx Context) (string, error) {
	r, err := expand(field, ctx, true)
	return r.Params, err
}

// hasQ reports whether the template contains the %Q placeholder.
func hasQ(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		if s[i+1] == '%' {
			i++
			continue
		}
		if s[i+1] == 'Q' || s[i+1] == 'q' {
			return true
		}
	}
	return false
}

type expander struct {
	ctx       Context
	st        Style
	pathMode  bool
	autoQuote bool
	src, tgt  Panel
	res       *Result
	words     [][]part
	cur       []part
	quote     byte
	err       error
}

func (e *expander) lit(s string) {
	if n := len(e.cur); n > 0 && !e.cur[n-1].sub && e.cur[n-1].quote == e.quote {
		e.cur[n-1].text += s
		return
	}
	e.cur = append(e.cur, part{text: s, quote: e.quote})
}

func (e *expander) flush() {
	if len(e.cur) > 0 {
		e.words = append(e.words, e.cur)
		e.cur = nil
	}
}

func (e *expander) fullPath(p Panel, it Entry) string {
	var full string
	if IsVirtual(p.Path) {
		full = WithTrailingSep(p.Path, e.st) + it.Name
	} else {
		full = JoinPath(p.Path, it.Name, e.st)
	}
	if it.Dir {
		full = WithTrailingSep(full, e.st)
	}
	return full
}

// listFile writes a list file for the source panel and returns its path.
func (e *expander) listFile(fullPaths bool, enc ListEncoding) (string, error) {
	var lines []string
	for _, it := range e.src.items() {
		if fullPaths {
			lines = append(lines, e.fullPath(e.src, it))
		} else {
			n := it.Name
			if it.Dir {
				n = WithTrailingSep(n, e.st)
			}
			lines = append(lines, n)
		}
	}
	f, err := WriteListFile(e.ctx.TempDir, lines, enc, e.st == StyleWindows)
	if err != nil {
		return "", err
	}
	e.res.ListFiles = append(e.res.ListFiles, f)
	return f, nil
}

func names(items []Entry) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Name
	}
	return out
}

func (e *expander) left() Panel {
	if e.ctx.SourceIsRight {
		return e.ctx.Target
	}
	return e.ctx.Source
}

func (e *expander) right() Panel {
	if e.ctx.SourceIsRight {
		return e.ctx.Source
	}
	return e.ctx.Target
}

// placeholder handles the text at s[i] == '%' and returns the next index.
func (e *expander) placeholder(s string, i int) int {
	if i+1 >= len(s) {
		e.lit("%")
		return i + 1
	}
	n := s[i+1]
	sub := func(v string, next int) int {
		p := part{text: v, sub: true, quote: e.quote}
		p.beforeClose = next < len(s) && s[next] == '"'
		e.cur = append(e.cur, p)
		return next
	}
	if n == '%' {
		e.lit("%")
		return i + 2
	}
	if n == '[' {
		end := strings.IndexByte(s[i+2:], ']')
		if end < 0 {
			e.lit("%")
			return i + 1
		}
		caption, def, _ := strings.Cut(s[i+2:i+2+end], ";")
		v := def
		if e.ctx.Ask != nil {
			var ok bool
			if v, ok = e.ctx.Ask(caption, def); !ok {
				e.res.Canceled = true
			}
		}
		e.cur = append(e.cur, part{text: v, sub: true, raw: true, quote: e.quote})
		return i + 3 + end
	}
	env := e.ctx.Env
	env.Percent = true
	if v, next, ok := env.percentVar(s, i); ok {
		e.cur = append(e.cur, part{text: v, sub: true, raw: true, quote: e.quote})
		return next
	}
	// Two-letter list placeholders: %WL %WF %UL %UF.
	if (n == 'W' || n == 'w' || n == 'U' || n == 'u') && i+2 < len(s) &&
		strings.IndexByte("LlFf", s[i+2]) >= 0 {
		if e.pathMode {
			e.lit(s[i : i+3])
			return i + 3
		}
		enc := ListUTF8
		if n == 'W' || n == 'w' {
			enc = ListUTF16
		}
		f, err := e.listFile(s[i+2] == 'L' || s[i+2] == 'l', enc)
		if err != nil {
			e.err = err
		}
		return sub(f, i+3)
	}
	next := i + 2
	switch n {
	case 'P', 'p':
		return sub(WithTrailingSep(e.src.Path, e.st), next)
	case 'T', 't':
		return sub(WithTrailingSep(e.tgt.Path, e.st), next)
	case 'N', 'n':
		return sub(cursorName(e.src), next)
	case 'M', 'm':
		return sub(cursorName(e.tgt), next)
	case 'O', 'o':
		b, _ := splitExt(cursorName(e.src), e.src.Cursor.Dir)
		return sub(b, next)
	case 'E', 'e':
		_, x := splitExt(cursorName(e.src), e.src.Cursor.Dir)
		return sub(x, next)
	case 'S', 's', 'R', 'r':
		p := e.src
		if n == 'R' || n == 'r' {
			p = e.tgt
		}
		e.cur = append(e.cur, part{sub: true, isMulti: true, multi: names(p.items()), quote: e.quote,
			beforeClose: next < len(s) && s[next] == '"'})
		return next
	case 'Q', 'q':
		return next
	case 'L', 'l', 'D', 'F', 'f', 'd':
		if e.pathMode {
			e.lit(s[i:next])
			return next
		}
		f, err := e.listFile(n == 'L' || n == 'l' || n == 'D', ListUTF8)
		if err != nil {
			e.err = err
		}
		return sub(f, next)
	case 'X', 'x':
		e.src, e.tgt = e.left(), e.right()
		return next
	case 'Y', 'y':
		e.src, e.tgt = e.right(), e.left()
		return next
	case 'Z', 'z':
		e.src, e.tgt = e.ctx.Source, e.ctx.Target
		return next
	}
	e.lit("%")
	return i + 1
}

func cursorName(p Panel) string {
	if p.Cursor.valid() {
		return p.Cursor.Name
	}
	return ""
}

func expand(params string, ctx Context, pathMode bool) (Result, error) {
	res := Result{}
	s := params
	if !pathMode && strings.HasPrefix(s, "?") {
		res.Prompt = true
		s = s[1:]
	}
	e := &expander{ctx: ctx, st: ctx.style(), pathMode: pathMode, res: &res,
		autoQuote: !pathMode && !hasQ(s), src: ctx.Source, tgt: ctx.Target}
	for i := 0; i < len(s) && e.err == nil && !res.Canceled; {
		c := s[i]
		if c == '%' {
			i = e.placeholder(s, i)
			continue
		}
		if e.quote == 0 && (c == ' ' || c == '\t') && !pathMode {
			e.flush()
			i++
			continue
		}
		if e.st == StyleWindows {
			if c == '"' {
				e.lit(`"`)
				if e.quote == 0 {
					e.quote = '"'
				} else {
					e.quote = 0
				}
				i++
				continue
			}
		} else if !pathMode {
			switch {
			case c == '\\' && e.quote != '\'' && i+1 < len(s) && s[i+1] != '%':
				e.lit(s[i : i+2])
				i += 2
				continue
			case (c == '"' || c == '\'') && e.quote == 0:
				e.lit(string(c))
				e.quote = c
				i++
				continue
			case c == e.quote:
				e.lit(string(c))
				e.quote = 0
				i++
				continue
			}
		}
		e.lit(string(c))
		i++
	}
	e.flush()
	if e.err != nil || res.Canceled {
		err := e.err
		res.Cleanup()
		res.Params = ""
		return res, err
	}
	var out []string
	for _, w := range e.words {
		out = append(out, e.render(w)...)
	}
	if !pathMode {
		for _, d := range ctx.Dropped {
			out = append(out, e.quoteName(d))
		}
	}
	sep := " "
	if pathMode {
		sep = ""
	}
	res.Params = strings.Join(out, sep)
	return res, nil
}

// render turns one template word into zero or more output words.
func (e *expander) render(w []part) []string {
	mi := -1
	for i, p := range w {
		if p.isMulti {
			if mi < 0 {
				mi = i
			} else {
				w[i].text = e.joinMulti(p)
			}
		}
	}
	if mi < 0 {
		if s, ok := e.renderOne(w); ok {
			return []string{s}
		}
		return nil
	}
	if e.pathMode {
		w[mi].text = strings.Join(w[mi].multi, " ")
		s, _ := e.renderOne(w)
		return []string{s}
	}
	var out []string
	for _, m := range w[mi].multi {
		inst := append([]part(nil), w...)
		inst[mi].text = m
		if s, ok := e.renderOne(inst); ok {
			out = append(out, s)
		}
	}
	return out
}

// joinMulti renders a second multi placeholder in the same word as one
// space-separated string; the word as a whole is then quoted as usual.
func (e *expander) joinMulti(p part) string {
	return strings.Join(p.multi, " ")
}

func (e *expander) renderOne(w []part) (string, bool) {
	var b strings.Builder
	allSubsEmpty, anyLit := true, false
	userQuoted := false
	for _, p := range w {
		if p.sub {
			if p.text != "" {
				allSubsEmpty = false
			}
		} else {
			anyLit = true
		}
		if p.quote != 0 || !p.sub && strings.ContainsAny(p.text, e.quoteChars()) {
			userQuoted = true
		}
	}
	if !anyLit && allSubsEmpty {
		return "", false
	}
	if !e.autoQuote {
		for _, p := range w {
			b.WriteString(p.text)
		}
		return b.String(), true
	}
	if userQuoted {
		for _, p := range w {
			b.WriteString(e.escapeIn(p))
		}
		return b.String(), true
	}
	needs := false
	for _, p := range w {
		b.WriteString(p.text)
		if p.sub && !p.raw && p.text != "" && e.needsQuote(p.text) {
			needs = true
		}
	}
	if needs {
		return QuoteAlways(b.String(), e.st), true
	}
	return b.String(), true
}

func (e *expander) quoteChars() string {
	if e.st == StyleWindows {
		return `"`
	}
	return `"'\`
}

// escapeIn renders a part that sits in (or next to) user-written quotes.
func (e *expander) escapeIn(p part) string {
	if !p.sub || p.raw {
		return p.text
	}
	if e.st == StyleWindows {
		if p.quote == '"' {
			return escapeWindowsInQuotes(p.text, p.beforeClose)
		}
		return e.quoteName(p.text)
	}
	switch p.quote {
	case '"':
		return escapePOSIXInDouble(p.text)
	case '\'':
		return strings.ReplaceAll(p.text, "'", `'\''`)
	}
	if p.text == "" {
		return ""
	}
	return Quote(p.text, e.st)
}
