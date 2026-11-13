// Quoting and splitting of command lines. Total Commander stores a program's parameters as one line of text
// ("-n %P%N"), so that text has to be split into arguments on macOS and Linux, and quoted correctly into the single
// command line a Windows program receives. Three dialects live here: Windows argument quoting (the rules of
// CommandLineToArgvW, which most Windows programs use), POSIX shell quoting for the few places where a real shell
// runs a line the user typed, and the simple splitting rules used for parameter text on macOS and Linux.
//
// The splitting rules are deliberately small and predictable: whitespace separates arguments, double quotes group,
// single quotes group literally, and a backslash makes the next character literal everywhere except inside single
// quotes. EscapeArg produces text that survives those rules unchanged, which is what lets placeholder expansion
// insert any file name safely, quoted by the user or not.
package launch

import (
	"errors"
	"strings"
)

// ErrUnterminatedQuote is returned by SplitArgs for text with an opening quote that is never closed.
var ErrUnterminatedQuote = errors.New("launch: unterminated quote")

// SplitArgs splits parameter text into arguments with the macOS/Linux rules described in the file comment.
func SplitArgs(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inArg := false
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case quote == '\'':
			if ch == '\'' {
				quote = 0
			} else {
				cur.WriteByte(ch)
			}
		case ch == '\\':
			inArg = true
			if i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			} else {
				cur.WriteByte(ch)
			}
		case quote == '"':
			if ch == '"' {
				quote = 0
			} else {
				cur.WriteByte(ch)
			}
		case ch == '"' || ch == '\'':
			quote = ch
			inArg = true
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteByte(ch)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, ErrUnterminatedQuote
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}

// EscapeArg makes s one literal piece under SplitArgs rules, whether or not it ends up inside double quotes: every
// backslash, quote and whitespace character gets a backslash in front of it.
func EscapeArg(s string) string {
	if !strings.ContainsAny(s, "\\\"' \t\n\r") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '"', '\'', ' ', '\t', '\n', '\r':
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// QuoteWindowsArg quotes one argument for a Windows command line so that CommandLineToArgvW gives it back
// unchanged: it is wrapped in double quotes when it is empty or has spaces, tabs or quotes, inner quotes get a
// backslash, and backslashes in front of a quote (or of the closing quote) are doubled.
func QuoteWindowsArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\v\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '\\':
			// Written once now; doubled later only if a quote follows.
			slashes++
			b.WriteByte('\\')
		case '"':
			b.WriteString(strings.Repeat("\\", slashes+1))
			b.WriteByte('"')
			slashes = 0
		default:
			slashes = 0
			b.WriteByte(ch)
		}
	}
	b.WriteString(strings.Repeat("\\", slashes))
	b.WriteByte('"')
	return b.String()
}

// SplitWindowsArgs splits a Windows command line the way CommandLineToArgvW does (and the C runtime of most
// programs): used to find the program at the start of a line the user typed.
func SplitWindowsArgs(s string) []string {
	var args []string
	var cur strings.Builder
	inArg, inQuote := false, false
	for i := 0; i < len(s); {
		ch := s[i]
		switch {
		case ch == '\\':
			n := 0
			for i < len(s) && s[i] == '\\' {
				n++
				i++
			}
			if i < len(s) && s[i] == '"' {
				cur.WriteString(strings.Repeat("\\", n/2))
				if n%2 == 1 {
					cur.WriteByte('"')
					i++
				}
			} else {
				cur.WriteString(strings.Repeat("\\", n))
			}
			inArg = true
			continue
		case ch == '"':
			inArg = true
			if inQuote && i+1 < len(s) && s[i+1] == '"' {
				cur.WriteByte('"')
				i += 2
				continue
			}
			inQuote = !inQuote
		case (ch == ' ' || ch == '\t') && !inQuote:
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteByte(ch)
			inArg = true
		}
		i++
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args
}

// ShellQuote quotes s for a POSIX shell: it is left alone when it only has safe characters, otherwise it is put in
// single quotes, with each single quote written as '\”.
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("@%_-+=:,./", c) >= 0) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// QuoteName quotes a file name for display in parameters or the command line the way Total Commander does: on
// Windows a name with a space gets double quotes; elsewhere names are made safe for the shell.
func QuoteName(osName, s string) string {
	if osName == "windows" {
		if strings.ContainsAny(s, " \t") || s == "" {
			return `"` + s + `"`
		}
		return s
	}
	return ShellQuote(s)
}
