package netbrowse

// Watching the network for servers, for the Network panel: the system's browse tool keeps running, and every server
// that appears or goes away is reported as it happens, so the panel fills gradually and stays current while the app
// runs. Discover (discover.go) is the one-shot form.

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Event is a server that appeared (Gone false) or went away.
type Event struct {
	Gone   bool
	Server Server
}

// Watch reports servers until ctx ends, with this system's tool; a tool that stops is started again after a pause.
// note says why nothing can be found (a missing tool), "" when the watch runs.
func Watch(ctx context.Context, emit func(Event)) (note string) {
	return watch(ctx, emit)
}

// keepRunning runs start again and again until ctx ends, pausing between runs.
func keepRunning(ctx context.Context, pause time.Duration, start func(context.Context)) {
	for ctx.Err() == nil {
		start(ctx)
		select {
		case <-ctx.Done():
		case <-time.After(pause):
		}
	}
}

// lines runs a tool and hands each line it prints to fn, until it ends or ctx ends.
func lines(ctx context.Context, fn func(string), name string, args ...string) {
	cmd := exec.CommandContext(ctx, name, args...)
	hide(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if cmd.Start() != nil {
		return
	}
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		fn(sc.Text())
	}
	io.Copy(io.Discard, out)
	cmd.Wait()
}

// watchDNSSD keeps `dns-sd -B` running for every service type and resolves each instance as it appears (macOS).
func watchDNSSD(ctx context.Context, emit func(Event)) string {
	if _, err := exec.LookPath("dns-sd"); err != nil {
		return "dns-sd is missing, so servers can't be found; type a server instead."
	}
	var wg sync.WaitGroup
	for typ, kind := range serviceKinds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			keepRunning(ctx, 10*time.Second, func(ctx context.Context) {
				lines(ctx, func(l string) {
					m := dnssdAdd.FindStringSubmatch(l)
					if m == nil {
						return
					}
					name := m[4]
					if m[1] == "Rmv" {
						emit(Event{Gone: true, Server: Server{Name: name, Kind: kind, Source: "bonjour"}})
						return
					}
					go func() {
						res, _ := runFor(ctx, 3*time.Second, "dns-sd", "-L", name, typ, "local.")
						if host, port := ParseDNSSDLookup(res); host != "" {
							emit(Event{Server: Server{Name: name, Host: host, Port: nonDefaultPort(kind, port), Kind: kind, Source: "bonjour"}})
						}
					}()
				}, "dns-sd", "-B", typ, "local.")
			})
		}()
	}
	wg.Wait()
	return ""
}

// watchAvahi keeps `avahi-browse -pr` running for every service type (Linux): "=" lines are resolved servers, "-"
// lines servers that went away.
func watchAvahi(ctx context.Context, emit func(Event)) string {
	if _, err := exec.LookPath("avahi-browse"); err != nil {
		return "avahi-browse (package avahi-utils) is missing, so servers can't be found; type a server instead."
	}
	var wg sync.WaitGroup
	for typ, kind := range serviceKinds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			keepRunning(ctx, 10*time.Second, func(ctx context.Context) {
				lines(ctx, func(l string) {
					if strings.HasPrefix(l, "-;") {
						if f := strings.Split(l, ";"); len(f) >= 5 {
							emit(Event{Gone: true, Server: Server{Name: avahiUnescape(f[3]), Kind: kind, Source: "avahi"}})
						}
						return
					}
					for _, s := range ParseAvahi(l) {
						emit(Event{Server: s})
					}
				}, "avahi-browse", "-pr", typ)
			})
		}()
	}
	wg.Wait()
	return ""
}

// watchNetView asks `net view` again every minute (Windows) and reports the servers that came and went.
func watchNetView(ctx context.Context, emit func(Event)) string {
	have := map[string]Server{}
	keepRunning(ctx, time.Minute, func(ctx context.Context) {
		text, _ := runFor(ctx, 20*time.Second, "net", "view")
		now := map[string]Server{}
		for _, s := range ParseNetView(text) {
			now[strings.ToLower(s.Host)] = s
			if _, ok := have[strings.ToLower(s.Host)]; !ok {
				emit(Event{Server: s})
			}
		}
		for k, s := range have {
			if _, ok := now[k]; !ok {
				emit(Event{Gone: true, Server: s})
			}
		}
		have = now
	})
	return ""
}
