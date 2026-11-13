// This file converts file names between Go strings (always UTF-8) and the
// bytes the server expects. Modern servers use UTF-8, but old ones store
// names in a single-byte Western code page, and Total Commander lets the
// user pick the "codepage" per connection for exactly that reason. Latin-1
// is the one code page that maps every byte to a character and back without
// loss, so choosing it guarantees that a name read from a listing can be
// sent back in a command and still name the same file.
package ftp

import "strings"

func (c *Client) enc(s string) string {
	if c.latin1 {
		return toLatin1(s)
	}
	return s
}

func (c *Client) dec(s string) string {
	if c.latin1 {
		return fromLatin1(s)
	}
	return s
}

// toLatin1 encodes s as ISO-8859-1; characters outside it become '?'.
func toLatin1(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r < 256 {
			b = append(b, byte(r))
		} else {
			b = append(b, '?')
		}
	}
	return string(b)
}

// fromLatin1 decodes ISO-8859-1 bytes into a UTF-8 string.
func fromLatin1(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		b.WriteRune(rune(s[i]))
	}
	return b.String()
}
