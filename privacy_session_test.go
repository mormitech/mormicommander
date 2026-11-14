// The zero-network test, run-time half. privacy_test.go reads the source and proves that nothing outside the FTP, SFTP
// and WebDAV clients and two loopback listeners can open a connection; this test watches the code actually run. It
// builds every feature the way the app does (boundObjects, with a settings folder of its own in the temporary folder,
// never the user's) and drives a scripted session through them: list a folder, view a text file and a Markdown file
// that points at images on the web, search by name and by text, copy files, and make thumbnails of pictures. All the
// while it watches for network use in three ways, and fails on any of them:
//
//   - the default HTTP transport is replaced by one that records every request (and refuses it);
//   - the default host-name resolver is replaced by one that records every lookup (and refuses it);
//   - on macOS and Linux the process's open file descriptors are sampled every couple of milliseconds, and any new
//     internet (IPv4 or IPv6) socket that reaches beyond the loopback address is reported with its address
//     (privacy_sockets_unix_test.go).
//
// Nothing here starts the app or opens a window: the features are plain Go objects, and the page side of the promise
// is checked by frontend/test/privacy.test.js and frontend/test/viewer-noremote.test.js. docs/privacy.md explains all
// of it for users.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mormicommander/internal/fileops"
)

// netWatch records every attempt to use the network while it is installed.
type netWatch struct {
	mu       sync.Mutex
	attempts []string
}

func (w *netWatch) add(format string, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.attempts = append(w.attempts, fmt.Sprintf(format, args...))
}

func (w *netWatch) list() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.attempts...)
}

// refusingTransport stands in for http.DefaultTransport: it writes the request down and fails it.
type refusingTransport struct{ w *netWatch }

func (r refusingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.w.add("HTTP %s %s", req.Method, req.URL.Redacted())
	return nil, errors.New("zero-network test: HTTP request refused")
}

// installNetWatch swaps in the recording transport and resolver, and starts the socket sampler where the system has
// one. The returned function stops it all, puts the originals back and returns what was seen.
func installNetWatch(t *testing.T) (*netWatch, func() []string) {
	t.Helper()
	w := &netWatch{}
	oldTransport, oldResolver := http.DefaultTransport, net.DefaultResolver
	http.DefaultTransport = refusingTransport{w}
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		w.add("DNS lookup through %s %s", network, address)
		return nil, errors.New("zero-network test: name lookup refused")
	}}
	stopSockets := watchSockets(w)
	return w, func() []string {
		stopSockets()
		http.DefaultTransport, net.DefaultResolver = oldTransport, oldResolver
		return w.list()
	}
}

// writePNG writes a small picture for the thumbnail step.
func writePNG(t *testing.T, p string, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// waitFor polls cond until it holds or the time is up.
func waitFor(t *testing.T, what string, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen within %v", what, d)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// sessionFeatures builds the features as the app does and picks out the ones the session drives.
type sessionFeatures struct {
	fs     *FS
	lister *Lister
	find   *Find
	jobs   *Jobs
	views  *Views
}

func buildSessionFeatures(t *testing.T) sessionFeatures {
	t.Helper()
	var s sessionFeatures
	for _, o := range boundObjects(NewApp(startOpts{})) {
		switch v := o.(type) {
		case *FS:
			s.fs = v
		case *Lister:
			s.lister = v
		case *Find:
			s.find = v
		case *Jobs:
			s.jobs = v
		case *Views:
			s.views = v
		}
	}
	if s.fs == nil || s.lister == nil || s.find == nil || s.jobs == nil || s.views == nil {
		t.Fatalf("a feature is missing from boundObjects: %+v", s)
	}
	return s
}

func TestZeroNetworkSession(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("MORMICOMMANDER_CONFIG_DIR", cfg) // never the user's settings or thumbnail cache
	work := t.TempDir()
	src := filepath.Join(work, "src")
	dst := filepath.Join(work, "dst")
	for _, d := range []string{src, dst, filepath.Join(src, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"notes.txt":     "hello world\nsecond line\n",
		"sub/deep.txt":  "hello from below\n",
		"page.html":     `<html><head><link rel="stylesheet" href="https://example.com/a.css"></head><body><img src="https://example.com/b.png">Hi</body></html>`,
		"readme.md":     "# Title\n\n![logo](https://example.com/logo.png)\n\nSee [the site](https://example.com/).\n",
		"sub/other.css": "@import url(https://example.com/x.css);\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(src, filepath.FromSlash(name)), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writePNG(t, filepath.Join(src, "red.png"), color.RGBA{200, 30, 30, 255})
	writePNG(t, filepath.Join(src, "blue.png"), color.RGBA{30, 30, 200, 255})

	_, stop := installNetWatch(t)
	stopped := false
	defer func() {
		if !stopped {
			stop()
		}
	}()

	f := buildSessionFeatures(t)

	// Browse: list the folder and a subfolder, ask about the volume and a file.
	l, err := f.fs.List("left", src)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(l.Entries) < len(files) {
		t.Errorf("list shows %d entries", len(l.Entries))
	}
	if _, err := f.fs.List("right", filepath.Join(src, "sub")); err != nil {
		t.Fatalf("list sub: %v", err)
	}
	f.fs.VolumeOf(src)
	if _, err := f.fs.Stat(filepath.Join(src, "notes.txt")); err != nil {
		t.Fatalf("stat: %v", err)
	}

	// View: a text file, the Markdown file with remote images and the HTML page with remote styles.
	for _, name := range []string{"notes.txt", "readme.md", "page.html"} {
		info, err := f.lister.Open(filepath.Join(src, name), ListerOptions{Mode: "text", Wrap: true, Width: 80})
		if err != nil {
			t.Fatalf("view %s: %v", name, err)
		}
		if _, err := f.lister.Move(info.ID, 0, "top", 0, 20); err != nil {
			t.Fatalf("view %s page: %v", name, err)
		}
		if _, err := f.lister.Text(info.ID, 0, -1, false); err != nil && !strings.Contains(err.Error(), "range") {
			t.Logf("view %s text: %v", name, err)
		}
		f.lister.Close(info.ID)
	}

	// Search: by name, then by text.
	for _, r := range []FindRequest{
		{SearchFor: "*.txt", SearchIn: src, Depth: -1},
		{SearchFor: "*", SearchIn: src, Depth: -1, TextOn: true, Text: "hello", Encodings: []string{"utf8"}, Kind: "files"},
	} {
		id, err := f.find.Start(r)
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		waitFor(t, "the search", 10*time.Second, func() bool { return !f.find.Running(id) })
		if len(f.find.Results(id)) == 0 {
			t.Errorf("find %q found nothing", r.SearchFor)
		}
		f.find.Forget(id)
	}

	// Copy: the whole folder to the other side.
	id, err := f.jobs.Start(fileops.Request{Kind: fileops.Copy, Sources: []string{filepath.Join(src, "notes.txt"), filepath.Join(src, "sub")}, Target: dst})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	waitFor(t, "the copy", 10*time.Second, func() bool { return !f.jobs.Running() })
	for _, p := range f.jobs.List() {
		if p.ID == id && p.State != fileops.StateDone {
			t.Errorf("copy ended %s", p.State)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "sub", "deep.txt")); err != nil {
		t.Errorf("copy did not arrive: %v", err)
	}

	// Thumbnails: the two pictures, waited for through the cache in the test's own settings folder.
	f.views.Thumbs("left", src, []string{"red.png", "blue.png", "notes.txt"}, 96)
	thumbDir := filepath.Join(cfg, "thumbs")
	waitFor(t, "the thumbnails", 20*time.Second, func() bool {
		n := 0
		filepath.WalkDir(thumbDir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				n++
			}
			return nil
		})
		return n >= 2
	})
	f.views.ThumbsStop("left")
	f.views.close()

	stopped = true
	var outbound []string
	for _, s := range stop() {
		if !strings.HasSuffix(s, "[loopback]") { // loopback never leaves the machine (see privacy_sockets_unix_test.go)
			outbound = append(outbound, s)
		}
	}
	if len(outbound) > 0 {
		t.Errorf("the session used the network:\n  %s", strings.Join(outbound, "\n  "))
	}
}

// The watch itself has to see what it is there to see: a request through the default client, a name lookup through
// the default resolver and, where sockets are sampled, a real connection (to a listener of the test's own on
// 127.0.0.1, so nothing leaves the machine).
func TestZeroNetworkWatchCatches(t *testing.T) {
	_, stop := installNetWatch(t)
	http.Get("http://example.invalid/ping")
	net.DefaultResolver.LookupHost(context.Background(), "example.invalid")
	if socketsWatched {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			stop()
			t.Fatal(err)
		}
		defer ln.Close()
		go func() {
			if c, err := ln.Accept(); err == nil {
				time.Sleep(100 * time.Millisecond)
				c.Close()
			}
		}()
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			stop()
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		c.Close()
	}
	seen := strings.Join(stop(), "\n")
	for _, want := range []string{"HTTP GET http://example.invalid/ping", "DNS lookup"} {
		if !strings.Contains(seen, want) {
			t.Errorf("the watch missed %q; saw:\n%s", want, seen)
		}
	}
	if socketsWatched && !strings.Contains(seen, "socket") {
		t.Errorf("the socket sampler missed a connection; saw:\n%s", seen)
	}
}
