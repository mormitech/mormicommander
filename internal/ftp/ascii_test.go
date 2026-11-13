// This file tests the line-ending converters used by ASCII transfers. They
// work on streams, so the tests feed them through readers that return one
// byte at a time as well as all at once, to be sure a CR at the end of one
// chunk and the LF at the start of the next are still recognised as a pair.
package ftp

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestCRLFToLF(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"a\r\nb\r\n", "a\nb\n"},
		{"lone\rcr", "lone\rcr"},
		{"end with cr\r", "end with cr\r"},
		{"\r\r\n", "\r\n"},
		{"no newline", "no newline"},
		{"mixed\nlf\r\ncrlf", "mixed\nlf\ncrlf"},
	}
	for _, tt := range tests {
		for _, mode := range []string{"whole", "onebyte"} {
			var r io.Reader = strings.NewReader(tt.in)
			if mode == "onebyte" {
				r = iotest.OneByteReader(r)
			}
			got, err := io.ReadAll(&crlfToLF{r: r})
			if err != nil || string(got) != tt.want {
				t.Errorf("%s %q -> %q, %v; want %q", mode, tt.in, got, err, tt.want)
			}
		}
	}
	// Small destination buffers.
	got, err := io.ReadAll(iotest.OneByteReader(&crlfToLF{r: strings.NewReader("a\r\nb\rc\r\n")}))
	if err != nil || string(got) != "a\nb\rc\n" {
		t.Errorf("one-byte reads: %q %v", got, err)
	}
}

func TestLFToCRLF(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"a\nb\n", "a\r\nb\r\n"},
		{"already\r\nok", "already\r\nok"},
		{"\n\n", "\r\n\r\n"},
		{"lone\rcr", "lone\rcr"},
	}
	for _, tt := range tests {
		for _, mode := range []string{"whole", "onebyte"} {
			var r io.Reader = strings.NewReader(tt.in)
			if mode == "onebyte" {
				r = iotest.OneByteReader(r)
			}
			conv := &lfToCRLF{r: r}
			got, err := io.ReadAll(conv)
			if err != nil || string(got) != tt.want {
				t.Errorf("%s %q -> %q, %v; want %q", mode, tt.in, got, err, tt.want)
			}
			if conv.in != int64(len(tt.in)) {
				t.Errorf("%s: counted %d input bytes, want %d", mode, conv.in, len(tt.in))
			}
		}
	}
	got, err := io.ReadAll(iotest.OneByteReader(&lfToCRLF{r: strings.NewReader("\n\nx\n")}))
	if err != nil || string(got) != "\r\n\r\nx\r\n" {
		t.Errorf("one-byte destination: %q %v", got, err)
	}
	// Reader built for io.Copy-sized buffers.
	big := strings.Repeat("\n", 100000)
	got, err = io.ReadAll(&lfToCRLF{r: strings.NewReader(big)})
	if err != nil || string(got) != strings.Repeat("\r\n", 100000) {
		t.Errorf("large input: %d bytes, %v", len(got), err)
	}
}
