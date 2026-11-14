// Tests for the CI workflow's speed settings (issue #118): Go is pinned to the version go.mod names instead of
// "stable", the Go and npm downloads are cached between runs, and the wails3 tool that makes the Windows .syso is
// built once and cached at the same Wails version the app uses. The tests read .github/workflows/build.yml and
// scripts/build.sh as text, so a later edit can't quietly drop the pin or let the tool versions drift apart.
package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// readText returns the file's contents, failing the test when it can't be read.
func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// wailsModuleVersion is the github.com/wailsapp/wails/v3 version go.mod requires.
func wailsModuleVersion(t *testing.T) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^\s*github\.com/wailsapp/wails/v3 (v\S+)`).FindStringSubmatch(readText(t, "go.mod"))
	if m == nil {
		t.Fatal("go.mod does not require github.com/wailsapp/wails/v3")
	}
	return m[1]
}

func TestCIPinsGoToGoMod(t *testing.T) {
	yml := readText(t, ".github/workflows/build.yml")
	if strings.Contains(yml, "go-version: stable") {
		t.Error("build.yml still asks for go-version: stable")
	}
	if !strings.Contains(yml, "go-version-file: go.mod") {
		t.Error("build.yml does not take the Go version from go.mod")
	}
	if !regexp.MustCompile(`(?m)^go \d+\.\d+\.\d+\r?$`).MatchString(readText(t, "go.mod")) {
		t.Error("go.mod's go line is not an exact version (x.y.z), so the CI pin would float")
	}
}

func TestCICachesDownloads(t *testing.T) {
	yml := readText(t, ".github/workflows/build.yml")
	for _, want := range []string{"cache-dependency-path: go.sum", "cache: npm", "cache-dependency-path: frontend/package-lock.json"} {
		if !strings.Contains(yml, want) {
			t.Errorf("build.yml lacks %q", want)
		}
	}
}

func TestWails3ToolVersionMatchesGoMod(t *testing.T) {
	v := wailsModuleVersion(t)
	tool := regexp.MustCompile(`wails/v3/cmd/wails3@(v[0-9][0-9A-Za-z.+-]*)`)
	for _, f := range []string{".github/workflows/build.yml", "scripts/build.sh"} {
		found := tool.FindAllStringSubmatch(readText(t, f), -1)
		if len(found) == 0 {
			t.Errorf("%s does not name the wails3 tool", f)
		}
		for _, m := range found {
			if m[1] != v {
				t.Errorf("%s uses wails3@%s, go.mod has wails %s", f, m[1], v)
			}
		}
	}
	yml := readText(t, ".github/workflows/build.yml")
	if !strings.Contains(yml, "key: wails3-"+v+"-") {
		t.Errorf("build.yml's wails3 cache key does not carry version %s", v)
	}
	if !strings.Contains(yml, "WAILS3=") {
		t.Error("build.yml does not hand the cached wails3 to build.sh through WAILS3")
	}
	if !strings.Contains(readText(t, "scripts/build.sh"), `"${WAILS3:-}"`) {
		t.Error("scripts/build.sh does not use WAILS3 when it is set")
	}
}

// The live integration tests (issue #106) stay wired up: CI runs them with the live tag against the servers
// scripts/live_servers.sh starts, a missing server fails instead of skipping, and every live test file carries the
// build tag, so a plain `go test ./...` never compiles them.
func TestCIRunsLiveTests(t *testing.T) {
	yml := readText(t, ".github/workflows/build.yml")
	for _, want := range []string{"go test -tags live", "scripts/live_servers.sh", "live.env"} {
		if !strings.Contains(yml, want) {
			t.Errorf("build.yml lacks %q", want)
		}
	}
	if !strings.Contains(readText(t, "scripts/live_servers.sh"), "MC_LIVE_REQUIRE=all") {
		t.Error("scripts/live_servers.sh does not make every live server required")
	}
	files, _ := filepath.Glob("live_*_test.go")
	if len(files) == 0 {
		t.Fatal("no live_*_test.go files")
	}
	for _, f := range files {
		if !strings.HasPrefix(readText(t, f), "//go:build live\n") {
			t.Errorf("%s does not start with //go:build live", f)
		}
	}
}
