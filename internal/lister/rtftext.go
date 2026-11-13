// RTF shown as plain text. Rich Text Format files are text with control
// words like {\b bold} and lots of header tables (fonts, colours, styles).
// Total Commander's Lister shows .rtf files as readable text, so we parse the
// format just enough to keep the words: paragraphs and line breaks become
// newlines, \tab a tab, special characters (\'e1 in the document's ANSI code
// page, \u337? Unicode escapes with their fallback characters, \emdash and
// friends) are decoded, and destinations that hold no visible text (font and
// colour tables, style sheets, document info, embedded pictures and objects,
// anything marked \*) are skipped.

package lister

import (
	"errors"
	"strconv"
	"strings"
)

// ErrNotRTF is returned when the data does not start like an RTF document.
var ErrNotRTF = errors.New("lister: not an RTF document")

var rtfSkipDest = map[string]bool{
	"fonttbl": true, "colortbl": true, "stylesheet": true, "info": true, "pict": true,
	"object": true, "header": true, "footer": true, "headerl": true, "headerr": true,
	"headerf": true, "footerl": true, "footerr": true, "footerf": true, "listtable": true,
	"listoverridetable": true, "revtbl": true, "rsidtbl": true, "generator": true,
	"xmlnstbl": true, "themedata": true, "colorschememapping": true, "latentstyles": true,
	"datastore": true, "filetbl": true, "fldinst": true, "bkmkstart": true, "bkmkend": true,
	"pgdsctbl": true, "operator": true, "author": true, "title": true, "comment": true,
}

var rtfSymbols = map[string]string{
	"par": "\n", "line": "\n", "sect": "\n\n", "page": "\n\n", "tab": "\t", "cell": "\t", "row": "\n",
	"emdash": "—", "endash": "–", "bullet": "•", "lquote": "‘", "rquote": "’",
	"ldblquote": "“", "rdblquote": "”", "emspace": "\u2003", "enspace": "\u2002", "qmspace": "\u2005",
	"~": "\u00a0", "_": "\u2011", "-": "",
}

// RTFToText extracts the readable text from an RTF document.
func RTFToText(data []byte) (string, error) {
	s := string(data)
	if !strings.HasPrefix(strings.TrimLeft(s, " \r\n\t"), `{\rtf`) {
		return "", ErrNotRTF
	}
	type state struct {
		skip bool
		uc   int // characters to skip after \uN
		ansi Encoding
	}
	st := state{uc: 1, ansi: Win1252}
	var stack []state
	var out strings.Builder
	var pendingBytes []byte // consecutive \'hh bytes (double-byte code pages)
	skipChars := 0          // fallback characters still to skip after \uN

	flush := func() {
		if len(pendingBytes) > 0 {
			if !st.skip {
				out.WriteString(DecodeString(st.ansi, pendingBytes))
			}
			pendingBytes = pendingBytes[:0]
		}
	}
	write := func(t string) {
		flush()
		if !st.skip {
			out.WriteString(t)
		}
	}
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '{':
			flush()
			stack = append(stack, st)
			i++
			// "{\*\dest ...}" is an optional destination: skip it unless known.
			if strings.HasPrefix(s[i:], `\*`) {
				st.skip = true
			}
		case '}':
			flush()
			if len(stack) > 0 {
				st = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			i++
		case '\\':
			i++
			if i >= len(s) {
				break
			}
			c2 := s[i]
			switch {
			case c2 == '\'':
				if i+3 <= len(s) {
					if b, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
						if skipChars > 0 {
							skipChars--
						} else {
							pendingBytes = append(pendingBytes, byte(b))
						}
					}
				}
				i += 3
			case c2 == '\\' || c2 == '{' || c2 == '}':
				if skipChars > 0 {
					skipChars--
				} else {
					write(string(c2))
				}
				i++
			case c2 == '\r' || c2 == '\n':
				write("\n")
				i++
			case isASCIILetter(c2):
				j := i
				for j < len(s) && isASCIILetter(s[j]) {
					j++
				}
				word := s[i:j]
				k := j
				if k < len(s) && (s[k] == '-' || s[k] >= '0' && s[k] <= '9') {
					k++
					for k < len(s) && s[k] >= '0' && s[k] <= '9' {
						k++
					}
				}
				param, hasParam := 0, k > j
				if hasParam {
					param, _ = strconv.Atoi(s[j:k])
				}
				if k < len(s) && s[k] == ' ' {
					k++ // the delimiter space belongs to the control word
				}
				i = k
				switch {
				case rtfSkipDest[word]:
					st.skip = true
				case word == "ansicpg" && hasParam:
					if e, ok := EncodingForCodepage(param); ok {
						st.ansi = e
					}
				case word == "mac":
					st.ansi = MacRoman
				case word == "pc":
					st.ansi = DOS437
				case word == "pca":
					st.ansi = DOS850
				case word == "uc" && hasParam:
					st.uc = param
				case word == "u" && hasParam:
					if param < 0 {
						param += 65536
					}
					write(string(rune(param)))
					skipChars = st.uc
				default:
					if t, ok := rtfSymbols[word]; ok {
						write(t)
					}
				}
				continue
			default:
				// Control symbols like \~ \- \_ \|
				if t, ok := rtfSymbols[string(c2)]; ok {
					write(t)
				}
				i++
			}
		case '\r', '\n':
			i++ // raw line breaks in RTF source are not text
		default:
			if skipChars > 0 {
				skipChars--
				i++
				continue
			}
			j := i
			for j < len(s) && s[j] != '\\' && s[j] != '{' && s[j] != '}' && s[j] != '\r' && s[j] != '\n' {
				j++
			}
			write(s[i:j])
			i = j
		}
	}
	flush()
	text := strings.ReplaceAll(out.String(), "\r", "")
	return strings.TrimRight(text, "\n ") + "\n", nil
}

func isASCIILetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
