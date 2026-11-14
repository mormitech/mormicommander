//go:build live

package main

// Live integration tests (board issue #106): the app's own Go code paths (the Net service and the vfs router, the
// transfer jobs F5/F6 use, VfsJobs for F8 and archives, Synchronize folders, Find files and Feed to listbox) against
// real servers and real archive tools, not the in-process test servers of the ordinary tests.
//
// They are behind the "live" build tag, so `go test ./...` never compiles them; CI's "live" job (.github/workflows/
// build.yml) starts vsftpd (FTP, explicit FTPS, passive and active), OpenSSH (SFTP with a password and with a key) and
// Apache mod_dav (WebDAV) on the runner, installs 7-Zip and unrar, and runs
//
//	go test -tags live -run '^TestLive' -v .
//
// Each test reads its server from environment variables and skips when they are not set, unless MC_LIVE_REQUIRE
// names it ("ftp,ftps,ftpactive,sftp,webdav,7z,unrar" or "all"): then a missing server or tool fails the test, so CI
// can't pass by skipping. Every test works in a folder of its own on the server (mc-live-<test>-<time>), removed at
// the end, and points HOME and the settings folder at temporary folders, so the user's keys, agent, known_hosts and
// settings are never read or written.
//
//	MC_LIVE_FTP_HOST, MC_LIVE_FTP_PORT, MC_LIVE_FTP_USER, MC_LIVE_FTP_PASS, MC_LIVE_FTP_DIR (a writable folder)
//	MC_LIVE_FTPS=1         the FTP server also takes explicit TLS (AUTH TLS); MC_LIVE_FTP_CA: its CA certificate
//	MC_LIVE_FTP_ACTIVE=1   active mode (PORT/EPRT) works against it
//	MC_LIVE_SFTP_HOST, MC_LIVE_SFTP_PORT, MC_LIVE_SFTP_USER, MC_LIVE_SFTP_PASS, MC_LIVE_SFTP_DIR
//	MC_LIVE_SFTP_KEY       a private key (no passphrase) the server takes for MC_LIVE_SFTP_USER
//	MC_LIVE_SFTP_HOSTKEY   the server's public host key (a .pub file), to check the key the app saves
//	MC_LIVE_DAV_URL        e.g. webdav://127.0.0.1:8080/dav; MC_LIVE_DAV_USER, MC_LIVE_DAV_PASS
//	7-Zip (7zz, 7z or 7za) and unrar are found on PATH, the way the app finds 7-Zip.

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"mormicommander/internal/keychain"
	"mormicommander/internal/syncdirs"
	"mormicommander/internal/vfs"
)

// liveTimeout bounds every wait on a server: generous, a slow runner is not a failure.
const liveTimeout = 2 * time.Minute

// liveRequired says whether MC_LIVE_REQUIRE names kind (or "all").
func liveRequired(kind string) bool {
	for _, k := range strings.Split(os.Getenv("MC_LIVE_REQUIRE"), ",") {
		k = strings.TrimSpace(k)
		if k == kind || k == "all" {
			return true
		}
	}
	return false
}

// liveMissing skips the test, or fails it when kind is required.
func liveMissing(t *testing.T, kind, why string) {
	t.Helper()
	if liveRequired(kind) {
		t.Fatalf("%s is required (MC_LIVE_REQUIRE) but %s", kind, why)
	}
	t.Skipf("no live %s: %s", kind, why)
}

// liveServer is one real server from the environment.
type liveServer struct {
	kind   string // ftp, sftp, webdav
	scheme string // ftp, ftpes, sftp, webdav, webdavs
	host   string
	port   int
	user   string
	pass   string
	dir    string // a writable folder on the server ("/" style)
}

// liveFTP is the FTP server, or the test is skipped (or failed when required).
func liveFTP(t *testing.T) liveServer {
	t.Helper()
	return liveFromEnv(t, "ftp", "FTP", "ftp")
}

func liveSFTP(t *testing.T) liveServer {
	t.Helper()
	return liveFromEnv(t, "sftp", "SFTP", "sftp")
}

func liveFromEnv(t *testing.T, kind, env, scheme string) liveServer {
	t.Helper()
	s := liveServer{kind: kind, scheme: scheme, host: os.Getenv("MC_LIVE_" + env + "_HOST"), user: os.Getenv("MC_LIVE_" + env + "_USER"),
		pass: os.Getenv("MC_LIVE_" + env + "_PASS"), dir: os.Getenv("MC_LIVE_" + env + "_DIR")}
	if s.host == "" || s.user == "" {
		liveMissing(t, kind, "MC_LIVE_"+env+"_HOST and MC_LIVE_"+env+"_USER are not set")
	}
	if p := os.Getenv("MC_LIVE_" + env + "_PORT"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("MC_LIVE_%s_PORT: %v", env, err)
		}
		s.port = n
	}
	if s.dir == "" {
		s.dir = "/"
	}
	return s
}

// liveDAV is the WebDAV share.
func liveDAV(t *testing.T) liveServer {
	t.Helper()
	raw := os.Getenv("MC_LIVE_DAV_URL")
	if raw == "" {
		liveMissing(t, "webdav", "MC_LIVE_DAV_URL is not set")
	}
	u, err := netParseURL(raw)
	if err != nil || netSchemes[u.Scheme] != "webdav" {
		t.Fatalf("MC_LIVE_DAV_URL %q is not a webdav:// address: %v", raw, err)
	}
	s := liveServer{kind: "webdav", scheme: u.Scheme, host: u.Host, port: u.Port, user: os.Getenv("MC_LIVE_DAV_USER"),
		pass: os.Getenv("MC_LIVE_DAV_PASS"), dir: u.Path}
	if s.dir == "" {
		s.dir = "/"
	}
	return s
}

// root is the server's address without a folder (user@host:port), as the app writes it.
func (s liveServer) root() string {
	u := netURL{Scheme: s.scheme, User: s.user, Host: s.host, Port: s.port}
	if u.Port == netDefaultPorts[u.Scheme] {
		u.Port = 0
	}
	return u.key()
}

// url is the panel address of a folder or file under the server's folder: url("a", "b") → <root><dir>/a/b.
func (s liveServer) url(parts ...string) string {
	return s.root() + path.Join(append([]string{"/", s.dir}, parts...)...)
}

// hostPort is host:port for known_hosts.
func (s liveServer) hostPort(def int) string {
	p := s.port
	if p == 0 {
		p = def
	}
	if p == def {
		return s.host
	}
	return "[" + s.host + "]:" + strconv.Itoa(p)
}

// liveEnv is an app for the live tests: the panels' FS and router, VfsJobs (F8, archives), and the Net service,
// whose questions answer() answers (kind and the question's data → the value or choice, ok). asked records the kinds.
type liveEnv struct {
	t     *testing.T
	f     *FS
	js    *VfsJobs
	n     *Net
	cfg   string // the settings folder (known_hosts of the app)
	mu    sync.Mutex
	asked []string
}

func (e *liveEnv) askedKinds() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.asked, ",")
}

func (e *liveEnv) resetAsked() {
	e.mu.Lock()
	e.asked = nil
	e.mu.Unlock()
}

// newLiveEnv isolates HOME, the SSH agent and the settings folder and builds the app's services.
func newLiveEnv(t *testing.T, answer func(kind string, data map[string]any) (string, bool)) *liveEnv {
	t.Helper()
	cfg := netSSHIsolate(t)
	f, js := vfsTestApp(t)
	e := &liveEnv{t: t, f: f, js: js, cfg: cfg}
	e.n = e.newNet(answer)
	if e.n.router() != f.router {
		t.Fatal("the Net service does not use the panels' router")
	}
	return e
}

// newNet makes another Net on the same router (a "new run" of the connection layer).
func (e *liveEnv) newNet(answer func(kind string, data map[string]any) (string, bool)) *Net {
	n := newNet(e.f.app)
	n.openKeychain = func() (keychain.Keychain, error) { return keychain.NewFake(), nil } // never the real keychain
	n.asker = func(kind string, data map[string]any) (map[string]any, bool) {
		e.mu.Lock()
		e.asked = append(e.asked, kind)
		e.mu.Unlock()
		if answer == nil {
			return nil, false
		}
		v, ok := answer(kind, data)
		if !ok {
			return nil, false
		}
		return map[string]any{"ok": true, "value": v, "choice": v}, true
	}
	e.t.Cleanup(func() {
		for _, s := range n.Sessions() {
			n.disconnectSession(s.URL, true)
		}
		n.close()
	})
	return n
}

// answers is an answer function from a fixed map kind → value.
func answers(m map[string]string) func(string, map[string]any) (string, bool) {
	return func(kind string, _ map[string]any) (string, bool) { v, ok := m[kind]; return v, ok }
}

func liveCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), liveTimeout)
	t.Cleanup(cancel)
	return ctx
}

// workDir makes a fresh folder for the test under the server's folder (F7, FS.Mkdir) and removes it at the end.
func (e *liveEnv) workDir(s liveServer) string {
	e.t.Helper()
	name := fmt.Sprintf("mc-live-%s-%s", strings.ToLower(strings.NewReplacer("/", "-", " ", "-").Replace(e.t.Name())),
		strconv.FormatInt(time.Now().UnixNano(), 36))
	p := s.url(name)
	if err := e.f.Mkdir(p); err != nil {
		e.t.Fatalf("F7 on the server: %v", err)
	}
	r := e.f.router
	e.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), liveTimeout)
		defer cancel()
		if err := vfs.RemoveAll(ctx, r, p); err != nil {
			e.t.Logf("cleanup of %s: %v", p, err)
		}
	})
	return p
}

// names lists a folder the way a panel does (FS.List) and returns the names, sorted.
func (e *liveEnv) names(p string) []string {
	e.t.Helper()
	l, err := e.f.List("live", p)
	if err != nil {
		e.t.Fatalf("listing %s: %v", p, err)
	}
	var out []string
	for _, r := range l.Rows {
		if n := r[0].(string); n != ".." {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// entry stats a path through the router.
func (e *liveEnv) entry(p string) vfs.Entry {
	e.t.Helper()
	en, err := e.f.router.Stat(p)
	if err != nil {
		e.t.Fatalf("stat %s: %v", p, err)
	}
	return en
}

func (e *liveEnv) exists(p string) bool {
	_, err := e.f.router.Stat(p)
	return err == nil
}

// read reads a file through the router (what F3 on a server does).
func (e *liveEnv) read(p string) []byte {
	e.t.Helper()
	rc, err := vfs.OpenFile(liveCtx(e.t), e.f.router, p)
	if err != nil {
		e.t.Fatalf("open %s: %v", p, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		e.t.Fatalf("read %s: %v", p, err)
	}
	return b
}

// xfer runs a transfer of the Net service (F5/F6 to, from and between servers; the download list) and waits for it.
// answer answers the "file is in the way" question (nil: none expected).
func (e *liveEnv) xfer(req NetJobRequest, answer func(NetJobProgress) *NetJobAnswer) NetJobProgress {
	e.t.Helper()
	id, err := e.n.StartJob(req)
	if err != nil {
		e.t.Fatalf("%s %v → %s: %v", req.Kind, req.Sources, req.Target, err)
	}
	deadline := time.Now().Add(liveTimeout)
	asked := false
	for time.Now().Before(deadline) {
		p, ok := e.n.JobState(id)
		if !ok {
			e.t.Fatal("unknown transfer")
		}
		switch p.State {
		case "done", "failed", "cancelled":
			return p
		case "asking":
			if !asked || answer != nil {
				if answer == nil {
					e.t.Fatalf("%s %v → %s asked about %+v", req.Kind, req.Sources, req.Target, p.Ask)
				}
				asked = true
				if a := answer(p); a != nil {
					e.n.AnswerJob(id, *a)
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.t.Fatalf("%s %v → %s did not end", req.Kind, req.Sources, req.Target)
	return NetJobProgress{}
}

// mustXfer is xfer that has to end "done".
func (e *liveEnv) mustXfer(req NetJobRequest) NetJobProgress {
	e.t.Helper()
	p := e.xfer(req, nil)
	if p.State != "done" || len(p.Errors) > 0 {
		e.t.Fatalf("%s %v → %s: %s %v", req.Kind, req.Sources, req.Target, p.State, p.Errors)
	}
	return p
}

// vfsJob runs a VfsJobs job (F8 on a server or in an archive, F5 out of an archive, pack, test) and waits for it.
func (e *liveEnv) vfsJob(req VfsRequest) *vfsJob {
	e.t.Helper()
	var job *vfsJob
	e.js.mu.Lock()
	e.js.started = func(j *vfsJob) { job = j }
	e.js.mu.Unlock()
	id, err := e.js.Start(req)
	if err != nil {
		e.t.Fatalf("%s %v: %v", req.Kind, req.Sources, err)
	}
	if id == 0 {
		e.t.Fatalf("%s %v: VfsJobs did not take the job (it went to the local job engine)", req.Kind, req.Sources)
	}
	deadline := time.Now().Add(liveTimeout)
	for e.js.Running() {
		if time.Now().After(deadline) {
			e.t.Fatalf("%s %v did not end", req.Kind, req.Sources)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return job
}

// liveFile is one file of a local test tree.
type liveFile struct {
	rel  string
	data []byte
}

// liveTree is the test tree: text files (FTP sends *.txt in ASCII in the automatic mode), a 1.5 MiB binary file of
// random bytes, a name with spaces and one with accented letters, and an empty file, in nested folders.
func liveTree() []liveFile {
	blob := make([]byte, 1536*1024+7)
	rand.Read(blob)
	return []liveFile{
		{"a.txt", []byte("alpha\nsecond line\n")},
		{"bin/blob.bin", blob},
		{"sub/deep/c.txt", []byte("gamma\n")},
		{"sub/with space.dat", []byte("spaces in the name")},
		{"sub/árvíztűrő.txt", []byte("accents in the name\n")},
		{"sub/empty.dat", nil},
	}
}

// writeTree writes files under dir with the date mt.
func writeTree(t *testing.T, dir string, files []liveFile, mt time.Time) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f.rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, f.data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
}

// checkTree checks that dir holds files with their contents.
func checkTree(t *testing.T, dir string, files []liveFile) {
	t.Helper()
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.rel)))
		if err != nil {
			t.Errorf("%s: %v", f.rel, err)
			continue
		}
		if !bytes.Equal(b, f.data) {
			t.Errorf("%s: %d bytes, want %d (content differs)", f.rel, len(b), len(f.data))
		}
	}
}

// ---- Synchronize folders ------------------------------------------------------------------------------------------

func liveSyncApp(e *liveEnv) *SyncDirs {
	s := &SyncDirs{app: e.f.app, sess: map[int]*syncSession{}}
	old := syncConnect
	syncConnect = func(raw string) (string, error) { c, err := e.n.Connect(raw); return c.Path, err }
	e.t.Cleanup(func() { syncConnect = old })
	return s
}

func liveSyncWait(t *testing.T, s *SyncDirs, id int) {
	t.Helper()
	deadline := time.Now().Add(liveTimeout)
	for time.Now().Before(deadline) {
		ss, _ := s.get(id)
		ss.mu.Lock()
		busy := ss.busy
		ss.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("Synchronize folders did not finish")
}

func liveSyncCompare(t *testing.T, s *SyncDirs, id int, l, r string, opts syncdirs.Options) *SyncView {
	t.Helper()
	if err := s.Compare(id, SyncCompareRequest{Left: l, Right: r, Options: opts}); err != nil {
		t.Fatalf("compare %s with %s: %v", l, r, err)
	}
	liveSyncWait(t, s, id)
	v, err := s.Rows(id, syncdirs.ShowAll())
	if err != nil || !v.Compared {
		t.Fatalf("rows: %v %+v", err, v)
	}
	if len(v.Errors) > 0 {
		t.Fatalf("compare errors: %v", v.Errors)
	}
	return v
}

// liveSyncRun synchronizes every row (deletes permanent) and returns the end report.
func liveSyncRun(t *testing.T, s *SyncDirs, id int) map[string]any {
	t.Helper()
	ss, _ := s.get(id)
	ss.mu.Lock()
	l, r := ss.lside.raw, ss.rside.raw
	ss.mu.Unlock()
	if err := s.Synchronize(id, SyncExecRequest{Filter: syncdirs.AllOps(), Left: l, Right: r}); err != nil {
		t.Fatalf("synchronize: %v", err)
	}
	liveSyncWait(t, s, id)
	ss.mu.Lock()
	done := ss.lastDone
	ss.mu.Unlock()
	if msg, _ := done["error"].(string); msg != "" {
		t.Fatalf("synchronize: %s", msg)
	}
	if errs, _ := done["errors"].([]string); len(errs) > 0 {
		t.Fatalf("synchronize: %v", errs)
	}
	return done
}

func liveCategory(t *testing.T, v *SyncView, p string) syncdirs.Category {
	t.Helper()
	for _, it := range v.Rows {
		if it.Path == p {
			return it.Category
		}
	}
	t.Fatalf("no row %s in the compare", p)
	return 0
}
