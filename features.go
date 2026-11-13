package main

import (
	"fmt"
	"sort"
	"sync"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Features plug themselves in here, so that main.go and app.go never need editing for a new feature (see
// docs/ARCHITECTURE.md). A feature is one or more files feature_<name>.go with an init function that calls:
//
//	registerFeature("hotlist", func(a *App) []any { return []any{&Hotlist{app: a}} })   // objects the page can call
//	onStartup(func(a *App) { ... })                                                  // after the window exists
//	onShutdown(func(a *App) { ... })                                                 // when the app ends
//	registerControl("hotlist", func(a *App, m map[string]any) (map[string]any, error) { ... }) // a control-mode command
//
// The page calls a bound object as window.go.main.<Type>.<Method>(...) (see frontend/src/core/go.js). Type names must be
// unique, and exported methods need a one-line doc comment in plain words. A method whose first parameter is a
// context.Context gets the call's context from Wails (with the window the call came from, see App.Mode); the page does
// not pass it.

type featureEntry struct {
	name string
	bind func(a *App) []any
}

var (
	featMu          sync.Mutex
	featureList     []featureEntry
	startupHooks    []func(a *App)
	shutdownHooks   []func(a *App)
	controlCommands = map[string]func(a *App, m map[string]any) (map[string]any, error){}
)

// registerFeature adds the objects of a feature to the ones the page can call.
func registerFeature(name string, bind func(a *App) []any) {
	featMu.Lock()
	defer featMu.Unlock()
	featureList = append(featureList, featureEntry{name, bind})
}

// onStartup runs f once the window and the app's context exist.
func onStartup(f func(a *App)) {
	featMu.Lock()
	defer featMu.Unlock()
	startupHooks = append(startupHooks, f)
}

// onShutdown runs f when the app ends.
func onShutdown(f func(a *App)) {
	featMu.Lock()
	defer featMu.Unlock()
	shutdownHooks = append(shutdownHooks, f)
}

// registerControl adds a control-mode command handled in Go (commands not found here go to the page).
func registerControl(name string, f func(a *App, m map[string]any) (map[string]any, error)) {
	featMu.Lock()
	defer featMu.Unlock()
	controlCommands[name] = f
}

// boundObjects is everything the page can call: the App itself and the objects of every feature, in name order so the
// generated bindings don't change from one build to the next.
func boundObjects(a *App) []any {
	featMu.Lock()
	list := append([]featureEntry(nil), featureList...)
	featMu.Unlock()
	sort.SliceStable(list, func(i, j int) bool { return list[i].name < list[j].name })
	out := []any{a}
	for _, f := range list {
		out = append(out, f.bind(a)...)
	}
	return out
}

// featureNames lists the registered Go features (for the control mode's "info").
func featureNames() []string {
	featMu.Lock()
	defer featMu.Unlock()
	out := make([]string, 0, len(featureList))
	for _, f := range featureList {
		out = append(out, f.name)
	}
	sort.Strings(out)
	return out
}

// serviceLayout is application.Service as Wails v3.1.0 lays it out. application.NewService is generic (it wants the
// concrete pointer type at compile time), while features hand their objects over as []any; rather than make twenty
// feature files spell out a service each, asService builds the same value from the object. servicesOK checks the layout
// against a service made the official way, so a Wails upgrade that changes it fails loudly at start (and in the tests)
// instead of binding nothing.
type serviceLayout struct {
	instance any
	options  application.ServiceOptions
}

func asService(obj any) application.Service {
	l := serviceLayout{instance: obj}
	return *(*application.Service)(unsafe.Pointer(&l))
}

type serviceProbe struct{}

// servicesOK says whether asService makes the same thing as application.NewService.
func servicesOK() error {
	p := &serviceProbe{}
	official := application.NewService(p)
	if unsafe.Sizeof(official) != unsafe.Sizeof(serviceLayout{}) || official.Instance() != any(p) || asService(p).Instance() != any(p) {
		return fmt.Errorf("application.Service has changed in this Wails version; update asService in features.go")
	}
	return nil
}
