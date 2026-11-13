// Tests for the INI-style shortcut formats (.url and .desktop), the Exec
// command splitter, and the conversions between local paths and file://
// URLs that every writable shortcut format relies on.

package links

import (
	"errors"
	"runtime"
	"testing"
)

func TestParseURLShortcut(t *testing.T) {
	data := "\xEF\xBB\xBF[DEFAULT]\r\nBASEURL=http://other/\r\n[InternetShortcut]\r\nURL=https://example.com/\r\nWorkingDirectory=C:\\w\r\nIconFile=C:\\i.ico\r\nURL=http://second/\r\n"
	s, err := ParseURLShortcut([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if s.URL != "https://example.com/" || s.WorkingDir != `C:\w` || s.Icon != `C:\i.ico` {
		t.Errorf("got %+v", s)
	}
	if got, err := ParseURLShortcut(URLShortcutData("ftp://h/x")); err != nil || got.URL != "ftp://h/x" {
		t.Errorf("round trip: %+v %v", got, err)
	}
	for _, bad := range []string{"", "[InternetShortcut]\nX=1", "URL=http://no-group/"} {
		if _, err := ParseURLShortcut([]byte(bad)); !errors.Is(err, ErrBadShortcut) {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
}

func TestParseDesktopEntry(t *testing.T) {
	tests := []struct {
		name string
		data string
		want IniShortcut
		err  bool
	}{
		{
			name: "link",
			data: "# comment\n[Desktop Entry]\nType=Link\nName=Docs\\sfolder\nName[de]=Dokumente\nURL=file:///home/me/Docs\nIcon=folder\n[Other]\nType=X\n",
			want: IniShortcut{Type: "Link", Name: "Docs folder", URL: "file:///home/me/Docs", Icon: "folder"},
		},
		{
			name: "application",
			data: "[Desktop Entry]\nType=Application\nName=Editor\nExec=\"/opt/my editor/bin/ed\" --new %F\nPath=/tmp\nComment=Edits\n",
			want: IniShortcut{Type: "Application", Name: "Editor", Exec: `"/opt/my editor/bin/ed" --new %F`, WorkingDir: "/tmp", Comment: "Edits"},
		},
		{name: "no type", data: "[Desktop Entry]\nName=x\n", err: true},
		{name: "no group", data: "Type=Link\n", err: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDesktopEntry([]byte(tc.data))
			if tc.err {
				if !errors.Is(err, ErrBadShortcut) {
					t.Errorf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if *got != tc.want {
				t.Errorf("got %+v\nwant %+v", *got, tc.want)
			}
		})
	}
	data := DesktopLinkData(" lead\\name\n", "https://example.com/")
	got, err := ParseDesktopEntry(data)
	if err != nil || got.Name != " lead\\name\n" || got.URL != "https://example.com/" || got.Type != "Link" {
		t.Errorf("round trip: %+v %v", got, err)
	}
}

func TestExecArgs(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"gedit %U", []string{"gedit"}},
		{`"/opt/my app/run" --x "a \"q\"" %f`, []string{"/opt/my app/run", "--x", `a "q"`}},
		{"echo 100%%", []string{"echo", "100%"}},
		{"  ", nil},
	}
	for _, tc := range tests {
		got := ExecArgs(tc.in)
		if !equal(got, tc.want) {
			t.Errorf("ExecArgs(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFileURL(t *testing.T) {
	tests := []struct {
		path string
		url  string
	}{
		{`C:\Program Files\a#b.txt`, "file:///C:/Program%20Files/a%23b.txt"},
		{`\\server\share\x y`, "file://server/share/x%20y"},
	}
	if runtime.GOOS != "windows" {
		tests = append(tests, struct{ path, url string }{"/home/me/a b.txt", "file:///home/me/a%20b.txt"})
	}
	for _, tc := range tests {
		if got := FileURL(tc.path); got != tc.url {
			t.Errorf("FileURL(%q) = %q, want %q", tc.path, got, tc.url)
		}
		back, ok := PathFromFileURL(tc.url)
		if !ok || back != tc.path {
			t.Errorf("PathFromFileURL(%q) = %q %v, want %q", tc.url, back, ok, tc.path)
		}
	}
	if _, ok := PathFromFileURL("https://example.com/"); ok {
		t.Error("https treated as file URL")
	}
	if p, ok := PathFromFileURL("file://localhost/etc/hosts"); !ok || p != fromSlashForTest("/etc/hosts") {
		t.Errorf("localhost: %q", p)
	}
}

func fromSlashForTest(p string) string {
	if runtime.GOOS == "windows" {
		return `\etc\hosts`
	}
	return p
}

func TestIsURL(t *testing.T) {
	tests := map[string]bool{
		"https://x":     true,
		"mailto:a@b":    true,
		"file:///x":     true,
		"svn+ssh://h/r": true,
		`C:\x`:          false,
		"c:/x":          false,
		"/usr/bin":      false,
		"relative/a:b":  false,
		"9p://x":        false,
		"":              false,
	}
	for in, want := range tests {
		if got := IsURL(in); got != want {
			t.Errorf("IsURL(%q) = %v, want %v", in, got, want)
		}
	}
}
