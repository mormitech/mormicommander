// The "Open with" list on macOS: scan the application folders, match the file's extension and type identifiers
// against each app's declared document types, and mark the user's default handler from the LaunchServices
// preferences (see macapps.go for how).
package launch

import (
	"os"
	"path/filepath"
	"strings"
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
	return macAppsFor(ext, apps, extUTIs, conforms, defaultID), nil
}

func platformAppsForMIME(l *Launcher, mimeType string) ([]App, error) { return nil, ErrUnsupported }
