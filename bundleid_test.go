// Tests for the app's bundle identifier (issue #115): wails.json holds it, and the macOS Info.plist and the Windows
// manifest take it from there through the @BUNDLE_ID@ placeholder that scripts/build.sh fills in, so the old Wails
// default (com.wails.mormicommander) can't come back in one of them by accident.
package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestBundleIDIsMormitech(t *testing.T) {
	id := bundleID()
	if id != "com.mormitech.mormicommander" {
		t.Fatalf("bundleID() = %q, want com.mormitech.mormicommander", id)
	}
	// Apple allows letters, digits, hyphens and dots in a CFBundleIdentifier; reverse-DNS with at least three parts
	if !regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+){2,}$`).MatchString(id) {
		t.Fatalf("bundleID() = %q is not a valid reverse-DNS identifier", id)
	}
}

func TestBuildFilesUseBundleIDPlaceholder(t *testing.T) {
	for _, f := range []string{"build/darwin/Info.plist", "build/windows/wails.exe.manifest"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if strings.Contains(s, "com.wails.") {
			t.Errorf("%s still names the Wails default identifier", f)
		}
		if !strings.Contains(s, "@BUNDLE_ID@") {
			t.Errorf("%s does not take the identifier from wails.json (@BUNDLE_ID@)", f)
		}
	}
	plist, _ := os.ReadFile("build/darwin/Info.plist")
	if !regexp.MustCompile(`<key>CFBundleIdentifier</key>\s*<string>@BUNDLE_ID@</string>`).Match(plist) {
		t.Error("Info.plist: CFBundleIdentifier is not @BUNDLE_ID@")
	}
	sh, err := os.ReadFile("scripts/build.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sh), "s|@BUNDLE_ID@|$(info bundleIdentifier)|g") {
		t.Error("scripts/build.sh does not fill in @BUNDLE_ID@ from wails.json")
	}
}
