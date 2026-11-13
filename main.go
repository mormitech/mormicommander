// mormiCommander is a free two-panel file manager for macOS, Windows and Linux, in the spirit of Total Commander.
//
// The Go side lists folders, knows the volumes, runs file jobs and builds the native menu; the page (frontend/) draws
// the panels and owns the keyboard. Features plug in through features.go (Go) and frontend/src/features/ (page), so this
// file does not change when a feature is added. See docs/ARCHITECTURE.md.
package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
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
	title, width, height := "mormiCommander", windowW, windowH
	if o.About {
		title, width, height = "About mormiCommander", 440, 560
	} else if st, ok := loadWinState(); ok {
		width, height = st.W, st.H
		app.saved = &st
	}
	app.winW, app.winH = width, height
	if !o.About { // the About window has no menu of its own
		app.menuBar = baseMenu(app, nil)
	}

	err := wails.Run(&options.App{
		Menu:          app.menuBar,
		Title:         title,
		Width:         width,
		Height:        height,
		MinWidth:      minWinW,
		MinHeight:     minWinH,
		DisableResize: o.About,
		AssetServer:   &assetserver.Options{Assets: assets},
		// Matches --bg of the dark theme (style.css), so the window doesn't flash white while the page loads.
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		// Without Mac options Wails leaves the green zoom (maximise) button disabled.
		Mac:     &mac.Options{Appearance: macAppearance(o.Theme)},
		Windows: &windows.Options{Theme: windowsTheme(o.Theme)},
		// ProgramName is the window class, so the taskbar and the desktop files group the window under mormicommander.
		Linux:      &linux.Options{Icon: appIcon, ProgramName: "mormicommander"},
		OnStartup:  app.startup,
		OnDomReady: func(context.Context) { app.reveal() },
		OnShutdown: app.shutdown,
		OnBeforeClose: func(context.Context) bool {
			app.SaveWindow()
			return false
		},
		Bind: boundObjects(app),
	})
	if err != nil {
		println("Error:", err.Error())
	}
}

// The window follows the system's light or dark appearance, unless --theme says otherwise (used for screenshots).
func macAppearance(theme string) mac.AppearanceType {
	switch theme {
	case "dark":
		return mac.NSAppearanceNameDarkAqua
	case "light":
		return mac.NSAppearanceNameAqua
	}
	return mac.DefaultAppearance
}

func windowsTheme(theme string) windows.Theme {
	switch theme {
	case "dark":
		return windows.Dark
	case "light":
		return windows.Light
	}
	return windows.SystemDefault
}
