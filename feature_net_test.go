package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/webdav"

	"mormicommander/internal/ftp"
	"mormicommander/internal/vfs"
)

func TestNetParseURL(t *testing.T) {
	cases := []struct{ in, want, key string }{
		{"example.com", "ftp://example.com/", "ftp://example.com"},
		{"ftp://Peter:pw@Example.COM:21/pub/../x", "ftp://Peter@example.com/x", "ftp://Peter@example.com"},
		{"ftps://h:2121", "ftps://h:2121/", "ftps://h:2121"},
		{"scp://bob@h:22/~/docs", "sftp://bob@h/~/docs", "sftp://bob@h"},
		{"davs://u@cloud.example.org/remote.php/dav", "webdavs://u@cloud.example.org/remote.php/dav", "webdavs://u@cloud.example.org"},
		{"ftp://a%40b@h/", "ftp://a%40b@h/", "ftp://a%40b@h"},
	}
	for _, c := range cases {
		u, err := netParseURL(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if u.String() != c.want || u.key() != c.key {
			t.Errorf("%s: got %s / %s, want %s / %s", c.in, u.String(), u.key(), c.want, c.key)
		}
	}
	if _, err := netParseURL("gopher://x"); err == nil {
		t.Error("gopher:// should be refused")
	}
	if _, err := netParseURL("ftp://h:99999"); err == nil {
		t.Error("a bad port should be refused")
	}
	// the canonical form is the one the vfs router shows
	r := vfs.NewRouter()
	defer r.Close()
	for _, in := range []string{"ftp://Peter@Example.COM:21/pub/../x", "ftpes://u@h:2121/a b/c"} {
		u, _ := netParseURL(in)
		if got := r.Clean(in); got != u.String() {
			t.Errorf("router %s, ours %s", got, u.String())
		}
	}
	u, _ := netParseURL("ftp://u:secret@h/x")
	if strings.Contains(u.String(), "secret") || !strings.Contains(u.withPassword(), "secret") {
		t.Errorf("password handling: %s / %s", u.String(), u.withPassword())
	}
}

func TestNetFTPKeyMatchesURL(t *testing.T) {
	for _, in := range []string{"ftp://peter@host/x", "ftps://h:991/", "ftpes://u@h/"} {
		u, _ := netParseURL(in)
		cfg, _, err := ftp.ParseURL(u.key())
		if err != nil {
			t.Fatal(err)
		}
		if got := ftpKey(&cfg); got != u.key() {
			t.Errorf("%s: ftpKey %s, want %s", in, got, u.key())
		}
	}
}

// testNet makes a Net with its own settings folder and an asker that answers from the map (kind → value).
func testNet(t *testing.T, answers map[string]string) (*Net, *[]string) {
	t.Helper()
	t.Setenv("MORMICOMMANDER_CONFIG_DIR", t.TempDir())
	n := newNet(&App{})
	var asked []string
	n.asker = func(kind string, data map[string]any) (map[string]any, bool) {
		asked = append(asked, kind)
		v, ok := answers[kind]
		if !ok {
			return nil, false
		}
		return map[string]any{"ok": true, "value": v, "choice": v}, true
	}
	t.Cleanup(n.close)
	return n, &asked
}

func TestNetConfigureFTP(t *testing.T) {
	n, asked := testNet(t, map[string]string{"password": "typed"})
	site := ftp.Site{ID: "s1", Name: "Work", Protocol: ftp.ProtoFTP, Host: "h", User: "peter", Active: true, Transfer: "ascii",
		Encoding: "latin1", ListCommand: "LIST -la", RemoteDir: "/pub", TryTLS: true}
	n.sites["ftp://peter@h"] = site
	cfg := ftp.Config{Host: "h", User: "peter"}
	if err := n.configureFTP(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Password != "typed" || cfg.DataMode != ftp.Active || cfg.TransferMode != ftp.TransferASCII || cfg.Encoding != ftp.EncodingLatin1 ||
		cfg.ListCommand != "LIST -la" || cfg.InitialDir != "/pub" || cfg.Security != ftp.SecurityExplicitTLSIfAvailable || cfg.Logf == nil {
		t.Fatalf("site settings not applied: %+v", cfg)
	}
	// the password is remembered: a reconnect does not ask again
	cfg2 := ftp.Config{Host: "h", User: "peter"}
	if err := n.configureFTP(&cfg2); err != nil || cfg2.Password != "typed" || len(*asked) != 1 {
		t.Fatalf("second dial: %v %q asked %v", err, cfg2.Password, *asked)
	}
	// the transfer mode chosen with Ctrl+Shift+M wins
	if m, err := n.CycleTransferMode("ftp://peter@h/x"); err != nil || m != "auto" {
		t.Fatalf("cycle from ascii: %q %v", m, err)
	}
	cfg3 := ftp.Config{Host: "h", User: "peter"}
	n.configureFTP(&cfg3)
	if cfg3.TransferMode != ftp.TransferAuto {
		t.Fatalf("mode %v", cfg3.TransferMode)
	}
	// anonymous logins ask nothing; a cancelled password stops the connection
	anon := ftp.Config{Host: "other"}
	if err := n.configureFTP(&anon); err != nil {
		t.Fatal(err)
	}
	n2, _ := testNet(t, nil)
	if err := n2.configureFTP(&ftp.Config{Host: "h", User: "x"}); err != errNetCancelled {
		t.Fatalf("cancelled password: %v", err)
	}
}

func TestNetSitesStoreAndMasterPassword(t *testing.T) {
	n, asked := testNet(t, map[string]string{"master-new": "m4ster", "master": "m4ster"})
	id, err := n.SaveSite(NetSite{Name: "Box", Folder: "Work/Clients", Protocol: "sftp", Host: "sftp://bob@box.example:2222/home/bob",
		Password: "pw1", SavePassword: true, ShowHidden: true, Passive: true})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(n.storePath)
	if bytes.Contains(b, []byte("pw1")) {
		t.Fatal("the password is in the file in plain text")
	}
	l, err := n.Sites()
	if err != nil || len(l.Sites) != 1 || !l.HasMaster {
		t.Fatalf("%+v %v", l, err)
	}
	s := l.Sites[0]
	if s.Host != "box.example" || s.Port != 2222 || s.User != "bob" || s.RemoteDir != "/home/bob" || !s.HasPassword || !s.ShowHidden || s.Folder != "Work/Clients" {
		t.Fatalf("site %+v", s)
	}
	if _, err := n.CopySite(id); err != nil {
		t.Fatal(err)
	}
	if err := n.RenameFolder("Work", "Jobs"); err != nil {
		t.Fatal(err)
	}
	l, _ = n.Sites()
	if len(l.Sites) != 2 || l.Sites[0].Folder != "Jobs/Clients" {
		t.Fatalf("after copy and rename: %+v", l)
	}
	// a fresh run asks for the master password once, then reads the saved password
	n2 := newNet(&App{})
	n2.storePath = n.storePath
	n2.asker = n.asker
	box, err := n2.unlockIfNeeded()
	if err != nil {
		t.Fatal(err)
	}
	st, _ := n2.loadStore()
	if pw, err := st.Password(id, box); err != nil || pw != "pw1" {
		t.Fatalf("password %q %v", pw, err)
	}
	// unticking "save password" removes it
	s.SavePassword, s.Password, s.Folder = false, "", "Jobs/Clients"
	s.ID = id
	if _, err := n.SaveSite(s); err != nil {
		t.Fatal(err)
	}
	l, _ = n.Sites()
	for _, x := range l.Sites {
		if x.ID == id && x.HasPassword {
			t.Fatal("password still saved")
		}
	}
	if err := n.DeleteFolder("Jobs"); err != nil {
		t.Fatal(err)
	}
	if l, _ = n.Sites(); len(l.Sites) != 0 {
		t.Fatalf("folder delete left %+v", l.Sites)
	}
	if strings.Count(strings.Join(*asked, ","), "master-new") != 1 {
		t.Fatalf("asked %v", *asked)
	}
}

// davServer starts a WebDAV server on 127.0.0.1 over a temp folder; with a password it wants basic auth.
func davServer(t *testing.T, user, pass string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	h := &webdav.Handler{FileSystem: webdav.Dir(dir), LockSystem: webdav.NewMemLS()}
	var handler http.Handler = h
	if pass != "" {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok || u != user || p != pass {
				w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
			h.ServeHTTP(w, r)
		})
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://"), dir
}

func waitJob(t *testing.T, n *Net, id int, answer func(p NetJobProgress) *NetJobAnswer) NetJobProgress {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		p, ok := n.JobState(id)
		if !ok {
			t.Fatal("unknown transfer")
		}
		if p.State == "done" || p.State == "failed" || p.State == "cancelled" {
			return p
		}
		if p.State == "asking" && answer != nil {
			if a := answer(p); a != nil {
				n.AnswerJob(id, *a)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the transfer did not end")
	return NetJobProgress{}
}

func TestNetWebDAVPanelAndTransfers(t *testing.T) {
	host, dir := davServer(t, "bob", "s3cret")
	n, asked := testNet(t, map[string]string{"password": "s3cret"})
	os.MkdirAll(filepath.Join(dir, "pub", "sub"), 0o755)
	big := bytes.Repeat([]byte("0123456789abcdef"), 64*1024) // 1 MiB
	os.WriteFile(filepath.Join(dir, "pub", "big.bin"), big, 0o644)
	os.WriteFile(filepath.Join(dir, "pub", "sub", "a.txt"), []byte("hello"), 0o644)

	res, err := n.Connect("webdav://bob@" + host + "/pub")
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "webdav://bob@"+host+"/pub" || len(*asked) != 1 || (*asked)[0] != "password" {
		t.Fatalf("connect: %+v asked %v", res, *asked)
	}
	fsys := vfs.For(res.Path, nil)
	l, err := fsys.List(context.Background(), res.Path)
	if err != nil || len(l.Entries) != 2 {
		t.Fatalf("list: %+v %v", l, err)
	}
	if st := n.Status(res.Path); !st.Connected || st.Kind != "webdav" {
		t.Fatalf("status %+v", st)
	}

	// download with resume: half the file is already there
	local := t.TempDir()
	os.WriteFile(filepath.Join(local, "big.bin"), big[:300000], 0o644)
	id, err := n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{res.Path + "/big.bin", res.Path + "/sub"}, Target: local})
	if err != nil {
		t.Fatal(err)
	}
	var sawResume bool
	p := waitJob(t, n, id, func(p NetJobProgress) *NetJobAnswer {
		sawResume = p.Ask.CanResume
		return &NetJobAnswer{Choice: "resume"}
	})
	if p.State != "done" || !sawResume {
		t.Fatalf("download: %+v resume offered %v", p, sawResume)
	}
	if got, _ := os.ReadFile(filepath.Join(local, "big.bin")); !bytes.Equal(got, big) {
		t.Fatalf("resumed download has %d bytes", len(got))
	}
	if got, _ := os.ReadFile(filepath.Join(local, "sub", "a.txt")); string(got) != "hello" {
		t.Fatal("folder not downloaded with its structure")
	}

	// upload a folder, then move it on the server (server-side rename)
	os.MkdirAll(filepath.Join(local, "up", "deep"), 0o755)
	os.WriteFile(filepath.Join(local, "up", "deep", "b.txt"), []byte("bee"), 0o644)
	id, _ = n.StartJob(NetJobRequest{Kind: "copy", Sources: []string{filepath.Join(local, "up")}, Target: res.Path})
	if p := waitJob(t, n, id, nil); p.State != "done" {
		t.Fatalf("upload: %+v", p)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "pub", "up", "deep", "b.txt")); string(got) != "bee" {
		t.Fatal("upload missing")
	}
	id, _ = n.StartJob(NetJobRequest{Kind: "move", Sources: []string{res.Path + "/up"}, Target: res.Path + "/sub"})
	if p := waitJob(t, n, id, nil); p.State != "done" {
		t.Fatalf("move: %+v", p)
	}
	if _, err := os.Stat(filepath.Join(dir, "pub", "sub", "up", "deep", "b.txt")); err != nil {
		t.Fatal("moved folder missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "pub", "up")); err == nil {
		t.Fatal("source still there after move")
	}

	// a move from the server to the disk deletes the server copy
	id, _ = n.StartJob(NetJobRequest{Kind: "move", Sources: []string{res.Path + "/sub/a.txt"}, Target: local, Overwrite: "overwrite"})
	if p := waitJob(t, n, id, nil); p.State != "done" {
		t.Fatalf("move down: %+v", p)
	}
	if _, err := os.Stat(filepath.Join(dir, "pub", "sub", "a.txt")); err == nil {
		t.Fatal("server copy still there")
	}

	// disconnect forgets the password: the next connect asks again
	if err := n.Disconnect(res.Path); err != nil {
		t.Fatal(err)
	}
	if len(n.Sessions()) != 0 {
		t.Fatalf("sessions after disconnect: %+v", n.Sessions())
	}
	if _, err := n.Connect("webdav://bob@" + host + "/pub"); err != nil || len(*asked) != 2 {
		t.Fatalf("reconnect: %v asked %v", err, *asked)
	}
}

func TestNetWrongPasswordAsksAgain(t *testing.T) {
	host, _ := davServer(t, "bob", "right")
	n, asked := testNet(t, map[string]string{"password": "right"})
	if _, err := n.Connect("webdav://bob:wrong@" + host + "/"); err != nil {
		t.Fatal(err)
	}
	if len(*asked) != 1 {
		t.Fatalf("asked %v", *asked)
	}
	n2, _ := testNet(t, nil)
	if _, err := n2.Connect("webdav://bob:wrong@" + host + "/"); err != errNetCancelled {
		t.Fatalf("got %v", err)
	}
}
