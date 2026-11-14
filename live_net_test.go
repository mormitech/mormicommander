//go:build live

package main

// Live tests of the Net menu against real FTP, FTPS, SFTP and WebDAV servers (board issue #106); see
// live_helpers_test.go for how they are configured and run.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"mormicommander/internal/ftp"
	"mormicommander/internal/syncdirs"
)

// liveTimeTolerance is how far a date read back from the server may be from the one sent: FTP's LIST shows minutes
// only, SFTP keeps seconds.
func liveTimeTolerance(kind string) time.Duration {
	if kind == "ftp" {
		return 61 * time.Second
	}
	return 2 * time.Second
}

// liveFileSuite is what a user does on a server panel, through the same calls the page makes: F7 (FS.Mkdir), F5 of a
// folder up and down (Net.StartJob), the panel listing (FS.List), Shift+F6 (FS.Rename), a resumed download, the
// download list (one queued transfer after the other, the second time with the "file is in the way" question), F5
// and F6 within the server, F6 from the server to the disk and back, and F8 (VfsJobs delete).
func liveFileSuite(e *liveEnv, s liveServer, work string) {
	t := e.t
	local := t.TempDir()
	up := filepath.Join(local, "up")
	tree := liveTree()
	blob := tree[1].data
	mt := time.Now().Add(-49 * time.Hour).Truncate(time.Second)
	writeTree(t, up, tree, mt)

	t.Run("F7", func(t *testing.T) {
		if err := e.f.Mkdir(work + "/made"); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(e.names(work), "made") {
			t.Fatalf("the new folder is not in the listing: %v", e.names(work))
		}
	})

	t.Run("F5 upload", func(t *testing.T) {
		p := e.mustXfer(NetJobRequest{Kind: "copy", Sources: []string{up}, Target: work})
		if p.FilesDone != len(tree) {
			t.Errorf("%d files copied, want %d", p.FilesDone, len(tree))
		}
		if got := strings.Join(e.names(work+"/up"), ","); got != "a.txt,bin,sub" {
			t.Fatalf("listing of the uploaded folder: %s", got)
		}
		if got := strings.Join(e.names(work+"/up/sub"), ","); got != "deep,empty.dat,with space.dat,árvíztűrő.txt" {
			t.Fatalf("listing of up/sub: %s", got)
		}
		for _, f := range tree {
			if en := e.entry(work + "/up/" + f.rel); en.Size != int64(len(f.data)) {
				t.Errorf("%s is %d bytes on the server, want %d (an FTP server that stores ASCII uploads with CRLF?)", f.rel, en.Size, len(f.data))
			}
		}
		if got := e.read(work + "/up/bin/blob.bin"); !bytes.Equal(got, blob) {
			t.Fatalf("blob.bin read back: %d bytes, content differs", len(got))
		}
		// the upload keeps the date where the server lets it be set
		en := e.entry(work + "/up/bin/blob.bin")
		d := time.UnixMilli(en.MTime).Sub(mt).Abs()
		switch {
		case s.kind == "webdav":
		case d <= liveTimeTolerance(s.kind):
		case s.kind == "ftp" && !e.f.router.Caps(work).Chtimes:
			t.Logf("the FTP server has no MFMT: the upload kept no date (%v off)", d)
		default:
			t.Errorf("blob.bin on the server dated %v, want %v", time.UnixMilli(en.MTime), mt)
		}
	})

	t.Run("Shift+F6", func(t *testing.T) {
		if err := e.f.Rename(work+"/up/a.txt", work+"/up/a-renamed.txt"); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(e.names(work+"/up"), ","); got != "a-renamed.txt,bin,sub" {
			t.Fatalf("after the rename: %s", got)
		}
	})
	renamed := append([]liveFile{{"a-renamed.txt", tree[0].data}}, tree[1:]...)

	down := filepath.Join(local, "down")
	os.MkdirAll(down, 0o755)
	t.Run("F5 download", func(t *testing.T) {
		e.mustXfer(NetJobRequest{Kind: "copy", Sources: []string{work + "/up"}, Target: down})
		checkTree(t, filepath.Join(down, "up"), renamed)
		if s.kind == "sftp" || (s.kind == "ftp" && e.f.router.Caps(work).Chtimes) {
			fi, err := os.Stat(filepath.Join(down, "up", "bin", "blob.bin"))
			if err != nil || fi.ModTime().Sub(mt).Abs() > liveTimeTolerance(s.kind) {
				t.Errorf("downloaded blob.bin dated %v, want %v (%v)", fi.ModTime(), mt, err)
			}
		}
	})

	t.Run("resume", func(t *testing.T) {
		part := filepath.Join(down, "blob.bin")
		os.WriteFile(part, blob[:len(blob)/3], 0o644)
		var offered bool
		p := e.xfer(NetJobRequest{Kind: "copy", Sources: []string{work + "/up/bin/blob.bin"}, Target: down}, func(p NetJobProgress) *NetJobAnswer {
			offered = p.Ask != nil && p.Ask.CanResume
			return &NetJobAnswer{Choice: "resume"}
		})
		if p.State != "done" || !offered {
			t.Fatalf("resume: %s %v, resume offered %v", p.State, p.Errors, offered)
		}
		if got, _ := os.ReadFile(part); !bytes.Equal(got, blob) {
			t.Fatalf("resumed download: %d bytes, content differs", len(got))
		}
	})

	t.Run("download list", func(t *testing.T) {
		// The page's download list (cm_FtpDownloadList) runs its queued items one by one as transfers that ask when a
		// file is in the way (overwrite "").
		dl := t.TempDir()
		queue := []string{work + "/up/sub/deep/c.txt", work + "/up/bin/blob.bin", work + "/up/sub/with space.dat"}
		for _, src := range queue {
			e.mustXfer(NetJobRequest{Kind: "copy", Sources: []string{src}, Target: dl})
		}
		checkTree(t, dl, []liveFile{{"c.txt", []byte("gamma\n")}, {"blob.bin", blob}, {"with space.dat", []byte("spaces in the name")}})
		// run again: c.txt is in the way; skip keeps the local file, overwrite replaces it
		os.WriteFile(filepath.Join(dl, "c.txt"), []byte("local"), 0o644)
		p := e.xfer(NetJobRequest{Kind: "copy", Sources: []string{queue[0]}, Target: dl}, func(NetJobProgress) *NetJobAnswer { return &NetJobAnswer{Choice: "skip"} })
		if b, _ := os.ReadFile(filepath.Join(dl, "c.txt")); p.State != "done" || string(b) != "local" {
			t.Fatalf("skip: %s %v, c.txt %q", p.State, p.Errors, b)
		}
		p = e.xfer(NetJobRequest{Kind: "copy", Sources: []string{queue[0]}, Target: dl}, func(NetJobProgress) *NetJobAnswer { return &NetJobAnswer{Choice: "overwrite"} })
		if b, _ := os.ReadFile(filepath.Join(dl, "c.txt")); p.State != "done" || string(b) != "gamma\n" {
			t.Fatalf("overwrite: %s %v, c.txt %q", p.State, p.Errors, b)
		}
	})

	t.Run("F5 within the server", func(t *testing.T) {
		e.mustXfer(NetJobRequest{Kind: "copy", Sources: []string{work + "/up"}, Target: work + "/made"})
		if got := e.read(work + "/made/up/bin/blob.bin"); !bytes.Equal(got, blob) {
			t.Fatal("blob.bin copied on the server differs")
		}
		if got := string(e.read(work + "/made/up/sub/árvíztűrő.txt")); got != "accents in the name\n" {
			t.Fatalf("árvíztűrő.txt copied on the server: %q", got)
		}
	})

	t.Run("F6 within the server", func(t *testing.T) {
		e.mustXfer(NetJobRequest{Kind: "move", Sources: []string{work + "/made/up"}, Target: work, Rename: "moved"})
		if e.exists(work + "/made/up") {
			t.Fatal("the moved folder is still in its old place")
		}
		if got := e.read(work + "/moved/bin/blob.bin"); !bytes.Equal(got, blob) {
			t.Fatal("blob.bin after the move differs")
		}
	})

	t.Run("F6 down and up", func(t *testing.T) {
		back := t.TempDir()
		e.mustXfer(NetJobRequest{Kind: "move", Sources: []string{work + "/moved/a-renamed.txt"}, Target: back})
		if e.exists(work + "/moved/a-renamed.txt") {
			t.Fatal("the file moved down is still on the server")
		}
		if b, _ := os.ReadFile(filepath.Join(back, "a-renamed.txt")); !bytes.Equal(b, tree[0].data) {
			t.Fatalf("moved down: %q", b)
		}
		e.mustXfer(NetJobRequest{Kind: "move", Sources: []string{filepath.Join(back, "a-renamed.txt")}, Target: work})
		if _, err := os.Stat(filepath.Join(back, "a-renamed.txt")); err == nil {
			t.Fatal("the file moved up is still on the disk")
		}
		if got := e.read(work + "/a-renamed.txt"); !bytes.Equal(got, tree[0].data) {
			t.Fatalf("moved up: %q", got)
		}
	})

	t.Run("F8", func(t *testing.T) {
		job := e.vfsJob(VfsRequest{Kind: "delete", Sources: []string{work + "/moved", work + "/a-renamed.txt"}})
		if len(job.p.Errors) > 0 {
			t.Fatalf("delete: %v", job.p.Errors)
		}
		for _, p := range []string{work + "/moved", work + "/a-renamed.txt"} {
			if e.exists(p) {
				t.Errorf("%s is still there after F8", p)
			}
		}
	})
}

// ---- FTP ----------------------------------------------------------------------------------------------------------

// Typing an ftp:// address without a password: the password is asked once and kept for the session; files work in
// passive mode; Disconnect forgets the password.
func TestLiveFTPPassive(t *testing.T) {
	s := liveFTP(t)
	e := newLiveEnv(t, answers(map[string]string{"password": s.pass}))
	res, err := e.n.Connect(s.url())
	if err != nil {
		t.Fatal(err)
	}
	if e.askedKinds() != "password" || res.Path != s.url() {
		t.Fatalf("connect: %+v, asked %s", res, e.askedKinds())
	}
	if st := e.n.Status(res.Path); !st.Connected || st.Kind != "ftp" {
		t.Fatalf("status %+v", st)
	}
	liveFileSuite(e, s, e.workDir(s))
	log := strings.Join(e.n.Log(res.Path), "\n")
	if !strings.Contains(log, "PASV") && !strings.Contains(log, "EPSV") {
		t.Errorf("no passive data connection in the protocol log")
	}
	if strings.Contains(log, s.pass) && len(s.pass) > 2 {
		t.Error("the password is in the protocol log")
	}
	if len(e.askedKinds()) != len("password") {
		t.Fatalf("asked again during the work: %s", e.askedKinds())
	}
	if err := e.n.Disconnect(res.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := e.n.Connect(s.url()); err != nil || e.askedKinds() != "password,password" {
		t.Fatalf("after Disconnect: %v, asked %s", err, e.askedKinds())
	}
}

// A wrong password is refused by the real server and asked again with the "did not accept" text.
func TestLiveFTPWrongPassword(t *testing.T) {
	s := liveFTP(t)
	typed := []string{s.pass + "-wrong", s.pass}
	var messages []string
	e := newLiveEnv(t, func(kind string, data map[string]any) (string, bool) {
		if kind != "password" || len(typed) == 0 {
			return "", false
		}
		msg, _ := data["message"].(string)
		messages = append(messages, msg)
		v := typed[0]
		typed = typed[1:]
		return v, true
	})
	if _, err := e.n.Connect(s.url()); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0] != "" || !strings.Contains(messages[1], "did not accept") {
		t.Fatalf("password questions: %q", messages)
	}
	// cancelling the question gives the plain "cancelled" error
	e2 := newLiveEnv(t, nil)
	other := s
	other.user += "x"
	if _, err := e2.n.Connect(other.url()); err != errNetCancelled {
		t.Fatalf("cancelled: %v", err)
	}
}

// A saved connection in active mode (PORT/EPRT) with its password saved under the master password.
func TestLiveFTPActiveSavedSite(t *testing.T) {
	s := liveFTP(t)
	if os.Getenv("MC_LIVE_FTP_ACTIVE") != "1" {
		liveMissing(t, "ftpactive", "MC_LIVE_FTP_ACTIVE is not 1")
	}
	e := newLiveEnv(t, answers(map[string]string{"master-new": "m4ster", "master": "m4ster"}))
	id, err := e.n.SaveSite(NetSite{Name: "Live active", Protocol: "ftp", Host: s.host, Port: s.port, User: s.user,
		Password: s.pass, SavePassword: true, Passive: false, RemoteDir: s.dir})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.n.ConnectSite(id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.askedKinds(), "password") && !strings.Contains(e.askedKinds(), "master") {
		t.Fatalf("asked %s", e.askedKinds())
	}
	work := e.workDir(s)
	local := t.TempDir()
	tree := liveTree()
	writeTree(t, local, tree, time.Now().Add(-time.Hour))
	e.mustXfer(NetJobRequest{Kind: "copy", Sources: []string{filepath.Join(local, "bin"), filepath.Join(local, "sub")}, Target: work})
	down := t.TempDir()
	e.mustXfer(NetJobRequest{Kind: "copy", Sources: []string{work + "/bin", work + "/sub"}, Target: down})
	checkTree(t, down, tree[1:])
	log := strings.Join(e.n.Log(res.Path), "\n")
	if !strings.Contains(log, "PORT") && !strings.Contains(log, "EPRT") {
		t.Fatalf("no active data connection in the protocol log:\n%s", log)
	}
}

// Explicit FTPS (ftpes://): AUTH TLS on the control connection and protected data connections, with the server's
// certificate checked against the system's trusted certificates (CI adds its test CA there).
func TestLiveFTPSExplicit(t *testing.T) {
	s := liveFTP(t)
	if os.Getenv("MC_LIVE_FTPS") != "1" {
		liveMissing(t, "ftps", "MC_LIVE_FTPS is not 1")
	}
	s.scheme = "ftpes"
	e := newLiveEnv(t, answers(map[string]string{"password": s.pass}))
	if ca := os.Getenv("MC_LIVE_FTP_CA"); ca != "" {
		// The app trusts the system's certificates only (it has no "accept this certificate" question yet). Where the
		// test CA can't go into the system's store (a developer's Mac), it is handed to the TLS config directly.
		pem, err := os.ReadFile(ca)
		if err != nil {
			t.Fatal(err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			t.Fatalf("MC_LIVE_FTP_CA %s holds no certificate", ca)
		}
		r := e.n.router()
		prev := r.FTP.Configure
		r.FTP.Configure = func(c *ftp.Config) error {
			c.TLSConfig = &tls.Config{RootCAs: pool}
			return prev(c)
		}
		t.Cleanup(func() { r.FTP.Configure = prev })
	}
	res, err := e.n.Connect(s.url())
	if err != nil {
		t.Fatal(err)
	}
	log := strings.Join(e.n.Log(res.Path), "\n")
	if !strings.Contains(log, "AUTH TLS") || !strings.Contains(log, "PROT P") {
		t.Fatalf("no TLS in the protocol log:\n%s", log)
	}
	liveFileSuite(e, s, e.workDir(s))
}

// ---- SFTP ---------------------------------------------------------------------------------------------------------

// liveHostKey is the server's public host key from MC_LIVE_SFTP_HOSTKEY (nil when not given).
func liveHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	p := os.Getenv("MC_LIVE_SFTP_HOSTKEY")
	if p == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	k, _, _, _, err := ssh.ParseAuthorizedKey(b)
	if err != nil {
		t.Fatalf("MC_LIVE_SFTP_HOSTKEY: %v", err)
	}
	return k
}

// The first connection shows the real server's key (its fingerprint is the one of the key file), saves it in the app's
// own known_hosts, then asks for the password; the files work; a new run trusts the saved key and asks only for the
// password.
func TestLiveSFTPPasswordAndHostKey(t *testing.T) {
	s := liveSFTP(t)
	if s.pass == "" {
		liveMissing(t, "sftp", "MC_LIVE_SFTP_PASS is not set")
	}
	hk := liveHostKey(t)
	var shown string
	e := newLiveEnv(t, func(kind string, data map[string]any) (string, bool) {
		switch kind {
		case "hostkey":
			shown, _ = data["fingerprint"].(string)
			return "save", true
		case "password", "interactive": // (a server may take the password as keyboard-interactive)
			return s.pass, true
		}
		return "", false
	})
	res, err := e.n.Connect(s.url())
	if err != nil {
		t.Fatal(err)
	}
	login := e.askedKinds()
	if login != "hostkey,password" && login != "hostkey,interactive" {
		t.Fatalf("asked %s, want the host key, then the password", login)
	}
	kh, _ := os.ReadFile(filepath.Join(e.cfg, "known_hosts"))
	if len(kh) == 0 {
		t.Fatal("the host key was not saved in the app's known_hosts")
	}
	if hk != nil {
		if shown != ssh.FingerprintSHA256(hk) {
			t.Fatalf("the key shown has the fingerprint %s, the server's is %s", shown, ssh.FingerprintSHA256(hk))
		}
		if !strings.Contains(string(kh), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hk)))) {
			t.Fatalf("known_hosts does not hold the server's key:\n%s", kh)
		}
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")); err == nil {
		t.Fatal("~/.ssh/known_hosts was written")
	}
	if st := e.n.Status(res.Path); !st.Connected || st.Kind != "sftp" {
		t.Fatalf("status %+v", st)
	}
	liveFileSuite(e, s, e.workDir(s))
	if e.askedKinds() != login {
		t.Fatalf("asked again during the work: %s", e.askedKinds())
	}

	// a new run: the saved key is trusted, only the password is asked
	e.n.disconnectSession(res.Path, true)
	e.resetAsked()
	n2 := e.newNet(answers(map[string]string{"password": s.pass, "interactive": s.pass}))
	if _, err := n2.Connect(s.url()); err != nil {
		t.Fatal(err)
	}
	if e.askedKinds() != strings.TrimPrefix(login, "hostkey,") {
		t.Fatalf("new run asked %s, want only the password", e.askedKinds())
	}
}

// liveInstallKey copies the test key to ~/.ssh under the name OpenSSH gives its type, and returns that path.
func liveInstallKey(t *testing.T) string {
	t.Helper()
	p := os.Getenv("MC_LIVE_SFTP_KEY")
	if p == "" {
		liveMissing(t, "sftp", "MC_LIVE_SFTP_KEY is not set")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	k, err := ssh.ParseRawPrivateKey(b)
	if err != nil {
		t.Fatalf("MC_LIVE_SFTP_KEY: %v", err)
	}
	name := "id_rsa"
	switch k.(type) {
	case *ed25519.PrivateKey, ed25519.PrivateKey:
		name = "id_ed25519"
	default:
		if strings.Contains(strings.ToLower(string(b)), "ec private") {
			name = "id_ecdsa"
		}
	}
	dir := filepath.Join(os.Getenv("HOME"), ".ssh")
	os.MkdirAll(dir, 0o700)
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}

// A key in ~/.ssh logs in without any question when the server's key is already in the app's known_hosts; a saved
// connection with a key file (agent off) does too.
func TestLiveSFTPKey(t *testing.T) {
	s := liveSFTP(t)
	hk := liveHostKey(t)
	if hk == nil {
		liveMissing(t, "sftp", "MC_LIVE_SFTP_HOSTKEY is not set")
	}
	e := newLiveEnv(t, nil) // no question may be asked
	key := liveInstallKey(t)
	line := knownhosts.Line([]string{knownhosts.Normalize(s.hostPort(22))}, hk)
	if err := os.WriteFile(filepath.Join(e.cfg, "known_hosts"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := e.n.Connect(s.url())
	if err != nil {
		t.Fatalf("key login: %v (asked %s)", err, e.askedKinds())
	}
	if e.askedKinds() != "" {
		t.Fatalf("asked %s", e.askedKinds())
	}
	work := e.workDir(s)
	local := t.TempDir()
	writeTree(t, local, liveTree()[:2], time.Now().Add(-time.Hour))
	e.mustXfer(NetJobRequest{Kind: "copy", Sources: []string{filepath.Join(local, "a.txt"), filepath.Join(local, "bin")}, Target: work})
	if got := strings.Join(e.names(work), ","); got != "a.txt,bin" {
		t.Fatalf("listing %s", got)
	}
	e.n.disconnectSession(res.Path, true)

	// a saved connection with the key file somewhere else and the agent off
	other := filepath.Join(t.TempDir(), "live-key")
	b, _ := os.ReadFile(key)
	os.WriteFile(other, b, 0o600)
	os.Remove(key)
	id, err := e.n.SaveSite(NetSite{Name: "Live key", Protocol: "sftp", Host: s.host, Port: s.port, User: s.user, KeyFile: other, RemoteDir: s.dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.n.ConnectSite(id); err != nil {
		t.Fatalf("saved connection with a key file: %v (asked %s)", err, e.askedKinds())
	}
	if got := strings.Join(e.names(work), ","); got != "a.txt,bin" || e.askedKinds() != "" {
		t.Fatalf("listing %s, asked %s", got, e.askedKinds())
	}
}

// A saved key that is not the server's: the warning is shown, refusing it does not connect.
func TestLiveSFTPChangedHostKeyRefused(t *testing.T) {
	s := liveSFTP(t)
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := ssh.NewPublicKey(pub)
	var warned bool
	e := newLiveEnv(t, func(kind string, data map[string]any) (string, bool) {
		msg, _ := data["message"].(string)
		warned = warned || kind == "hostkey" && strings.Contains(msg, "WARNING")
		return "", false
	})
	// an ed25519 key of another server under this host name; the real server offers an ed25519 key first (OpenSSH's
	// default order), so this is "changed", not "new kind"
	line := knownhosts.Line([]string{knownhosts.Normalize(s.hostPort(22))}, other)
	os.WriteFile(filepath.Join(e.cfg, "known_hosts"), []byte(line+"\n"), 0o600)
	_, err = e.n.Connect(s.url())
	if err == nil || !strings.Contains(err.Error(), "was not accepted") {
		t.Fatalf("got %v, want the key refused", err)
	}
	if !warned || e.askedKinds() != "hostkey" {
		t.Fatalf("warned %v, asked %s", warned, e.askedKinds())
	}
}

// ---- WebDAV -------------------------------------------------------------------------------------------------------

// A WebDAV share: the password is asked after the first 401, and everything a server panel does works.
func TestLiveWebDAV(t *testing.T) {
	s := liveDAV(t)
	e := newLiveEnv(t, answers(map[string]string{"password": s.pass}))
	res, err := e.n.Connect(s.url())
	if err != nil {
		t.Fatal(err)
	}
	if s.pass != "" && e.askedKinds() != "password" {
		t.Fatalf("asked %s", e.askedKinds())
	}
	if st := e.n.Status(res.Path); !st.Connected || st.Kind != "webdav" {
		t.Fatalf("status %+v", st)
	}
	liveFileSuite(e, s, e.workDir(s))
}

// ---- between servers ----------------------------------------------------------------------------------------------

// F5 from one server straight to another: FTP → SFTP → WebDAV → the disk.
func TestLiveCopyBetweenServers(t *testing.T) {
	ftpS, sftpS, davS := liveFTP(t), liveSFTP(t), liveDAV(t)
	e := newLiveEnv(t, func(kind string, data map[string]any) (string, bool) {
		title, _ := data["title"].(string)
		switch {
		case kind == "hostkey":
			return "once", true
		case kind == "password" && strings.Contains(title, ftpS.user+" on "+ftpS.host) && !strings.Contains(title, sftpS.user+" on "+sftpS.host):
			return ftpS.pass, true
		case (kind == "password" || kind == "interactive") && strings.Contains(title, sftpS.user+" on "+sftpS.host):
			return sftpS.pass, true
		case kind == "password":
			return davS.pass, true
		}
		return "", false
	})
	// one user name may serve FTP and SFTP alike: tell the passwords apart by the URL that asks
	if ftpS.user == sftpS.user && ftpS.host == sftpS.host && ftpS.pass != sftpS.pass {
		t.Skip("FTP and SFTP have the same user and host but different passwords; the question can't tell them apart")
	}
	for _, u := range []string{ftpS.url(), sftpS.url(), davS.url()} {
		if _, err := e.n.Connect(u); err != nil {
			t.Fatalf("connect %s: %v", u, err)
		}
	}
	wf, ws, wd := e.workDir(ftpS), e.workDir(sftpS), e.workDir(davS)
	local := t.TempDir()
	tree := liveTree()
	writeTree(t, local, tree, time.Now().Add(-time.Hour))
	e.mustXfer(NetJobRequest{Kind: "copy", Sources: []string{filepath.Join(local, "bin"), filepath.Join(local, "sub")}, Target: wf})
	e.mustXfer(NetJobRequest{Kind: "copy", Sources: []string{wf + "/bin", wf + "/sub"}, Target: ws})
	e.mustXfer(NetJobRequest{Kind: "move", Sources: []string{ws + "/bin", ws + "/sub"}, Target: wd})
	if e.exists(ws+"/bin") || e.exists(ws+"/sub") {
		t.Fatal("F6 from SFTP to WebDAV left the sources")
	}
	down := t.TempDir()
	e.mustXfer(NetJobRequest{Kind: "copy", Sources: []string{wd + "/bin", wd + "/sub"}, Target: down})
	checkTree(t, down, tree[1:])
}

// ---- Synchronize folders ------------------------------------------------------------------------------------------

// liveSyncSuite compares a local folder with a server folder (typed as a URL, as in the dialog), synchronizes both
// ways (copies up and down, a delete on the server), and compares again: every file must then be equal. Where the
// server keeps the dates (SFTP; FTP with MFMT) the compare is by date with the server's precision; where it can't
// (WebDAV, FTP without MFMT), the sync says so and the second compare is by content.
func liveSyncSuite(e *liveEnv, s liveServer) {
	t := e.t
	if _, err := e.n.Connect(s.url()); err != nil { // the panel is on the server before F7 makes the test's folder
		t.Fatal(err)
	}
	work := e.workDir(s)
	sd := liveSyncApp(e)
	oldT, newT := time.Now().Add(-48*time.Hour).Truncate(time.Second), time.Now().Add(-time.Hour).Truncate(time.Second)

	// the server side is put there by the app's own upload, which keeps the dates where the server can
	seed := t.TempDir()
	scWrite(t, filepath.Join(seed, "same.txt"), "same", oldT)
	scWrite(t, filepath.Join(seed, "up.txt"), "older there", oldT)
	scWrite(t, filepath.Join(seed, "down.txt"), "only on the server", newT)
	scWrite(t, filepath.Join(seed, "gone.txt"), "delete me", newT)
	scWrite(t, filepath.Join(seed, "olddir", "inner.txt"), "in a folder only on the server", newT)
	var sources []string
	for _, n := range []string{"same.txt", "up.txt", "down.txt", "gone.txt", "olddir"} {
		sources = append(sources, filepath.Join(seed, n))
	}
	e.mustXfer(NetJobRequest{Kind: "copy", Sources: sources, Target: work})

	l := t.TempDir()
	scWrite(t, filepath.Join(l, "same.txt"), "same", oldT)
	scWrite(t, filepath.Join(l, "up.txt"), "newer here", newT)
	scWrite(t, filepath.Join(l, "dir", "new.txt"), "new folder", newT)

	opts := syncdirs.DefaultOptions()
	opts.ToleranceSeconds = int(liveTimeTolerance(s.kind) / time.Second)
	id := sd.Open()
	defer sd.Close(id)
	v := liveSyncCompare(t, sd, id, l, work, opts)
	times := v.RightSide.Times
	t.Logf("%s side: %+v", s.kind, v.RightSide)
	if v.RightSide.ReadOnly || v.RightSide.Trash || strings.Contains(v.Right, s.pass) && len(s.pass) > 2 {
		t.Fatalf("right side %+v %s", v.RightSide, v.Right)
	}
	if want := map[string]string{"ftp": "ftp", "sftp": "sftp", "webdav": "other"}[s.kind]; v.RightSide.Kind != want {
		t.Fatalf("right side kind %q, want %q", v.RightSide.Kind, want)
	}
	if !times {
		// the dates on the server are the upload's: compare by content, choose the copy up by hand
		opts = syncdirs.Options{Subdirs: true, ByContent: true, IgnoreDate: true}
		v = liveSyncCompare(t, sd, id, l, work, opts)
		if _, err := sd.SetAction(id, []string{"up.txt"}, syncdirs.CopyLeftToRight, false); err != nil {
			t.Fatal(err)
		}
	} else {
		if c := liveCategory(t, v, "same.txt"); c != syncdirs.Equal {
			t.Fatalf("same.txt is %v (dates not kept by the upload, or read back wrongly?)", c)
		}
		if c := liveCategory(t, v, "up.txt"); c != syncdirs.LeftNewer {
			t.Fatalf("up.txt is %v, want newer on the left", c)
		}
	}
	if c := liveCategory(t, v, "down.txt"); c != syncdirs.RightOnly {
		t.Fatalf("down.txt is %v", c)
	}
	if c := liveCategory(t, v, "dir/new.txt"); c != syncdirs.LeftOnly {
		t.Fatalf("dir/new.txt is %v", c)
	}
	if _, err := sd.SetAction(id, []string{"gone.txt"}, syncdirs.DeleteRight, false); err != nil {
		t.Fatal(err)
	}
	done := liveSyncRun(t, sd, id)
	warnings, _ := done["warnings"].([]string)
	if !times && (len(warnings) == 0 || !strings.Contains(strings.Join(warnings, " "), "can't set file dates")) {
		t.Errorf("the lost dates were not reported: %v", warnings)
	}
	if got := string(e.read(work + "/up.txt")); got != "newer here" {
		t.Fatalf("up.txt on the server: %q", got)
	}
	if got := string(e.read(work + "/dir/new.txt")); got != "new folder" {
		t.Fatalf("dir/new.txt on the server: %q", got)
	}
	if b, _ := os.ReadFile(filepath.Join(l, "down.txt")); string(b) != "only on the server" {
		t.Fatalf("down.txt here: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(l, "olddir", "inner.txt")); string(b) != "in a folder only on the server" {
		t.Fatalf("olddir/inner.txt here: %q", b)
	}
	if e.exists(work + "/gone.txt") {
		t.Fatal("gone.txt is still on the server")
	}
	for _, n := range e.names(work) {
		if strings.HasPrefix(n, ".mcsync") {
			t.Errorf("a temporary file was left on the server: %s", n)
		}
	}
	if times {
		if en := e.entry(work + "/up.txt"); time.UnixMilli(en.MTime).Sub(newT).Abs() > liveTimeTolerance(s.kind) {
			t.Errorf("up.txt on the server dated %v, want %v", time.UnixMilli(en.MTime), newT)
		}
	}
	v = liveSyncCompare(t, sd, id, l, work, opts)
	if !v.Summary.AllEqual() {
		var bad []string
		for _, it := range v.Rows {
			if it.Category != syncdirs.Equal {
				bad = append(bad, it.Path+"="+it.Category.String())
			}
		}
		t.Fatalf("not all equal after the sync: %v", bad)
	}
	v = liveSyncCompare(t, sd, id, l, work, syncdirs.Options{Subdirs: true, ByContent: true, IgnoreDate: !times, ToleranceSeconds: opts.ToleranceSeconds})
	if !v.Summary.AllEqual() {
		t.Fatalf("not all equal by content after the sync: %+v", v.Summary)
	}
}

// Synchronize folders with an FTP side (issue #9 never tried a real one).
func TestLiveSyncFTP(t *testing.T) {
	s := liveFTP(t)
	liveSyncSuite(newLiveEnv(t, answers(map[string]string{"password": s.pass})), s)
}

func TestLiveSyncSFTP(t *testing.T) {
	s := liveSFTP(t)
	liveSyncSuite(newLiveEnv(t, answers(map[string]string{"password": s.pass, "interactive": s.pass, "hostkey": "once"})), s)
}

func TestLiveSyncWebDAV(t *testing.T) {
	s := liveDAV(t)
	liveSyncSuite(newLiveEnv(t, answers(map[string]string{"password": s.pass})), s)
}
