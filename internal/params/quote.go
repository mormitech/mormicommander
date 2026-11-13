// Quoting and splitting of command lines. Total Commander builds a single
// command-line string (with names in double quotes when they contain spaces)
// and hands it to Windows, which splits it again. We keep that model because
// users edit the expanded string in the "?" prompt and in the command-line
// box, but we never pass the string to a shell unless the user typed shell
// syntax: SplitArgs turns it back into an argument list that can be given to
// exec directly. Windows lines follow the CommandLineToArgvW rules (double
// quotes, backslashes are only special before a quote); POSIX lines follow
// sh rules (single quotes, double quotes, backslash escapes), so a quoted
// name always survives the round trip unchanged.
package params

import (
	"errors"
	"strings"
)

// Style selects the quoting and path rules: Windows or POSIX.
type Style int

const (
	// StyleWindows uses double quotes and backslash path separators.
	StyleWindows Style = iota + 1
	// StylePOSIX uses sh quoting rules and "/" path separators.
	StylePOSIX
)

// orDefault returns the style itself, or DefaultStyle when it is unset.
func (st Style) orDefault() Style {
	if st == StyleWindows || st == StylePOSIX {
		return st
	}
	return DefaultStyle
}

// Sep returns the path separator of the style.
func (st Style) Sep() string {
	if st.orDefault() == StyleWindows {
		return `\`
	}
	return "/"
}

// ErrUnterminatedQuote is returned by SplitArgs for a POSIX line with an
// open quote or a trailing backslash.
var ErrUnterminatedQuote = errors.New("params: unterminated quote")

// NeedsQuote reports whether s must be quoted to stay one argument. Windows
// follows Total Commander and only quotes for blanks (and for a double quote
// or an empty string, which would otherwise vanish); POSIX quotes anything
// that is not plainly safe for sh.
func NeedsQuote(s string, st Style) bool {
	if s == "" {
		return true
	}
	if st.orDefault() == StyleWindows {
		return strings.ContainsAny(s, " \t\"")
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 0x80:
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("_@%+=:,./-", c) >= 0:
		default:
			return true
		}
	}
	return false
}

// Quote returns s quoted for the style when it needs quoting, else s.
func Quote(s string, st Style) string {
	if !NeedsQuote(s, st) {
		return s
	}
	return QuoteAlways(s, st)
}

// QuoteAlways quotes s even when it would not need it.
func QuoteAlways(s string, st Style) string {
	if st.orDefault() == StylePOSIX {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	var b strings.Builder
	b.WriteByte('"')
	b.WriteString(escapeWindowsInQuotes(s, true))
	b.WriteByte('"')
	return b.String()
}

// escapeWindowsInQuotes escapes s for the inside of a Windows double-quoted
// argument: backslashes in front of a quote are doubled and the quote gets a
// backslash; when closing is true, trailing backslashes are doubled too so
// the closing quote stays a quote ("C:\dir\" would otherwise eat it).
func escapeWindowsInQuotes(s string, closing bool) string {
	var b strings.Builder
	slashes := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			slashes++
			b.WriteByte(c)
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes+1))
			b.WriteByte('"')
			slashes = 0
		default:
			slashes = 0
			b.WriteByte(c)
		}
	}
	if closing {
		b.WriteString(strings.Repeat(`\`, slashes))
	}
	return b.String()
}

// escapePOSIXInDouble escapes s for the inside of a POSIX double-quoted word.
func escapePOSIXInDouble(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte("\\\"$`", s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// JoinArgs quotes each argument as needed and joins them with spaces.
func JoinArgs(args []string, st Style) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = Quote(a, st)
	}
	return strings.Join(q, " ")
}

// SplitArgs splits a command line into arguments using the style's rules.
// Windows lines never fail (an open quote runs to the end, as in Windows);
// POSIX lines return ErrUnterminatedQuote for an open quote.
func SplitArgs(line string, st Style) ([]string, error) {
	if st.orDefault() == StyleWindows {
		return splitWindows(line), nil
	}
	return splitPOSIX(line)
}

// splitWindows implements the CommandLineToArgvW / msvcrt 2008 rules.
func splitWindows(line string) []string {
	var args []string
	var cur strings.Builder
	inArg, inQuote := false, false
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case (c == ' ' || c == '\t') && !inQuote:
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
			i++
		case c == '\\':
			n := 0
			for i < len(line) && line[i] == '\\' {
				n++
				i++
			}
			inArg = true
			if i < len(line) && line[i] == '"' {
				cur.WriteString(strings.Repeat(`\`, n/2))
				if n%2 == 1 {
					cur.WriteByte('"')
					i++
				}
			} else {
				cur.WriteString(strings.Repeat(`\`, n))
			}
		case c == '"':
			inArg = true
			if inQuote && i+1 < len(line) && line[i+1] == '"' {
				cur.WriteByte('"')
				i += 2
				continue
			}
			inQuote = !inQuote
			i++
		default:
			inArg = true
			cur.WriteByte(c)
			i++
		}
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args
}

// splitPOSIX implements sh word splitting without expansions.
func splitPOSIX(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inArg := false
	var q byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch q {
		case '\'':
			if c == '\'' {
				q = 0
			} else {
				cur.WriteByte(c)
			}
			continue
		case '"':
			switch {
			case c == '"':
				q = 0
			case c == '\\' && i+1 < len(line) && strings.IndexByte("\\\"$`\n", line[i+1]) >= 0:
				i++
				if line[i] != '\n' {
					cur.WriteByte(line[i])
				}
			default:
				cur.WriteByte(c)
			}
			continue
		}
		switch c {
		case ' ', '\t', '\n':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		case '\'', '"':
			q = c
			inArg = true
		case '\\':
			if i+1 >= len(line) {
				return nil, ErrUnterminatedQuote
			}
			i++
			inArg = true
			if line[i] != '\n' {
				cur.WriteByte(line[i])
			}
		default:
			inArg = true
			cur.WriteByte(c)
		}
	}
	if q != 0 {
		return nil, ErrUnterminatedQuote
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}

// InsertName appends a file name (or full path) to a command-line text the
// way Ctrl+Enter / Ctrl+Shift+Enter do: quoted when needed and separated
// from the existing text by one space. Pressing the key twice therefore
// gives two names one after the other (F-813).
func InsertName(line, name string, st Style) string {
	q := Quote(name, st)
	if line == "" || strings.HasSuffix(line, " ") {
		return line + q
	}
	return line + " " + q
}
