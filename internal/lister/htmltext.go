// HTML shown as plain text. The Lister can render HTML in the web view, but
// Total Commander also offers to show HTML "as text": the readable content
// with the markup removed, which is safer (no scripts, no remote images) and
// works for quick view of saved web pages and HTML e-mails. HTMLToText does
// that with a small forgiving tokenizer: it drops scripts, styles and
// comments, turns block elements into line breaks, list items into bullets,
// table cells into tab-separated columns, keeps <pre> blocks as they are,
// decodes entities, and collapses runs of whitespace the way a browser does.

package lister

import (
	"html"
	"regexp"
	"strings"
)

var blockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "dd": true, "div": true,
	"dl": true, "dt": true, "fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"form": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hr": true, "main": true, "nav": true, "ol": true, "p": true, "section": true,
	"table": true, "tr": true, "ul": true, "title": true, "caption": true, "details": true, "summary": true,
}

var skipTags = map[string]bool{"script": true, "style": true, "noscript": true, "template": true, "svg": true, "object": true}

var metaCharset = regexp.MustCompile(`(?i)<meta[^>]+charset\s*=\s*["']?\s*([a-zA-Z0-9_\-]+)`)

// HTMLCharset returns the encoding named by a <meta charset> or
// http-equiv Content-Type tag in the first bytes of an HTML file.
func HTMLCharset(head []byte) (Encoding, bool) {
	m := metaCharset.FindSubmatch(head[:min(len(head), 4096)])
	if m == nil {
		return "", false
	}
	return LookupEncoding(string(m[1]))
}

// HTMLToText converts an HTML document to readable plain text.
func HTMLToText(src string) string {
	var out strings.Builder
	pendingSpace := false
	newlines := 2 // treat the start as after a paragraph, so no leading blank lines
	pre := 0
	listDepth := 0
	var olCounters []int
	cellInRow := 0

	emitNewlines := func(n int) {
		for newlines < n {
			out.WriteByte('\n')
			newlines++
		}
		pendingSpace = false
	}
	emitText := func(s string) {
		if s == "" {
			return
		}
		if pre > 0 {
			out.WriteString(s)
			if strings.HasSuffix(s, "\n") {
				newlines = 1
			} else {
				newlines = 0
			}
			return
		}
		if isSpaceByte(s[0]) {
			pendingSpace = true
		}
		for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r < 0x80 && isSpaceByte(byte(r)) }) {
			if pendingSpace && newlines == 0 && out.Len() > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(f)
			newlines = 0
			pendingSpace = true
		}
		pendingSpace = isSpaceByte(s[len(s)-1]) || strings.TrimSpace(s) == "" && pendingSpace
	}

	i := 0
	for i < len(src) {
		lt := strings.IndexByte(src[i:], '<')
		if lt < 0 {
			emitText(html.UnescapeString(src[i:]))
			break
		}
		emitText(html.UnescapeString(src[i : i+lt]))
		i += lt
		if strings.HasPrefix(src[i:], "<!--") {
			end := strings.Index(src[i+4:], "-->")
			if end < 0 {
				break
			}
			i += 4 + end + 3
			continue
		}
		gt := tagEnd(src, i+1)
		if gt < 0 {
			emitText(src[i:])
			break
		}
		tag := src[i+1 : gt]
		i = gt + 1
		if strings.HasPrefix(tag, "!") || strings.HasPrefix(tag, "?") {
			continue
		}
		closing := strings.HasPrefix(tag, "/")
		name := strings.ToLower(tagName(strings.TrimPrefix(tag, "/")))
		if name == "" {
			emitText("<" + tag + ">")
			continue
		}
		if !closing && skipTags[name] {
			// Skip to the matching close tag.
			closeTag := "</" + name
			end := indexFold(src[i:], closeTag)
			if end < 0 {
				break
			}
			i += end
			if gt2 := strings.IndexByte(src[i:], '>'); gt2 >= 0 {
				i += gt2 + 1
			} else {
				break
			}
			continue
		}
		switch name {
		case "br":
			out.WriteByte('\n')
			newlines++
			pendingSpace = false
		case "pre", "textarea", "listing", "xmp":
			emitNewlines(1)
			if closing {
				pre = max(0, pre-1)
				emitNewlines(2)
			} else {
				pre++
			}
		case "li":
			if !closing {
				emitNewlines(1)
				out.WriteString(strings.Repeat("  ", max(0, listDepth-1)))
				if len(olCounters) > 0 && olCounters[len(olCounters)-1] >= 0 {
					olCounters[len(olCounters)-1]++
					out.WriteString(itoa(olCounters[len(olCounters)-1]) + ". ")
				} else {
					out.WriteString("• ")
				}
				newlines = 0
				pendingSpace = false
			}
		case "ul", "ol":
			if closing {
				listDepth = max(0, listDepth-1)
				if len(olCounters) > 0 {
					olCounters = olCounters[:len(olCounters)-1]
				}
			} else {
				listDepth++
				if name == "ol" {
					olCounters = append(olCounters, 0)
				} else {
					olCounters = append(olCounters, -1)
				}
			}
			emitNewlines(1)
		case "td", "th":
			if !closing {
				if cellInRow > 0 {
					out.WriteByte('\t')
					pendingSpace = false
				}
				cellInRow++
			}
		case "tr":
			cellInRow = 0
			emitNewlines(1)
		case "img":
			if alt := attr(tag, "alt"); alt != "" {
				emitText("[" + html.UnescapeString(alt) + "]")
			}
		case "hr":
			emitNewlines(1)
			out.WriteString("────────────────────")
			newlines = 0
			emitNewlines(1)
		case "p", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "table", "title":
			emitNewlines(2)
		default:
			if blockTags[name] {
				emitNewlines(1)
			}
		}
	}
	return strings.TrimRight(out.String(), " \n") + "\n"
}

// tagEnd finds the '>' that closes the tag starting at i, skipping quoted
// attribute values that may contain '>'.
func tagEnd(s string, i int) int {
	var q byte
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case q != 0:
			if c == q {
				q = 0
			}
		case c == '"' || c == '\'':
			q = c
		case c == '>':
			return i
		case c == '<' && i > 0:
			return -1
		}
	}
	return -1
}

func tagName(t string) string {
	for i := 0; i < len(t); i++ {
		c := t[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == ':') {
			return t[:i]
		}
	}
	return t
}

var attrRe = regexp.MustCompile(`(?is)\s([a-z][a-z0-9:_-]*)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)

func attr(tag, name string) string {
	for _, m := range attrRe.FindAllStringSubmatch(tag, -1) {
		if strings.EqualFold(m[1], name) {
			v := m[2]
			if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
				v = v[1 : len(v)-1]
			}
			return v
		}
	}
	return ""
}

func indexFold(s, sub string) int {
	n := len(sub)
	for i := 0; i+n <= len(s); i++ {
		if strings.EqualFold(s[i:i+n], sub) {
			return i
		}
	}
	return -1
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}
