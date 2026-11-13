//go:build unix && !darwin

// The "Open with" list on Linux and the BSDs: the file's MIME type is looked up among the installed .desktop files
// and the mimeapps.list choices (see xdgapps.go).
package launch

func platformApps(l *Launcher, path string) ([]App, error) {
	return xdgAppsForMIME(xdgEnv{getenv: l.getenv}, MIMEType(path)), nil
}

func platformAppsForMIME(l *Launcher, mimeType string) ([]App, error) {
	return xdgAppsForMIME(xdgEnv{getenv: l.getenv}, mimeType), nil
}
