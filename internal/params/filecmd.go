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

// insideDoubleQuotes reports whether position i of t is inside a pair of
// double quotes written by the user.
func insideDoubleQuotes(t string, i int) bool {
	return strings.Count(t[:i], `"`)%2 == 1
}

// escapePercent doubles every % so Expand leaves substituted text alone.
func escapePercent(s string) string { return strings.ReplaceAll(s, "%", "%%") }

// ExpandFileCommand expands the parameters of an editor, viewer or compare
// tool. files[0] replaces %1, files[1] %2 and so on (missing ones become
// empty); each is quoted when needed unless the template already puts it
// inside double quotes or contains %Q. All other placeholders behave as in
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
		if v != "" && !noQuote && !insideDoubleQuotes(body, i) {
			v = Quote(v, st)
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
