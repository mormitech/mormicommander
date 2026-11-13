// These tests check the Linux file-list formats: that file URIs are escaped exactly like glib's g_filename_to_uri
// (so the strings match Nautilus), that parsing accepts the variants real tools write (localhost host, single slash,
// LF line ends, comments), and that the GNOME copy/cut format round-trips. Windows path forms are only checked when the
// tests run on Windows, since drive letters mean nothing on other systems.

package clipfiles

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestFileURI(t *testing.T) {
	if runtime.GOOS == "windows" {
		for in, want := range map[string]string{
			`C:\a b\c.txt`:        "file:///C:/a%20b/c.txt",
			`\\srv\share\x y.txt`: "file://srv/share/x%20y.txt",
		} {
			if got := FileURI(in); got != want {
				t.Errorf("FileURI(%q) = %q, want %q", in, got, want)
			}
		}
		return
	}
	tests := []struct{ in, want string }{
		{"/tmp/a.txt", "file:///tmp/a.txt"},
		{"/tmp/a b.txt", "file:///tmp/a%20b.txt"},
		{"/tmp/ünï", "file:///tmp/%C3%BCn%C3%AF"},
		{"/tmp/100%#?", "file:///tmp/100%25%23%3F"},
		{"/tmp/keep!$&'()*+,;=:@~-._", "file:///tmp/keep!$&'()*+,;=:@~-._"},
		{`/tmp/back\slash`, "file:///tmp/back%5Cslash"},
		{"/tmp/new\nline", "file:///tmp/new%0Aline"},
	}
	for _, tt := range tests {
		if got := FileURI(tt.in); got != tt.want {
			t.Errorf("FileURI(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if back, ok := PathFromURI(tt.want); !ok || back != tt.in {
			t.Errorf("PathFromURI(%q) = %q, %v; want %q", tt.want, back, ok, tt.in)
		}
	}
}

func TestPathFromURI(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"file:///tmp/a%20b", "/tmp/a b", true},
		{"file://localhost/tmp/x", "/tmp/x", true},
		{"FILE:///tmp/x", "/tmp/x", true},
		{"file:/tmp/x", "/tmp/x", true},
		{"file:///tmp/x?query#frag", "/tmp/x", true},
		{"  file:///tmp/x  ", "/tmp/x", true},
		{"http://example.com/x", "", false},
		{"trash:///x", "", false},
		{"file://", "", false},
		{"file:relative", "", false},
		{"file:///tmp/%00", "", false},
		{"file:///tmp/%zz", "", false},
		{"/tmp/plain", "", false},
	}
	if runtime.GOOS != "windows" {
		tests = append(tests, struct {
			in   string
			want string
			ok   bool
		}{"file://otherhost/tmp/x", "", false})
	}
	for _, tt := range tests {
		got, ok := PathFromURI(tt.in)
		if ok != tt.ok || (ok && got != filepath.FromSlash(tt.want)) {
			t.Errorf("PathFromURI(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestURIList(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix paths")
	}
	paths := []string{"/a/one.txt", "/b/two words"}
	enc := EncodeURIList(paths)
	if want := "file:///a/one.txt\r\nfile:///b/two%20words\r\n"; enc != want {
		t.Fatalf("EncodeURIList = %q, want %q", enc, want)
	}
	if got := ParseURIList(enc); !reflect.DeepEqual(got, paths) {
		t.Fatalf("round trip = %q", got)
	}
	messy := "# comment\nfile:///a/one.txt\n\nhttp://skip.me/\r\nfile:///b/two%20words"
	if got := ParseURIList(messy); !reflect.DeepEqual(got, paths) {
		t.Fatalf("ParseURIList(messy) = %q", got)
	}
}

func TestGnomeCopied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix paths")
	}
	paths := []string{"/a/one.txt", "/b/two"}
	for _, op := range []Op{Copy, Cut} {
		enc := EncodeGnomeCopied(paths, op)
		want := op.String() + "\nfile:///a/one.txt\nfile:///b/two"
		if enc != want {
			t.Fatalf("EncodeGnomeCopied = %q, want %q", enc, want)
		}
		got, gotOp, ok := ParseGnomeCopied(enc + "\n") // a trailing newline must not matter
		if !ok || gotOp != op || !reflect.DeepEqual(got, paths) {
			t.Fatalf("ParseGnomeCopied = %q, %v, %v", got, gotOp, ok)
		}
	}
	if _, _, ok := ParseGnomeCopied("file:///a\nfile:///b"); ok {
		t.Fatal("a plain URI list must not parse as the GNOME format")
	}
	if _, op, ok := ParseGnomeCopied("cut\r\nfile:///a"); !ok || op != Cut {
		t.Fatal("CRLF after the op line must be accepted")
	}
}
