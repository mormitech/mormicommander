// Tests for the shared remote file system types: the fs.FileInfo adapter must
// report folders and links to folders the way the panel expects, and the
// progress wrappers must count bytes, report them and stop on cancellation.
package remotefs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"
)

func TestEntryInfo(t *testing.T) {
	now := time.Unix(1700000000, 0)
	tests := []struct {
		name    string
		e       Entry
		wantDir bool
		wantMod fs.FileMode
	}{
		{"file", Entry{Name: "a.txt", Size: 3, Mode: 0o644, ModTime: now}, false, 0o644},
		{"dir", Entry{Name: "d", Mode: fs.ModeDir | 0o755, IsDir: true}, true, fs.ModeDir | 0o755},
		{"dir without type bit", Entry{Name: "d", Mode: 0o755, IsDir: true}, true, fs.ModeDir | 0o755},
		{"link to dir", Entry{Name: "l", Mode: fs.ModeSymlink | 0o777, IsDir: true, IsLink: true}, true, fs.ModeSymlink | 0o777},
		{"link to file", Entry{Name: "l", Mode: fs.ModeSymlink | 0o777, IsLink: true}, false, fs.ModeSymlink | 0o777},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fi := tc.e.Info()
			if fi.Name() != tc.e.Name || fi.Size() != tc.e.Size || !fi.ModTime().Equal(tc.e.ModTime) {
				t.Fatalf("basic fields wrong: %v %v %v", fi.Name(), fi.Size(), fi.ModTime())
			}
			if fi.IsDir() != tc.wantDir {
				t.Errorf("IsDir = %v, want %v", fi.IsDir(), tc.wantDir)
			}
			if fi.Mode() != tc.wantMod {
				t.Errorf("Mode = %v, want %v", fi.Mode(), tc.wantMod)
			}
			if e, ok := fi.Sys().(Entry); !ok || e.Name != tc.e.Name {
				t.Errorf("Sys() did not return the entry")
			}
		})
	}
}

func TestJoin(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{[]string{"/a", "b"}, "/a/b"},
		{[]string{"/a/", "/b/"}, "/a/b"},
		{[]string{"a", "..", "c"}, "c"},
		{[]string{"/"}, "/"},
	}
	for _, tc := range tests {
		if got := Join(tc.in...); got != tc.want {
			t.Errorf("Join(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestProgressWriterAndReader(t *testing.T) {
	var reports []Progress
	var out bytes.Buffer
	w := &ProgressWriter{Ctx: context.Background(), W: &out, Path: "/x", Done: 10, Total: 20,
		Progress: func(p Progress) { reports = append(reports, p) }}
	if _, err := io.Copy(w, strings.NewReader("0123456789")); err != nil {
		t.Fatal(err)
	}
	if w.Done != 20 || out.String() != "0123456789" {
		t.Fatalf("Done=%d out=%q", w.Done, out.String())
	}
	if len(reports) == 0 || reports[len(reports)-1] != (Progress{Path: "/x", Done: 20, Total: 20}) {
		t.Fatalf("bad reports %v", reports)
	}

	reports = nil
	r := &ProgressReader{Ctx: context.Background(), R: strings.NewReader("abc"), Path: "/y", Total: 3,
		Progress: func(p Progress) { reports = append(reports, p) }}
	b, err := io.ReadAll(r)
	if err != nil || string(b) != "abc" || r.Done != 3 {
		t.Fatalf("read %q err %v done %d", b, err, r.Done)
	}
	if reports[len(reports)-1].Done != 3 {
		t.Fatalf("bad reports %v", reports)
	}
}

func TestProgressCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := &ProgressWriter{Ctx: ctx, W: io.Discard}
	if _, err := w.Write([]byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("writer err = %v", err)
	}
	r := &ProgressReader{Ctx: ctx, R: strings.NewReader("x")}
	if _, err := r.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("reader err = %v", err)
	}
	// A nil context must not panic.
	w2 := &ProgressWriter{W: io.Discard}
	if _, err := w2.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
}
