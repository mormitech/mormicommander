// Tests for clipboard name formats.

package masks

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatEntries(t *testing.T) {
	dir := filepath.FromSlash("/home/me")
	when := time.Date(2024, 5, 1, 12, 30, 0, 0, time.UTC)
	entries := []Entry{
		{Name: "a b.txt", Size: 12, ModTime: when, Attr: "-a--"},
		{Name: "docs", IsDir: true, Size: -1, ModTime: when},
	}
	sep := string(filepath.Separator)
	tests := []struct {
		f    NameFormat
		want []string
	}{
		{FormatNames, []string{"a b.txt", "docs"}},
		{FormatFullNames, []string{filepath.Join(dir, "a b.txt"), filepath.Join(dir, "docs") + sep}},
		{FormatDetails, []string{"a b.txt\t12\t2024-05-01 12:30:00\t-a--", "docs\t<DIR>\t2024-05-01 12:30:00\t"}},
		{FormatFullDetails, []string{filepath.Join(dir, "a b.txt") + "\t12\t2024-05-01 12:30:00\t-a--",
			filepath.Join(dir, "docs") + "\t<DIR>\t2024-05-01 12:30:00\t"}},
		{FormatUnixPaths, []string{filepath.ToSlash(filepath.Join(dir, "a b.txt")), filepath.ToSlash(filepath.Join(dir, "docs")) + "/"}},
	}
	for _, tt := range tests {
		got := FormatEntries(dir, entries, tt.f, ClipOptions{})
		if want := strings.Join(tt.want, lineEnding); got != want {
			t.Errorf("format %d: got %q, want %q", tt.f, got, want)
		}
	}
	got := FormatEntries(dir, entries[:1], FormatDetails, ClipOptions{TimeLayout: "02.01.2006"})
	if !strings.Contains(got, "\t01.05.2024\t") {
		t.Errorf("layout: %q", got)
	}
	mapped := FormatEntries(dir, entries[:1], FormatFullNames, ClipOptions{MapNet: func(p string) string { return `\\srv\share\x` }})
	if mapped != `\\srv\share\x` {
		t.Errorf("MapNet: %q", mapped)
	}
}

func TestFileURL(t *testing.T) {
	tests := map[string]string{
		"/home/me/a b.txt": "file:///home/me/a%20b.txt",
		"C:/Data/x#1.txt":  "file:///C:/Data/x%231.txt",
		"//srv/share/x y":  "file://srv/share/x%20y",
	}
	for in, want := range tests {
		if got := fileURL(in); got != want {
			t.Errorf("fileURL(%q) = %q, want %q", in, got, want)
		}
	}
}
