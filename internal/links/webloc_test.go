// Tests for .webloc reading and writing: our XML output round-trips, Finder's
// XML layout with extra keys is read, binary plists (built here with a tiny
// encoder that follows Apple's bplist00 layout) are read in both ASCII and
// UTF-16 form, and malformed input fails with ErrBadShortcut.

package links

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"unicode/utf16"
)

// buildBplist encodes a dictionary of strings as bplist00 with one-byte
// object references and offsets (enough for small test dictionaries).
func buildBplist(keys, vals []string) []byte {
	var objs [][]byte
	enc := func(s string) []byte {
		ascii := true
		for _, r := range s {
			if r > 127 {
				ascii = false
			}
		}
		var b bytes.Buffer
		n := len(s)
		marker := byte(0x50)
		var payload []byte
		if ascii {
			payload = []byte(s)
		} else {
			marker = 0x60
			u := utf16.Encode([]rune(s))
			n = len(u)
			for _, c := range u {
				payload = binary.BigEndian.AppendUint16(payload, c)
			}
		}
		if n < 15 {
			b.WriteByte(marker | byte(n))
		} else {
			b.WriteByte(marker | 0x0F)
			b.WriteByte(0x10)
			b.WriteByte(byte(n))
		}
		b.Write(payload)
		return b.Bytes()
	}
	dict := []byte{0xD0 | byte(len(keys))}
	for i := range keys {
		dict = append(dict, byte(1+i))
	}
	for i := range vals {
		dict = append(dict, byte(1+len(keys)+i))
	}
	objs = append(objs, dict)
	for _, k := range keys {
		objs = append(objs, enc(k))
	}
	for _, v := range vals {
		objs = append(objs, enc(v))
	}
	var out bytes.Buffer
	out.WriteString("bplist00")
	var offsets []byte
	for _, o := range objs {
		offsets = append(offsets, byte(out.Len()))
		out.Write(o)
	}
	table := out.Len()
	out.Write(offsets)
	tr := make([]byte, 32)
	tr[6], tr[7] = 1, 1
	binary.BigEndian.PutUint64(tr[8:], uint64(len(objs)))
	binary.BigEndian.PutUint64(tr[16:], 0)
	binary.BigEndian.PutUint64(tr[24:], uint64(table))
	out.Write(tr)
	return out.Bytes()
}

func TestParseWebloc(t *testing.T) {
	finderXML := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Other</key>
	<dict><key>URL</key><string>http://nested.invalid/</string></dict>
	<key>Flag</key>
	<true/>
	<key>URL</key>
	<string>https://example.com/a?b=1&amp;c=2</string>
</dict>
</plist>`
	tests := []struct {
		name string
		data []byte
		want string
		err  error
	}{
		{"own xml", WeblocData("https://example.com/x?a=1&b=<2>"), "https://example.com/x?a=1&b=<2>", nil},
		{"finder xml", []byte(finderXML), "https://example.com/a?b=1&c=2", nil},
		{"binary ascii", buildBplist([]string{"URL"}, []string{"https://example.com/binary/path/long"}), "https://example.com/binary/path/long", nil},
		{"binary utf16", buildBplist([]string{"Other", "URL"}, []string{"x", "https://例え.jp/ü"}), "https://例え.jp/ü", nil},
		{"file url", WeblocData("file:///Users/me/doc.txt"), "file:///Users/me/doc.txt", nil},
		{"no url", []byte(`<plist><dict><key>X</key><string>y</string></dict></plist>`), "", ErrBadShortcut},
		{"not plist", []byte("hello"), "", ErrBadShortcut},
		{"bad binary", []byte("bplist00\x00\x01\x02"), "", ErrBadShortcut},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseWebloc(tc.data)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Errorf("err = %v, want %v", err, tc.err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestBinaryPlistTruncations(t *testing.T) {
	data := buildBplist([]string{"URL", "Name"}, []string{"https://example.com/", "a name that is long enough"})
	for n := 0; n < len(data); n++ {
		_, _ = ParsePlistStrings(data[:n])
	}
	// Corrupt every byte in turn; the parser must never panic.
	for i := 8; i < len(data); i++ {
		c := append([]byte(nil), data...)
		c[i] ^= 0xFF
		_, _ = ParsePlistStrings(c)
	}
}
