// Command templates for external helper programs: the F4 editor, the F3
// external viewer, the external compare tool and "Open with" style
// associations. Total Commander configures these as a program plus
// parameters in which %1 (and %2 for the compare tool) stands for the file,
// while the usual button placeholders (%P, %N, %T, %O, %E, a leading "?")
// keep working. When the template names no file at all, TC appends the
// file name in quotes, so "notepad.exe" alone still opens the file. This
// file implements that rule on top of Expand, so the same quoting and
// list-file behaviour applies everywhere. It also holds SplitFileTarget,
// which implements TC's start-up habit of opening the containing folder
// with the cursor on the file when a file instead of a folder is passed.
package params

import (
	"os"
	"path/filepath"
	"strings"
)

// fileRefs reports whether the template mentions %1..%9 or a placeholder
// that already names the current file(s) (%N %S %L %F %D %UL %WL ...), in
// which case no file is appended.
func fileRefs(t string) (digits, named bool) {
	for i := 0; i+1 < len(t); i++ {
		if t[i] != '%' {
			continue
		}
		c := t[i+1]
		switch {
		case c == '%':
			i++
		case c >= '1' && c <= '9':
			digits = true
		case strings.IndexByte("NnSsLlFfDd", c) >= 0:
			named = true
		case strings.IndexByte("UuWw", c) >= 0 && i+2 < len(t) && strings.IndexByte("LlFf", t[i+2]) >= 0:
			named = true
		}
	}
	return
}

// quoteAt reports the user quote context at position i of template t:
// 0 (unquoted), a double quote or a single quote. It scans with the same rules Expand uses, so
// a substituted file name is escaped for exactly the context the command
// line splitter will see. Windows only knows double quotes; POSIX also
// knows single quotes and backslash escapes (except in single quotes and
// in front of a placeholder's %).
func quoteAt(t string, i int, st Style) byte {
	var q byte
	for j := 0; j < i; j++ {
		c := t[j]
		if c == '%' && j+1 < len(t) {
			j++
			continue
		}
		if st == StyleWindows {
			if c == '"' {
				if q == 0 {
					q = '"'
				} else {
					q = 0
				}
			}
			continue
		}
		switch {
		case c == '\\' && q != '\'' && j+1 < len(t) && t[j+1] != '%':
			j++
		case (c == '"' || c == '\'') && q == 0:
			q = c
		case c == q:
			q = 0
		}
	}
	return q
}

// escapeFileIn renders file name v for the quote context q at its place in
// the template (beforeClose: the template's next byte closes that quote).
// Inside user quotes the name is escaped, never inserted raw, so a name
// that contains a quote cannot end the argument and add new ones.
func escapeFileIn(v string, q byte, beforeClose bool, st Style) string {
	switch {
	case st == StyleWindows && q == '"':
		return escapeWindowsInQuotes(v, beforeClose)
	case q == '"':
		return escapePOSIXInDouble(v)
	case q == '\'':
		return strings.ReplaceAll(v, "'", `'\''`)
	}
	return Quote(v, st)
}

// escapePercent doubles every % so Expand leaves substituted text alone.
func escapePercent(s string) string { return strings.ReplaceAll(s, "%", "%%") }

// ExpandFileCommand expands the parameters of an editor, viewer or compare
// tool. files[0] replaces %1, files[1] %2 and so on (missing ones become
// empty); each is quoted when needed, or escaped for the user's own quotes
// when the template already puts it inside quotes, unless the template
// contains %Q. All other placeholders behave as in
// Expand. When the template references no file at all, the files are
// appended, quoted as needed.
func ExpandFileCommand(template string, files []string, ctx Context) (Result, error) {
	st := ctx.style()
	prompt := strings.HasPrefix(template, "?")
	body := strings.TrimPrefix(template, "?")
	digits, named := fileRefs(body)
	noQuote := hasQ(body)
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '%' || i+1 >= len(body) {
			b.WriteByte(c)
			continue
		}
		n := body[i+1]
		if n == '%' {
			b.WriteString("%%")
			i++
			continue
		}
		if n < '1' || n > '9' {
			b.WriteByte(c)
			continue
		}
		idx := int(n - '1')
		v := ""
		if idx < len(files) {
			v = files[idx]
		}
		if v != "" && !noQuote {
			q := quoteAt(body, i, st)
			v = escapeFileIn(v, q, q != 0 && i+2 < len(body) && body[i+2] == q, st)
		}
		b.WriteString(escapePercent(v))
		i++
	}
	t := b.String()
	if !digits && !named {
		for _, f := range files {
			q := f
			if !noQuote {
				q = Quote(f, st)
			}
			if t != "" && !strings.HasSuffix(t, " ") {
				t += " "
			}
			t += escapePercent(q)
		}
	}
	if prompt {
		t = "?" + t
	}
	return Expand(t, ctx)
}

// SplitFileTarget looks at a folder given on the start-up command line.
// When it names an existing regular file, TC opens the containing folder
// and puts the cursor on the file; SplitFileTarget returns that folder and
// the file name. For folders, missing paths and virtual paths it returns
// the path unchanged and an empty name.
func SplitFileTarget(p string) (dir, cursor string) {
	if p == "" || IsVirtual(p) {
		return p, ""
	}
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return p, ""
	}
	clean := filepath.Clean(p)
	return filepath.Dir(clean), filepath.Base(clean)
}
