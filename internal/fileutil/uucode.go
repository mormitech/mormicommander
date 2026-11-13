// UUE and XXE are the two classic ways of turning a binary file into plain
// text lines so it survives e-mail and Usenet. Both cut the data into lines
// of 45 bytes, write the line length as one character and then four
// characters for every three bytes; they only differ in the 64 characters
// used. UUE uses the ASCII range from space to underscore (with "`" in place
// of space, as modern encoders write it), XXE uses "+-", digits and letters
// so the text also survives gateways that mangle punctuation. This file holds
// the per-line encoding and decoding shared by both; encode.go builds the
// "begin 644 name" ... "end" frame around it.

package fileutil

const (
	uuLineBytes = 45
	xxTable     = "+-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// uuTable is the UUE alphabet with "`" for zero.
var uuTable = func() string {
	b := make([]byte, 64)
	b[0] = '`'
	for i := 1; i < 64; i++ {
		b[i] = byte(32 + i)
	}
	return string(b)
}()

// lineCodec encodes and decodes one UUE or XXE line.
type lineCodec struct {
	table string
	rev   [256]int8
	uu    bool
}

func newLineCodec(table string, uu bool) *lineCodec {
	c := &lineCodec{table: table, uu: uu}
	for i := range c.rev {
		c.rev[i] = -1
	}
	for i := 0; i < len(table); i++ {
		c.rev[table[i]] = int8(i)
	}
	if uu {
		// Old encoders wrote a space for zero; accept both.
		c.rev[' '] = 0
	}
	return c
}

var (
	uuCodec = newLineCodec(uuTable, true)
	xxCodec = newLineCodec(xxTable, false)
)

// encodeLine encodes up to 45 bytes into one line (without line ending).
func (c *lineCodec) encodeLine(dst []byte, src []byte) []byte {
	dst = append(dst, c.table[len(src)])
	for i := 0; i < len(src); i += 3 {
		var b [3]byte
		copy(b[:], src[i:])
		dst = append(dst,
			c.table[b[0]>>2],
			c.table[(b[0]&3)<<4|b[1]>>4],
			c.table[(b[1]&15)<<2|b[2]>>6],
			c.table[b[2]&63])
	}
	return dst
}

// terminator returns the zero-length line that precedes "end".
func (c *lineCodec) terminator() string { return string(c.table[0]) }

// decodeLine decodes one line. ok is false when the line is not a valid
// encoded line for this alphabet (wrong characters or too short for the
// length it announces), so the caller can skip junk between parts. Trailing
// padding some encoders add is ignored.
func (c *lineCodec) decodeLine(dst []byte, line string) (out []byte, n int, ok bool) {
	if line == "" {
		return dst, 0, false
	}
	l := c.rev[line[0]]
	if l < 0 || int(l) > uuLineBytes+18 {
		return dst, 0, false
	}
	n = int(l)
	need := (n + 2) / 3 * 4
	body := line[1:]
	if len(body) < need {
		// Some encoders drop trailing "spaces" of the last group; UUE lines
		// may therefore be short by up to the padding. Pad with zeros.
		if !c.uu || need-len(body) > 3 {
			return dst, 0, false
		}
		for len(body) < need {
			body += string(c.table[0])
		}
	}
	for i := 0; i < need; i++ {
		if c.rev[body[i]] < 0 {
			return dst, 0, false
		}
	}
	start := len(dst)
	for i := 0; i < need; i += 4 {
		a, b, cc, d := byte(c.rev[body[i]]), byte(c.rev[body[i+1]]), byte(c.rev[body[i+2]]), byte(c.rev[body[i+3]])
		dst = append(dst, a<<2|b>>4, b<<4|cc>>2, cc<<6|d)
	}
	return dst[:start+n], n, true
}

// validFor reports whether line is entirely made of the alphabet's
// characters and has the length its first character promises; it is used to
// tell UUE from XXE.
func (c *lineCodec) validFor(line string) bool {
	_, _, ok := c.decodeLine(nil, line)
	return ok
}
