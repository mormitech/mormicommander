// Tests for .desktop entries, the XDG application lookup and MIME type guessing.
package launch

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseDesktopEntry(t *testing.T) {
	data := "[Desktop Entry]\nType=Application\nName=Editor\nName[de]=Bearbeiter\nExec=ed\\s%f\nMimeType=text/plain;text/x-c;\nTerminal=true\nNoDisplay=true\nIcon=ed\n[Desktop Action new]\nName=Other\n"
	e := ParseDesktopEntry([]byte(data), "de_DE")
	if e.Name != "Bearbeiter" || e.Exec != "ed %f" || !e.Terminal || !e.NoDisplay || e.Icon != "ed" || e.Type != "Application" {
		t.Errorf("entry = %+v", e)
	}
	if !reflect.DeepEqual(e.MimeTypes, []string{"text/plain", "text/x-c"}) {
		t.Errorf("mime types = %q", e.MimeTypes)
	}
	if ParseDesktopEntry([]byte(data), "fr").Name != "Editor" {
		t.Error("unknown language should give the plain name")
	}
	if !e.Handles("text/x-c") || e.Handles("image/png") {
		t.Error("Handles is wrong")
	}
	img := DesktopEntry{MimeTypes: []string{"image/*"}}
	if !img.Handles("image/png") {
		t.Error("wildcard type not handled")
	}
	if !reflect.DeepEqual(ParseDesktopEntry([]byte("no group"), ""), DesktopEntry{}) {
		t.Error("a file without [Desktop Entry] should give an empty entry")
	}
}

func TestExpandDesktopExec(t *testing.T) {
	tests := []struct {
		exec  string
		files []string
		want  []string
	}{
		{"gedit %U", []string{"/a b", "/c"}, []string{"gedit", "/a b", "/c"}},
		{"gedit %f", []string{"/a", "/c"}, []string{"gedit", "/a"}},
		{"gedit %f", nil, []string{"gedit"}},
		{"app --file=%f", []string{"/x"}, []string{"app", "--file=/x"}},
		{"app %i %c %k", nil, []string{"app", "--icon", "ico", "Name", "/loc.desktop"}},
		{"app 100%%", []string{"/x"}, []string{"app", "100%", "/x"}},
		{"app %d %m", []string{"/x"}, []string{"app", "/x"}},
		{`sh -c "echo \"hi\"" sh %F`, []string{"/x"}, []string{"sh", "-c", `echo "hi"`, "sh", "/x"}},
		{"nofilecode", []string{"/x"}, []string{"nofilecode", "/x"}},
	}
	for _, tt := range tests {
		got, err := ExpandDesktopExec(tt.exec, tt.files, "Name", "ico", "/loc.desktop")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ExpandDesktopExec(%q) = %q, want %q", tt.exec, got, tt.want)
		}
	}
	if _, err := ExpandDesktopExec(`app "open`, nil, "", "", ""); err == nil {
		t.Error("broken quoting should fail")
	}
}

func TestXDGApps(t *testing.T) {
	home := t.TempDir()
	sys := t.TempDir()
	data := filepath.Join(home, ".local", "share")
	app := func(dir, id, body string) {
		writeFile(t, filepath.Join(dir, "applications", id), "[Desktop Entry]\nType=Application\n"+body, 0o644)
	}
	app(data, "my-edit.desktop", "Name=My Edit\nExec=myedit %F\nMimeType=text/plain;\n")
	app(sys, "gedit.desktop", "Name=Gedit\nExec=gedit %U\nMimeType=text/plain;text/x-python;\n")
	app(sys, "viewer.desktop", "Name=Viewer\nExec=viewer %f\nMimeType=text/*;\n")
	app(sys, "hidden.desktop", "Name=Hidden\nExec=h %f\nMimeType=text/plain;\nHidden=true\n")
	app(sys, "removed.desktop", "Name=Removed\nExec=r %f\nMimeType=text/plain;\n")
	app(sys, "kde/kate.desktop", "Name=Kate\nExec=kate %U\n")
	// The user's copy of gedit.desktop hides the system one.
	app(data, "gedit.desktop", "Name=Gedit (mine)\nExec=gedit --new %U\nMimeType=text/plain;text/x-python;\n")
	writeFile(t, filepath.Join(home, ".config", "mimeapps.list"),
		"[Default Applications]\ntext/plain=missing.desktop;gedit.desktop;\n[Added Associations]\ntext/plain=kde-kate.desktop;\n[Removed Associations]\ntext/plain=removed.desktop\n", 0o644)
	env := map[string]string{"HOME": home, "XDG_DATA_DIRS": sys, "XDG_CONFIG_DIRS": filepath.Join(sys, "etc")}
	getenv := func(k string) string { return env[k] }

	apps := xdgAppsForMIME(xdgEnv{getenv: getenv}, "text/plain")
	sortApps(apps)
	var names []string
	for _, a := range apps {
		names = append(names, a.Name)
	}
	want := []string{"Gedit (mine)", "Kate", "My Edit", "Viewer"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("apps = %q, want %q", names, want)
	}
	if !apps[0].Default || apps[1].Default || apps[0].Exec != "gedit --new %U" {
		t.Errorf("default app = %+v", apps[0])
	}
	if apps[3].rank != 1 {
		t.Errorf("wildcard-only handler should rank lower: %+v", apps[3])
	}
	// A specific text type also offers plain text editors, after the real handlers.
	py := xdgAppsForMIME(xdgEnv{getenv: getenv}, "text/x-python")
	sortApps(py)
	if len(py) < 2 || py[0].Name != "Gedit (mine)" {
		t.Errorf("python apps = %+v", py)
	}

	l, _ := fakeLauncher(t, "linux", env)
	if _, loc, err := findDesktopEntry(l, "kde-kate.desktop"); err != nil || filepath.Base(loc) != "kate.desktop" {
		t.Errorf("findDesktopEntry by id: %q %v", loc, err)
	}
}

func TestMIMEType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "README"), "plain words\n", 0o644)
	writeFile(t, filepath.Join(dir, "blob"), "\x00\x01\x02", 0o644)
	writeFile(t, filepath.Join(dir, "empty"), "", 0o644)
	writeFile(t, filepath.Join(dir, "page.html"), "<p>", 0o644)
	tests := map[string]string{
		"README":    "text/plain",
		"blob":      "application/octet-stream",
		"empty":     "application/x-zerosize",
		"page.html": "text/html",
		".":         "inode/directory",
	}
	for name, want := range tests {
		if got := MIMEType(filepath.Join(dir, name)); got != want {
			t.Errorf("MIMEType(%s) = %q, want %q", name, got, want)
		}
	}
	if !validExceptTail([]byte("ab\xc3")) {
		t.Error("a cut-off UTF-8 sequence at the end should still count as text")
	}
}
