// The "Open with" list (F-200, F-703, F-708): which installed applications can open a given file, with the
// system's default one marked. There is no portable API for this, so each platform reads its own records: the
// Info.plist of every application on macOS together with the LaunchServices defaults, the .desktop files and
// mimeapps.list on Linux and the BSDs, and the registry on Windows. The platform parts are in apps_<os>.go; the
// parsing they share is in macapps.go and xdgapps.go, which have no platform code so they are tested everywhere.
package launch

import (
	"sort"
	"strings"
)

// App is one application that can open a file.
type App struct {
	Name     string // display name
	Path     string // macOS: the .app bundle; Linux: the .desktop file; Windows: the program
	Exec     string // Linux: the Exec template; Windows: the registry command template (with %1)
	Icon     string // icon name or path, when known
	Terminal bool   // Linux: runs in a terminal
	Default  bool   // the system opens this kind of file with this app by default

	rank int // 0 for apps made for this exact type, higher for apps that open anything of a broad kind
}

// AppsFor lists the applications that can open the file at path, the default one first, then by name.
func (l *Launcher) AppsFor(path string) ([]App, error) {
	apps, err := platformApps(l, path)
	if err != nil {
		return nil, err
	}
	sortApps(apps)
	return apps, nil
}

// AppsForMIME lists the applications for a MIME type ("text/plain"). It is only available where the system
// describes applications by MIME type (Linux and the BSDs); elsewhere it returns ErrUnsupported.
func (l *Launcher) AppsForMIME(mimeType string) ([]App, error) {
	apps, err := platformAppsForMIME(l, mimeType)
	if err != nil {
		return nil, err
	}
	sortApps(apps)
	return apps, nil
}

func sortApps(apps []App) {
	sort.SliceStable(apps, func(i, j int) bool {
		if apps[i].Default != apps[j].Default {
			return apps[i].Default
		}
		if apps[i].rank != apps[j].rank {
			return apps[i].rank < apps[j].rank
		}
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})
}
