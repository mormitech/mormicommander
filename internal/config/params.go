// Parameter placeholders for buttons, Start menu items and user commands
// (F-310, F-814). Total Commander replaces short % codes in a button's
// parameters with the current panel state before it starts the program:
// %P is the source folder, %N the file under the cursor, %T the target
// folder, %M the file under the cursor in the target panel, %O and %E the
// name without and the extension, %S the selected names (quoted when they
// contain spaces), %R the target panel's selection, and %L, %F, %UL, %UF,
// %WL, %WF name a temporary list file of the selection (full paths or names
// only; ANSI, UTF-8 or UTF-16). %Z is our alias for %UL. %Q turns off the
// automatic quoting of %S, %$DATE% / %$TIME% insert the current date and
// time, %% is a percent sign, and any other %NAME% is an environment or
// commander variable. A "?" at the very start means "show the expanded
// parameters in an edit box before running", which ExpandParams reports so
// the UI can ask. All of this is pure string work, so it is fully tested here
// and the UI only supplies the panel state.

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

// ListEncoding is the text encoding of a %L-style list file.
type ListEncoding int

const (
	ListANSI  ListEncoding = iota // %L, %F: system code page (UTF-8 off Windows)
	ListUTF8                      // %UL, %UF, %Z
	ListUTF16                     // %WL, %WF (with BOM)
)

// ParamContext is the panel state used for expansion.
type ParamContext struct {
	SourceDir      string   // active panel folder
	TargetDir      string   // other panel folder
	Current        string   // name under the cursor in the source panel
	TargetCurrent  string   // name under the cursor in the target panel
	Selected       []string // marked names in the source panel (empty = Current)
	TargetSelected []string // marked names in the target panel (empty = TargetCurrent)
	Vars           Vars     // for %NAME% variables
	TempDir        string   // where list files go ("" = os.TempDir())
	Now            time.Time
	Separator      string // path separator to append to folders ("" = OS)
}

// Expansion is the result of ExpandParams.
type Expansion struct {
	Text      string
	Prompt    bool     // the parameters started with "?"; ask before running
	ListFiles []string // temporary list files created; delete when done
}

func (c *ParamContext) sep() string {
	if c.Separator != "" {
		return c.Separator
	}
	return string(filepath.Separator)
}

func withSep(dir, sep string) string {
	if dir == "" || strings.HasSuffix(dir, "/") || strings.HasSuffix(dir, `\`) {
		return dir
	}
	return dir + sep
}

func splitExt(name string) (string, string) {
	i := strings.LastIndex(name, ".")
	if i <= 0 {
		return name, ""
	}
	return name[:i], name[i+1:]
}

// ExpandParams replaces the placeholders in params (see the file comment).
func ExpandParams(params string, ctx ParamContext) (Expansion, error) {
	var res Expansion
	if strings.HasPrefix(params, "?") {
		res.Prompt = true
		params = params[1:]
	}
	if ctx.Now.IsZero() {
		ctx.Now = time.Now()
	}
	sel := ctx.Selected
	if len(sel) == 0 && ctx.Current != "" && ctx.Current != ".." {
		sel = []string{ctx.Current}
	}
	tsel := ctx.TargetSelected
	if len(tsel) == 0 && ctx.TargetCurrent != "" && ctx.TargetCurrent != ".." {
		tsel = []string{ctx.TargetCurrent}
	}
	quote := !strings.Contains(strings.ToUpper(params), "%Q")
	src := withSep(ctx.SourceDir, ctx.sep())
	dst := withSep(ctx.TargetDir, ctx.sep())

	joinNames := func(prefix string, names []string) string {
		parts := make([]string, len(names))
		for i, n := range names {
			p := prefix + n
			if quote {
				p = QuoteArg(p)
			}
			parts[i] = p
		}
		return strings.Join(parts, " ")
	}
	list := func(withPath bool, enc ListEncoding) (string, error) {
		lines := make([]string, len(sel))
		for i, n := range sel {
			if withPath {
				lines[i] = src + n
			} else {
				lines[i] = n
			}
		}
		p, err := WriteListFile(ctx.TempDir, lines, enc)
		if err == nil {
			res.ListFiles = append(res.ListFiles, p)
		}
		return p, err
	}

	var b strings.Builder
	for i := 0; i < len(params); i++ {
		c := params[i]
		if c != '%' || i+1 >= len(params) {
			b.WriteByte(c)
			continue
		}
		rest := params[i+1:]
		up := strings.ToUpper(rest)
		// Two-letter list codes first.
		two := ""
		if len(up) >= 2 {
			two = up[:2]
		}
		switch two {
		case "UL", "UF", "WL", "WF":
			enc := ListUTF8
			if two[0] == 'W' {
				enc = ListUTF16
			}
			p, err := list(two[1] == 'L', enc)
			if err != nil {
				return res, err
			}
			b.WriteString(p)
			i += 2
			continue
		}
		if strings.HasPrefix(up, "$DATE") || strings.HasPrefix(up, "$TIME") {
			if j := strings.IndexByte(rest, '%'); j > 0 {
				b.WriteString(formatStamp(rest[:j], ctx.Now))
				i += j + 1
				continue
			}
		}
		if strings.HasPrefix(up, "P%S") {
			b.WriteString(joinNames(src, sel))
			i += 3
			continue
		}
		if strings.HasPrefix(up, "T%R") {
			b.WriteString(joinNames(dst, tsel))
			i += 3
			continue
		}
		// A known %NAME% variable (two or more characters) wins over the
		// one-letter codes, so %MYVAR% isn't read as %M followed by "YVAR%".
		if j := strings.IndexByte(rest, '%'); j > 1 && isVarName(rest[:j]) {
			if v, ok := ctx.Vars.Get(rest[:j]); ok {
				b.WriteString(v)
				i += j + 1
				continue
			}
		}
		handled := true
		switch up[0] {
		case '%':
			b.WriteByte('%')
		case 'P':
			b.WriteString(src)
		case 'T':
			b.WriteString(dst)
		case 'N':
			b.WriteString(ctx.Current)
		case 'M':
			b.WriteString(ctx.TargetCurrent)
		case 'O':
			n, _ := splitExt(ctx.Current)
			b.WriteString(n)
		case 'E':
			_, e := splitExt(ctx.Current)
			b.WriteString(e)
		case 'S':
			b.WriteString(joinNames("", sel))
		case 'R':
			b.WriteString(joinNames("", tsel))
		case 'Q':
			// Quoting switch; produces no text.
		case 'L', 'Z':
			p, err := list(true, map[byte]ListEncoding{'L': ListANSI, 'Z': ListUTF8}[up[0]])
			if err != nil {
				return res, err
			}
			b.WriteString(p)
		case 'F':
			p, err := list(false, ListANSI)
			if err != nil {
				return res, err
			}
			b.WriteString(p)
		default:
			handled = false
		}
		if handled {
			i++
			continue
		}
		b.WriteByte(c)
	}
	res.Text = b.String()
	return res, nil
}

// formatStamp handles "$DATE", "$TIME", "$DATE:YYYY-MM-DD" and
// "$TIME:hh-mm-ss" (Y M D h m s tokens, as in TC's multi-rename tool).
func formatStamp(spec string, now time.Time) string {
	kind, layout, hasLayout := strings.Cut(spec, ":")
	if !hasLayout {
		if strings.EqualFold(kind, "$TIME") {
			return now.Format("15:04:05")
		}
		return now.Format("2006-01-02")
	}
	r := strings.NewReplacer("YYYY", "2006", "YY", "06", "MM", "01", "DD", "02",
		"hh", "15", "mm", "04", "ss", "05")
	return now.Format(r.Replace(layout))
}

// WriteListFile writes lines to a new temporary file and returns its path.
func WriteListFile(dir string, lines []string, enc ListEncoding) (string, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	nl := "\n"
	if runtime.GOOS == "windows" {
		nl = "\r\n"
	}
	text := ""
	for _, l := range lines {
		text += l + nl
	}
	var data []byte
	switch enc {
	case ListUTF16:
		u := utf16.Encode([]rune(text))
		data = []byte{0xFF, 0xFE}
		for _, c := range u {
			data = append(data, byte(c), byte(c>>8))
		}
	case ListANSI:
		if runtime.GOOS == "windows" {
			if d, err := Encode(text, EncCP1252); err == nil {
				data = d
				break
			}
		}
		data = []byte(text)
	default:
		data = []byte(text)
	}
	f, err := os.CreateTemp(dir, "mc-list-*.lst")
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
