package main

// Tests of issue #78 (FTP and network file systems): properties, permissions and symbolic links on an SFTP server
// (F-649, F-633) on the in-process SSH server of feature_net_sftp_test.go, HTTP downloads (F-644) from an httptest
// server on 127.0.0.1, the proxy password and the SFTP proxy (F-637), and the idle and connection defaults (F-650,
// F-643). Nothing here reaches a real server.

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/sftp"
)

func netSFTPReady(t *testing.T) (*Net, *netSSHServer) {
	t.Helper()
	netSSHIsolate(t)
	s := newNetSSHServer(t, false)
	answers := map[string]string{"hostkey": "once", "password": netSSHPass}
	n, _ := netSSHNet(t, func(kind, _ string) (string, bool) { v, ok := answers[kind]; return v, ok })
	netListHas(t, n, s.URL, "")
	return n, s
}

func TestNetRemoteInfoChmodSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions on the served folder")
	}
	n, s := netSFTPReady(t)
	os.WriteFile(filepath.Join(s.Root, "a.txt"), []byte("hello"), 0o644)
	os.MkdirAll(filepath.Join(s.Root, "dir", "sub"), 0o755)
	os.WriteFile(filepath.Join(s.Root, "dir", "b.sh"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(s.Root, "dir", "sub", "c.txt"), []byte("x"), 0o644)

	info, err := n.RemoteInfo(s.URL + "/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "a.txt" || info.Size != 5 || info.Kind != "sftp" || info.Octal != "0644" || info.Perms != "-rw-r--r--" ||
		!info.CanChmod || !info.CanSymlink || info.IsDir || !strings.HasPrefix(info.Server, netSSHUser+"@127.0.0.1:") {
		t.Fatalf("info %+v", info)
	}
	if _, err := n.RemoteInfo("/tmp/x"); err == nil {
		t.Fatal("a local path got remote info")
	}

	// one file
	res, err := n.RemoteChmod(NetChmodRequest{Paths: []string{s.URL + "/a.txt"}, Octal: "600"})
	if err != nil || res.Done != 1 || len(res.Failed) != 0 {
		t.Fatalf("chmod: %+v %v", res, err)
	}
	if fi, _ := os.Stat(filepath.Join(s.Root, "a.txt")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	// a folder tree, files only, plus the date
	when := time.Date(2020, 5, 6, 7, 8, 9, 0, time.UTC)
	res, err = n.RemoteChmod(NetChmodRequest{Paths: []string{s.URL + "/dir"}, Octal: "0700", Recursive: true, ApplyTo: "files", MTime: when.UnixMilli()})
	if err != nil || res.Done != 2 || len(res.Failed) != 0 {
		t.Fatalf("recursive chmod: %+v %v", res, err)
	}
	for _, f := range []string{"dir/b.sh", "dir/sub/c.txt"} {
		fi, _ := os.Stat(filepath.Join(s.Root, f))
		if fi.Mode().Perm() != 0o700 || !fi.ModTime().Equal(when) {
			t.Fatalf("%s: %v %v", f, fi.Mode(), fi.ModTime())
		}
	}
	if fi, _ := os.Stat(filepath.Join(s.Root, "dir", "sub")); fi.Mode().Perm() != 0o755 {
		t.Fatalf("a folder was changed with 'files only': %v", fi.Mode())
	}
	if _, err := n.RemoteChmod(NetChmodRequest{Paths: []string{s.URL + "/a.txt"}, Octal: "9z"}); err == nil {
		t.Fatal("a bad permission number was taken")
	}
	if _, err := n.RemoteChmod(NetChmodRequest{Paths: []string{s.URL + "/a.txt"}}); err == nil {
		t.Fatal("nothing to change was accepted")
	}

	// a symbolic link; an existing name is never replaced
	p, err := n.RemoteSymlink(s.URL, "link.txt", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	// (the test server stores a relative target as an absolute path, as some real servers do)
	if target, err := os.Readlink(filepath.Join(s.Root, "link.txt")); err != nil || filepath.Base(target) != "a.txt" {
		t.Fatalf("link: %q %v", target, err)
	}
	info, err = n.RemoteInfo(p)
	if err != nil || !info.IsLink || !strings.HasSuffix(info.LinkTarget, "a.txt") {
		t.Fatalf("link info %+v %v", info, err)
	}
	if _, err := n.RemoteSymlink(s.URL, "a.txt", "dir"); err == nil || !strings.Contains(err.Error(), "already there") {
		t.Fatalf("an existing name was replaced: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(s.Root, "a.txt")); string(b) != "hello" {
		t.Fatal("the file in the way changed")
	}
	if _, err := n.RemoteSymlink(s.URL, "../out", "a.txt"); err == nil {
		t.Fatal("a name leaving the folder was taken")
	}
	if _, err := n.RemoteSymlink("ftp://h/x", "l", "a"); err == nil {
		t.Fatal("a link on FTP was attempted")
	}
}

func TestNetOctalHelpers(t *testing.T) {
	for _, c := range []struct {
		in   string
		bits uint32
		ok   bool
	}{{"644", 0o644, true}, {"0755", 0o755, true}, {"4755", 0o4755, true}, {"", 0, false}, {"888", 0, false}, {"12345", 0, false}} {
		b, err := netParseOctal(c.in)
		if (err == nil) != c.ok || b != c.bits {
			t.Errorf("%q: %o %v", c.in, b, err)
		}
	}
	for _, b := range []uint32{0o644, 0o4755, 0o2750, 0o1777} {
		if got := netUnixBits(netModeOf(b)); got != b {
			t.Errorf("%o round trip gave %o", b, got)
		}
	}
	if netModeOf(0o4000)&fs.ModeSetuid == 0 {
		t.Error("set-user-ID lost")
	}
}

func TestNetHTTPDownload(t *testing.T) {
	body := strings.Repeat("0123456789", 1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files/data.bin":
			w.Header().Set("Last-Modified", "Wed, 01 Jan 2020 10:00:00 GMT")
			http.ServeContent(w, r, "data.bin", time.Date(2020, 1, 1, 10, 0, 0, 0, time.UTC), strings.NewReader(body))
		case "/":
			fmt.Fprint(w, "<html>home</html>")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	n, _ := testNet(t, nil)
	dir := t.TempDir()

	id, err := n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{srv.URL + "/files/data.bin"}, Target: dir})
	if err != nil {
		t.Fatal(err)
	}
	p := waitJob(t, n, id, nil)
	got, _ := os.ReadFile(filepath.Join(dir, "data.bin"))
	if p.State != "done" || string(got) != body || p.BytesDone != int64(len(body)) || p.FilesDone != 1 {
		t.Fatalf("download: %+v (%d bytes)", p, len(got))
	}
	if fi, _ := os.Stat(filepath.Join(dir, "data.bin")); !fi.ModTime().Equal(time.Date(2020, 1, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("date %v", fi.ModTime())
	}

	// a shorter file in the way: the question offers Resume, which fetches only the rest
	os.WriteFile(filepath.Join(dir, "data.bin"), []byte(body[:3000]), 0o644)
	asked := false
	id, _ = n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{srv.URL + "/files/data.bin"}, Target: dir})
	p = waitJob(t, n, id, func(p NetJobProgress) *NetJobAnswer {
		asked = p.Ask != nil && p.Ask.CanResume
		return &NetJobAnswer{Choice: "resume"}
	})
	got, _ = os.ReadFile(filepath.Join(dir, "data.bin"))
	if !asked || p.State != "done" || string(got) != body {
		t.Fatalf("resume: asked %v %+v (%d bytes)", asked, p, len(got))
	}

	// skip keeps the file; the root of a site is saved as index.html; a missing file fails with the server's answer
	os.WriteFile(filepath.Join(dir, "data.bin"), []byte("mine"), 0o644)
	id, _ = n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{srv.URL + "/files/data.bin"}, Target: dir, Overwrite: "skip"})
	if p = waitJob(t, n, id, nil); p.State != "done" {
		t.Fatalf("skip: %+v", p)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "data.bin")); string(b) != "mine" {
		t.Fatal("skip replaced the file")
	}
	id, _ = n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{srv.URL + "/"}, Target: dir})
	if p = waitJob(t, n, id, nil); p.State != "done" {
		t.Fatalf("index: %+v", p)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "index.html")); string(b) != "<html>home</html>" {
		t.Fatalf("index.html %q", b)
	}
	id, _ = n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{srv.URL + "/nope.txt"}, Target: dir})
	if p = waitJob(t, n, id, nil); p.State != "failed" || !strings.Contains(strings.Join(p.Errors, " "), "404") {
		t.Fatalf("missing: %+v", p)
	}
	if _, err := os.Stat(filepath.Join(dir, "nope.txt")); err == nil {
		t.Fatal("a failed download left a file")
	}
	// a move and a server target are refused
	id, _ = n.StartJob(NetJobRequest{Kind: "move", Sources: []string{srv.URL + "/files/data.bin"}, Target: dir})
	if p = waitJob(t, n, id, nil); p.State != "failed" {
		t.Fatalf("move: %+v", p)
	}
	id, _ = n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{srv.URL + "/files/data.bin"}, Target: "ftp://h/pub"})
	if p = waitJob(t, n, id, nil); p.State != "failed" {
		t.Fatalf("server target: %+v", p)
	}
}

func TestNetHTTPName(t *testing.T) {
	for in, want := range map[string]string{
		"https://example.com/a/b/file.zip?x=1": "file.zip",
		"http://example.com":                   "index.html",
		"http://example.com/dir/":              "dir",
		"http://example.com/%E2%82%AC.txt":     "€.txt",
	} {
		if got, err := netHTTPName(in); err != nil || got != want {
			t.Errorf("%s: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"http://", "http://example.com/..", "http://example.com/a%2F..%2Fb"} {
		if got, err := netHTTPName(bad); err == nil && strings.ContainsAny(got, `/\`) {
			t.Errorf("%s gave %q", bad, got)
		}
	}
	if !netIsHTTP(" HTTPS://x/y") || netIsHTTP("ftp://x") || netIsHTTP("/http/x") {
		t.Error("netIsHTTP")
	}
}

func TestNetProxyPasswordAndSFTPProxy(t *testing.T) {
	n, asked := testNet(t, map[string]string{"password": "pp"})
	if pw, err := n.proxyPassword("", "proxy:1080"); err != nil || pw != "" || len(*asked) != 0 {
		t.Fatalf("no user: %q %v %v", pw, err, *asked)
	}
	for i := 0; i < 2; i++ {
		if pw, err := n.proxyPassword("bob", "Proxy:1080"); err != nil || pw != "pp" {
			t.Fatalf("user: %q %v", pw, err)
		}
	}
	if len(*asked) != 1 {
		t.Fatalf("asked %v, want once per session", *asked)
	}
	n2, _ := testNet(t, nil)
	if _, err := n2.proxyPassword("bob", "proxy:1080"); err != errNetCancelled {
		t.Fatalf("cancelled: %v", err)
	}

	if p := netSFTPProxy(3, "socks.local", 0, "u"); p == nil || p.Type != sftp.ProxySOCKS5 || p.Addr != "socks.local:1080" || p.User != "u" {
		t.Fatalf("socks5: %+v", p)
	}
	if p := netSFTPProxy(1, "web.local", 3128, ""); p == nil || p.Type != sftp.ProxyHTTP || p.Addr != "web.local:3128" {
		t.Fatalf("http: %+v", p)
	}
	if netSFTPProxy(4, "ftpproxy", 21, "") != nil || netSFTPProxy(0, "x", 1, "") != nil || netSFTPProxy(2, " ", 1, "") != nil {
		t.Fatal("USER@host, none or no host gave an SFTP proxy")
	}
}

func TestNetIdleAndConnectionDefaults(t *testing.T) {
	if m := netIdleMinutes(netOptions{IdleMinutes: 3}); m != 3 {
		t.Fatalf("own option: %d", m)
	}
	if m := netIdleMinutes(netOptions{}); m < 1 {
		t.Fatalf("default idle %d", m)
	}
	if c := netMaxConns(); c < 1 || c > 8 {
		t.Fatalf("connections %d", c)
	}
}
