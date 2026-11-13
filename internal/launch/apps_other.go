//go:build !unix && !windows

// Platforms without a known application registry have no "Open with" list.
package launch

func platformApps(l *Launcher, path string) ([]App, error) { return nil, ErrUnsupported }

func platformAppsForMIME(l *Launcher, mimeType string) ([]App, error) { return nil, ErrUnsupported }
