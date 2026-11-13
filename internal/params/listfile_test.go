// Tests for list-file encoding and writing in a temporary folder.
package params

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestEncodeList(t *testing.T) {
	if got := EncodeList([]string{"a", "é"}, ListUTF8, false); string(got) != "a\né\n" {
		t.Errorf("utf8 = %q", got)
	}
	if got := EncodeList([]string{"a"}, ListUTF8, true); string(got) != "a\r\n" {
		t.Errorf("crlf = %q", got)
	}
	want := []byte{0xFF, 0xFE, 'a', 0, 0xE9, 0, '\n', 0, 0x3D, 0xD8, 0x00, 0xDE, '\n', 0}
	if got := EncodeList([]string{"aé", "\U0001F600"}, ListUTF16, false); !bytes.Equal(got, want) {
		t.Errorf("utf16 = % x", got)
	}
	if got := EncodeList(nil, ListUTF8, true); len(got) != 0 {
		t.Errorf("empty = %q", got)
	}
}

func TestWriteListFile(t *testing.T) {
	dir := t.TempDir()
	f, err := WriteListFile(dir, []string{"x y", "z"}, ListUTF8, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f, dir) || !strings.HasSuffix(f, ".lst") {
		t.Errorf("name %q", f)
	}
	b, _ := os.ReadFile(f)
	if string(b) != "x y\nz\n" {
		t.Errorf("content %q", b)
	}
	if _, err := WriteListFile(dir+"/missing", nil, ListUTF8, false); err == nil {
		t.Error("missing dir should fail")
	}
}
