// The "Open with" list on macOS: scan the application folders, match the file's extension and type identifiers
// against each app's declared document types, and mark the default handler: the user's choice from the LaunchServices
// preferences (see macapps.go for how), else the app NSWorkspace names for the file.
package launch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func platformApps(l *Launcher, path string) ([]App, error) {
	home := l.getenv("HOME")
	apps, extUTIs, conforms := scanMacApps(macAppDirs(home))
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	defaultID := ""
	if home != "" {
		ls := filepath.Join(home, "Library", "Preferences", "com.apple.LaunchServices", "com.apple.launchservices.secure.plist")
		if data, err := os.ReadFile(ls); err == nil {
			defaultID = macDefaultHandler(data, ext, utisForExt(ext, extUTIs, conforms))
		}
	}
	if defaultID == "" {
		// No choice of the user's own: ask LaunchServices (through NSWorkspace) which app opens the file now, which also
		// finds the system's default such as TextEdit for .txt.
		if appPath := macSystemDefaultApp(l, path); appPath != "" {
			found := false
			for _, a := range apps {
				if strings.EqualFold(filepath.Clean(a.path), filepath.Clean(appPath)) {
					defaultID, found = a.bundleID, a.bundleID != ""
					break
				}
			}
			if !found {
				if a, ok := readMacApp(appPath, extUTIs, conforms); ok && a.bundleID != "" {
					apps = append(apps, a)
					defaultID = a.bundleID
				}
			}
		}
	}
	return macAppsFor(ext, apps, extUTIs, conforms, defaultID), nil
}

// macDefaultScript prints the path of the app NSWorkspace would open the file in argv[0] with ("" when none).
const macDefaultScript = `function run(argv){ObjC.import("AppKit");var u=$.NSWorkspace.sharedWorkspace.URLForApplicationToOpenURL($.NSURL.fileURLWithPath(argv[0]));return u.isNil()?"":u.path.js}`

// macSystemDefaultApp returns the .app path that opens path by default, or "" when it cannot be told.
func macSystemDefaultApp(l *Launcher, path string) string {
	if l.Output == nil || path == "" {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := l.Output(ctx, Command{Path: "/usr/bin/osascript", Args: []string{"-l", "JavaScript", "-e", macDefaultScript, path}})
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(out))
	if !strings.HasSuffix(strings.ToLower(p), ".app") {
		return ""
	}
	return p
}

func platformAppsForMIME(l *Launcher, mimeType string) ([]App, error) { return nil, ErrUnsupported }
