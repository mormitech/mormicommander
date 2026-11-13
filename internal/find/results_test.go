// Tests for writing result lists as text (clipboard and "save results").

package find

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteResults(t *testing.T) {
	ts := time.Date(2026, 3, 4, 5, 6, 7, 0, time.Local)
	res := []Result{
		{Path: "/a/x.txt", Size: 12, ModTime: ts},
		{Path: "/a/sub", IsDir: true, ModTime: ts},
	}
	var b bytes.Buffer
	if err := WriteResults(&b, res, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if b.String() != "/a/x.txt\n/a/sub\n" {
		t.Errorf("plain: %q", b.String())
	}
	b.Reset()
	WriteResults(&b, res, WriteOptions{Details: true, Newline: "\r\n"})
	want := "/a/x.txt\t12\t2026-03-04 05:06:07\r\n/a/sub\t<DIR>\t2026-03-04 05:06:07\r\n"
	if b.String() != want {
		t.Errorf("details: %q", b.String())
	}
	b.Reset()
	WriteResults(&b, []Result{{Path: "1", DupGroup: 1}, {Path: "2", DupGroup: 1}, {Path: "3", DupGroup: 2}, {Path: "4", DupGroup: 2}}, WriteOptions{})
	if got := strings.Split(b.String(), "\n"); strings.Join(got, "|") != "1|2||3|4|" {
		t.Errorf("groups: %q", b.String())
	}
}
