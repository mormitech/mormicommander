// Tests for the connection itself and the metadata operations a remote panel
// uses: listing (with link resolution), stat, make folder, delete, rename,
// attributes, links, free space, remote commands, the keep-alive and idle
// timers, disconnect detection, context cancellation and SCP-only servers.
package sftp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/remotefs"
)

func names(es []remotefs.Entry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	sort.Strings(out)
	return out
}

func TestDialAndBasicInfo(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	if !c.HasSFTP() {
		t.Fatal("HasSFTP = false")
	}
	if c.Home() != s.Root {
		t.Errorf("Home = %q, want %q", c.Home(), s.Root)
	}
	if c.StartDir() != s.Root {
		t.Errorf("StartDir = %q", c.StartDir())
	}
	if !strings.HasPrefix(c.ServerVersion(), "SSH-2.0-") {
		t.Errorf("ServerVersion = %q", c.ServerVersion())
	}
	if c.SSH() == nil || c.Config().Host != "127.0.0.1" {
		t.Error("accessors broken")
	}
	if c.LastActivity().IsZero() {
		t.Error("LastActivity is zero")
	}
	if c.Err() != nil {
		t.Errorf("Err on open connection = %v", c.Err())
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal("second Close:", err)
	}
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done not closed")
	}
	if _, err := c.ReadDir("/"); !errors.Is(err, ErrClosed) {
		t.Errorf("ReadDir after Close = %v, want ErrClosed", err)
	}
}

func TestStartDirRelative(t *testing.T) {
	s := newTestServer(t)
	os.Mkdir(filepath.Join(s.Root, "sub"), 0o755)
	cfg := s.baseConfig(t)
	cfg.StartDir = "sub"
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := c.StartDir(); got != filepath.ToSlash(filepath.Join(s.Root, "sub")) {
		t.Errorf("StartDir = %q", got)
	}
}

func TestMetadataOperations(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	root := s.Root
	writeFile(t, filepath.Join(root, "a.txt"), "hello")
	writeFile(t, filepath.Join(root, "dir", "inner.txt"), "x")

	// Mkdir / MkdirAll
	if err := c.Mkdir(root + "/new"); err != nil {
		t.Fatal(err)
	}
	if err := c.Mkdir(root + "/new"); err == nil {
		t.Error("Mkdir on existing folder succeeded")
	}
	if err := c.MkdirAll(root + "/deep/er/est"); err != nil {
		t.Fatal(err)
	}
	if err := c.MkdirAll(root + "/deep/er"); err != nil {
		t.Errorf("MkdirAll on existing: %v", err)
	}

	// Relative paths resolve against home.
	if err := c.Mkdir("rel"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "rel")); err != nil {
		t.Fatal("relative mkdir did not land in home:", err)
	}

	// Symlinks: to a file, to a folder, dangling.
	if runtime.GOOS != "windows" {
		if err := c.Symlink(root+"/dir", root+"/link-dir"); err != nil {
			t.Fatal(err)
		}
		if err := c.Symlink(root+"/nowhere", root+"/link-broken"); err != nil {
			t.Fatal(err)
		}
		// pkg/sftp's server makes relative link targets absolute, so make
		// this one locally to check that relative link text is kept as is.
		if err := os.Symlink("a.txt", filepath.Join(root, "link-file")); err != nil {
			t.Fatal(err)
		}
	}

	list, err := c.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.txt", "deep", "dir", "link-broken", "link-dir", "link-file", "new", "rel"}
	if got := names(list); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ReadDir = %v, want %v", got, want)
	}
	byName := map[string]remotefs.Entry{}
	for _, e := range list {
		byName[e.Name] = e
	}
	tests := []struct {
		name                 string
		isDir, isLink, broke bool
		size                 int64
		target               string
	}{
		{"a.txt", false, false, false, 5, ""},
		{"dir", true, false, false, -1, ""},
		{"link-file", false, true, false, 5, "a.txt"},
		{"link-dir", true, true, false, -1, root + "/dir"},
		{"link-broken", false, true, true, -1, root + "/nowhere"},
	}
	for _, tc := range tests {
		e := byName[tc.name]
		if e.IsDir != tc.isDir || e.IsLink != tc.isLink || e.LinkBroken != tc.broke {
			t.Errorf("%s: IsDir=%v IsLink=%v Broken=%v", tc.name, e.IsDir, e.IsLink, e.LinkBroken)
		}
		if tc.size >= 0 && e.Size != tc.size {
			t.Errorf("%s: Size=%d want %d", tc.name, e.Size, tc.size)
		}
		if e.LinkTarget != tc.target {
			t.Errorf("%s: LinkTarget=%q want %q", tc.name, e.LinkTarget, tc.target)
		}
		if e.Path != root+"/"+tc.name {
			t.Errorf("%s: Path=%q", tc.name, e.Path)
		}
	}

	// Stat follows links, Lstat does not.
	st, err := c.Stat(root + "/link-dir")
	if err != nil || !st.IsDir || st.IsLink {
		t.Errorf("Stat(link-dir) = %+v, %v", st, err)
	}
	lst, err := c.Lstat(root + "/link-dir")
	if err != nil || !lst.IsLink || !lst.IsDir || lst.LinkTarget != root+"/dir" {
		t.Errorf("Lstat(link-dir) = %+v, %v", lst, err)
	}
	if tgt, err := c.ReadLink(root + "/link-file"); err != nil || tgt != "a.txt" {
		t.Errorf("ReadLink = %q, %v", tgt, err)
	}
	if _, err := c.Stat(root + "/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(missing) err = %v, want ErrNotExist", err)
	}
	if ok, err := c.Exists(root + "/a.txt"); !ok || err != nil {
		t.Errorf("Exists(a.txt) = %v, %v", ok, err)
	}
	if ok, err := c.Exists(root + "/missing"); ok || err != nil {
		t.Errorf("Exists(missing) = %v, %v", ok, err)
	}
	if ok, _ := c.Exists(root + "/link-broken"); !ok {
		t.Error("Exists(dangling link) should be true")
	}

	// RealPath
	if rp, err := c.RealPath(root + "/dir/../a.txt"); err != nil || !strings.HasSuffix(rp, "/a.txt") {
		t.Errorf("RealPath = %q, %v", rp, err)
	}

	// Chmod / Chtimes / Truncate / Chown (to our own ids, which must work)
	if err := c.Chmod(root+"/a.txt", 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(root, "a.txt")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode after Chmod = %v", fi.Mode().Perm())
	}
	mt := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := c.Chtimes(root+"/a.txt", mt, mt); err != nil {
		t.Fatal(err)
	}
	if e, _ := c.Stat(root + "/a.txt"); !e.ModTime.Equal(mt) {
		t.Errorf("ModTime after Chtimes = %v", e.ModTime)
	}
	if err := c.Truncate(root+"/a.txt", 2); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(root, "a.txt")); got != "he" {
		t.Errorf("after Truncate: %q", got)
	}
	if runtime.GOOS != "windows" {
		if err := c.Chown(root+"/a.txt", os.Getuid(), os.Getgid()); err != nil {
			t.Errorf("Chown to self: %v", err)
		}
		e, _ := c.Stat(root + "/a.txt")
		if int(e.UID) != os.Getuid() {
			t.Errorf("UID = %d, want %d", e.UID, os.Getuid())
		}
	}

	// Rename fails on existing target; RenameOverwrite replaces it.
	writeFile(t, filepath.Join(root, "b.txt"), "bee")
	if err := c.Rename(root+"/b.txt", root+"/a.txt"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Rename onto existing file = %v, want ErrExist", err)
	}
	if err := c.RenameOverwrite(root+"/b.txt", root+"/a.txt"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(root, "a.txt")); got != "bee" {
		t.Errorf("after RenameOverwrite: %q", got)
	}
	if err := c.Rename(root+"/a.txt", root+"/new/moved.txt"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(root, "new", "moved.txt")); got != "bee" {
		t.Errorf("after move: %q", got)
	}

	// Hard link
	if c.HasExtension("hardlink@openssh.com") {
		if err := c.Link(root+"/new/moved.txt", root+"/hard.txt"); err != nil {
			t.Errorf("Link: %v", err)
		}
	}

	// Remove: file, link (not its target), empty folder, non-empty fails.
	if err := c.Remove(root + "/link-dir"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "dir", "inner.txt")); err != nil {
		t.Fatal("removing a link deleted the target's content")
	}
	if err := c.Remove(root + "/rel"); err != nil {
		t.Fatal(err)
	}
	if err := c.Remove(root + "/dir"); err == nil {
		t.Error("Remove on non-empty folder succeeded")
	}
	if err := c.Remove(root + "/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove(missing) = %v", err)
	}

	// RemoveAll with progress callback
	var removed []string
	if err := c.RemoveAll(t.Context(), root+"/deep", func(p string) { removed = append(removed, p) }); err != nil {
		t.Fatal(err)
	}
	if len(removed) != 3 || removed[len(removed)-1] != root+"/deep" {
		t.Errorf("RemoveAll visited %v", removed)
	}
	if _, err := os.Stat(filepath.Join(root, "deep")); !os.IsNotExist(err) {
		t.Error("deep still exists")
	}
	if err := c.RemoveAll(t.Context(), root+"/does-not-exist", nil); err != nil {
		t.Errorf("RemoveAll(missing) = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.RemoveAll(ctx, root+"/dir", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("RemoveAll(cancelled) = %v", err)
	}
}

func TestChmodAllAndTreeSize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	s := newTestServer(t)
	c := s.dial(t)
	writeFile(t, filepath.Join(s.Root, "t", "a"), "12345")
	writeFile(t, filepath.Join(s.Root, "t", "sub", "b"), "123")
	os.Symlink("a", filepath.Join(s.Root, "t", "ln"))
	if err := c.ChmodAll(t.Context(), s.Root+"/t", 0o600, 0o750); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]fs.FileMode{"t": 0o750, "t/sub": 0o750, "t/a": 0o600, "t/sub/b": 0o600} {
		fi, err := os.Stat(filepath.Join(s.Root, p))
		if err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: mode %v, want %v (%v)", p, fi.Mode().Perm(), want, err)
		}
	}
	files, dirs, size, err := c.TreeSize(t.Context(), s.Root+"/t")
	if err != nil || files != 3 || dirs != 2 || size != 8 {
		t.Errorf("TreeSize = %d files, %d dirs, %d bytes, %v", files, dirs, size, err)
	}
}

func TestWalkSkipDir(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	writeFile(t, filepath.Join(s.Root, "w", "keep", "k"), "")
	writeFile(t, filepath.Join(s.Root, "w", "skip", "s"), "")
	var seen []string
	err := c.Walk(t.Context(), s.Root+"/w", func(e remotefs.Entry, err error) error {
		if err != nil {
			return err
		}
		seen = append(seen, strings.TrimPrefix(e.Path, s.Root+"/"))
		if e.Name == "skip" {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(seen)
	if got := strings.Join(seen, ","); got != "w,w/keep,w/keep/k,w/skip" {
		t.Errorf("walk saw %s", got)
	}
	// Walking a missing root reports the error through fn.
	err = c.Walk(t.Context(), s.Root+"/nope", func(e remotefs.Entry, err error) error { return err })
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("walk missing = %v", err)
	}
	// An error from fn stops the walk.
	stop := errors.New("stop")
	err = c.Walk(t.Context(), s.Root+"/w", func(e remotefs.Entry, err error) error { return stop })
	if !errors.Is(err, stop) {
		t.Errorf("walk stop = %v", err)
	}
}

func TestSpace(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	if !c.HasExtension("statvfs@openssh.com") {
		if _, err := c.Space(s.Root); err == nil {
			t.Error("Space without extension should fail")
		}
		return
	}
	sp, err := c.Space(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if sp.Total == 0 || sp.Free > sp.Total || sp.Available > sp.Total {
		t.Errorf("implausible space %+v", sp)
	}
}

func TestOpenCreate(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	w, err := c.Create(s.Root + "/created.txt")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(w, "content")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := c.Open(s.Root + "/created.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if seeker, ok := r.(io.Seeker); !ok {
		t.Error("Open result is not seekable")
	} else {
		seeker.Seek(3, io.SeekStart)
	}
	b, _ := io.ReadAll(r)
	if string(b) != "tent" {
		t.Errorf("read %q", b)
	}
	if _, err := c.Open(s.Root + "/none"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open(missing) = %v", err)
	}
}

func TestRunCommand(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	tests := []struct {
		cmd    string
		code   int
		stdout string
		stderr string
	}{
		{"echo hello world", 0, "hello world\n", ""},
		{"exit 3", 3, "", "bye\n"},
		{"nosuch", 127, "", "nosuch: command not found\n"},
	}
	for _, tc := range tests {
		r, err := c.RunCommand(t.Context(), tc.cmd)
		if err != nil {
			t.Fatalf("%s: %v", tc.cmd, err)
		}
		if r.ExitCode != tc.code || string(r.Stdout) != tc.stdout || string(r.Stderr) != tc.stderr {
			t.Errorf("%s: got %+v", tc.cmd, r)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.RunCommand(ctx, "sleep 5"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("cancelled command err = %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("cancellation took too long")
	}
}

func TestKeepAlive(t *testing.T) {
	s := newTestServer(t)
	cfg := s.baseConfig(t)
	cfg.KeepAlive = 20 * time.Millisecond
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	deadline := time.Now().Add(3 * time.Second)
	for s.keepalives.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := s.keepalives.Load(); n < 3 {
		t.Fatalf("only %d keep-alives sent", n)
	}
	if c.Err() != nil {
		t.Fatalf("connection reported %v", c.Err())
	}
}

func TestKeepAliveFailure(t *testing.T) {
	old := keepAliveMinWait
	keepAliveMinWait = 100 * time.Millisecond
	defer func() { keepAliveMinWait = old }()
	s := newTestServer(t)
	s.ignoreAlive.Store(true)
	cfg := s.baseConfig(t)
	cfg.KeepAlive = 20 * time.Millisecond
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("hung server not detected")
	}
	if !errors.Is(c.Err(), ErrKeepAliveFailed) {
		t.Errorf("Err = %v, want ErrKeepAliveFailed", c.Err())
	}
}

func TestServerDisconnectDetected(t *testing.T) {
	s := newTestServer(t)
	c := s.dial(t)
	s.DropConnections()
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("disconnect not noticed")
	}
	if _, err := c.Stat(s.Root); err == nil {
		t.Error("Stat after disconnect succeeded")
	}
}

func TestIdleTimeout(t *testing.T) {
	s := newTestServer(t)
	cfg := s.baseConfig(t)
	cfg.IdleTimeout = 150 * time.Millisecond
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// Activity keeps it alive.
	for i := 0; i < 5; i++ {
		time.Sleep(50 * time.Millisecond)
		if _, err := c.Stat(s.Root); err != nil {
			t.Fatalf("connection dropped while active: %v", err)
		}
	}
	select {
	case <-c.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("idle timeout did not fire")
	}
	if !errors.Is(c.Err(), ErrIdleTimeout) {
		t.Errorf("Err = %v, want ErrIdleTimeout", c.Err())
	}
	if _, err := c.ReadDir(s.Root); !errors.Is(err, ErrClosed) {
		t.Errorf("ReadDir after idle = %v", err)
	}
}

func TestNoSFTPSubsystem(t *testing.T) {
	s := newTestServer(t, withNoSFTP())
	c := s.dial(t)
	if c.HasSFTP() {
		t.Fatal("HasSFTP = true")
	}
	if _, err := c.ReadDir("/"); !errors.Is(err, ErrNoSFTP) {
		t.Errorf("ReadDir = %v, want ErrNoSFTP", err)
	}
	if c.HasExtension("statvfs@openssh.com") {
		t.Error("HasExtension without SFTP")
	}
	// SCP still works.
	local := filepath.Join(t.TempDir(), "f.txt")
	writeFile(t, local, "via scp")
	if _, err := c.SCPUpload(t.Context(), local, s.Root+"/f.txt", SCPOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(s.Root, "f.txt")); got != "via scp" {
		t.Errorf("uploaded %q", got)
	}
}

func TestDialCancelAndTimeout(t *testing.T) {
	// A TCP server that accepts but never speaks SSH.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	cfg := Config{Host: "127.0.0.1", Port: port, User: "u", Password: "p", InsecureIgnoreHostKey: true, Timeout: 10 * time.Second}

	ctx, cancel := context.WithCancel(t.Context())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	start := time.Now()
	if _, err := Dial(ctx, cfg); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled Dial err = %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("cancel took too long")
	}

	cfg.Timeout = 200 * time.Millisecond
	start = time.Now()
	if _, err := Dial(t.Context(), cfg); err == nil {
		t.Error("Dial against silent server succeeded")
	}
	if time.Since(start) > 3*time.Second {
		t.Error("timeout took too long")
	}
}

func TestDialValidation(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want error
	}{
		{"no host", Config{User: "u", Password: "p"}, nil},
		{"no user", Config{Host: "h", Password: "p"}, nil},
		{"bad port", Config{Host: "h", User: "u", Port: 70000, Password: "p"}, nil},
		{"no auth", Config{Host: "127.0.0.1", Port: 1, User: "u"}, ErrNoAuthMethod},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Dial(t.Context(), tc.cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDialConnectionRefused(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	_, err := Dial(t.Context(), Config{Host: "127.0.0.1", Port: port, User: "u", Password: "p", Timeout: time.Second})
	if err == nil {
		t.Fatal("dial to closed port succeeded")
	}
}

func TestCustomDialer(t *testing.T) {
	s := newTestServer(t)
	cfg := s.baseConfig(t)
	used := false
	cfg.Dial = func(network, addr string) (net.Conn, error) {
		used = true
		return net.Dial(network, addr)
	}
	c, err := Dial(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if !used {
		t.Error("custom dialer not used")
	}
}
