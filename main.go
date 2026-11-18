// mormiCommander is a free two-panel file manager for macOS, Windows and Linux, in the spirit of Total Commander.
//
// The Go side lists folders, knows the volumes, runs file jobs and builds the native menu; the page (frontend/) draws
// the panels and owns the keyboard. Features plug in through features.go (Go) and frontend/src/features/ (page), so this
// file does not change when a feature is added. See docs/ARCHITECTURE.md.
//
// The shell is Wails v3 (github.com/wailsapp/wails/v3, pinned to v3.1.0): one process, one application object, and as
// many windows as needed. The file manager is the window named "main"; the About box is a second window of the same
// process (window.go), where Wails v2 had to start a second copy of the program for it.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

// On Linux the window carries its icon itself (macOS and Windows take it from the app bundle and the .exe).
//
//go:embed build/appicon.png
var appIcon []byte

// The main window's preferred size; startup shrinks it on screens that are too small.
const windowW, windowH = 1280, 800

func main() {
	startThroughRealPath() // (macOS, started through a link: see reexec.go)
	o := parseArgs(os.Args[1:])
	if o.Control == "" && !o.About {
		o.Control = os.Getenv("MORMICOMMANDER_CONTROL") // control mode from the environment (0 = a free port)
	}
	if o.Version {
		fmt.Println("mormiCommander", version())
		return
	}
	if o.Help {
		fmt.Printf(usage, version())
		return
	}
	if o.Demo {
		startDemo()
	}
	app := NewApp(o)
	app.standalone = standaloneWanted(o)
	width, height := windowW, windowH
	if st, ok := loadWinState(); ok && !o.About {
		width, height = st.W, st.H
		app.saved = &st
	}
	app.winW, app.winH = width, height

	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		println("Error:", err.Error())
		os.Exit(1)
	}
	wapp := application.New(application.Options{
		Name:        "mormiCommander",
		Description: "Two-panel file manager.",
		Icon:        appIcon,
		Services:    services(app),
		Assets: application.AssetOptions{
			Handler:        application.BundledAssetFileServer(dist),
			DisableLogging: true,
		},
		LogLevel: slog.LevelWarn,
		// Closing the file manager's window ends the app (window.go quits on it too, so an open About window does not
		// keep the process alive).
		Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		// ProgramName is the window class, so the taskbar and the desktop files group the window under mormicommander.
		Linux:      application.LinuxOptions{ProgramName: "mormicommander"},
		OnShutdown: app.shutdown,
	})
	app.wapp = wapp
	app.ctx = wapp.Context()
	if o.About {
		// `mormicommander --about` shows just the About box (kept for scripts that used the v2 way of opening it).
		app.openAbout()
	} else {
		app.menuBar = baseMenu(app, nil)
		wapp.Menu.Set(app.menuBar)
		app.newMainWindow(width, height)
	}
	if err := wapp.Run(); err != nil {
		println("Error:", err.Error())
	}
}

// services is every object the page can call (the App and the objects of each feature, see features.go), each a v3
// service. The page reaches them as main.<Type>.<Method> through frontend/src/core/go.js.
func services(a *App) []application.Service {
	return boundServices(a)
}

// The window follows the system's light or dark appearance, unless --theme says otherwise (used for screenshots).
func macAppearance(theme string) application.MacAppearanceType {
	switch theme {
	case "dark":
		return application.NSAppearanceNameDarkAqua
	case "light":
		return application.NSAppearanceNameAqua
	}
	return application.DefaultAppearance
}

func windowsTheme(theme string) application.Theme {
	switch theme {
	case "dark":
		return application.Dark
	case "light":
		return application.Light
	}
	return application.SystemDefault
}
