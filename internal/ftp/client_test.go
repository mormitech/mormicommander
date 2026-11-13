// This file tests the FTP client end to end against the in-test server from
// server_test.go: logging in, browsing, the single-item commands, transfers
// with resume and the different transfer modes, active and passive data
// connections, FTPS in both flavours, encodings, the time offset, keepalive,
// cancellation and the whole-tree helpers. Every test runs on 127.0.0.1 in
// temporary folders only.
package ftp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func dialT(t *testing.T, cfg Config) *Client {
	t.Helper()
	c, err := Dial(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func names(es []Entry) []string {
	var n []string
	for _, e := range es {
		n = append(n, e.Name)
	}
	sort.Strings(n)
	return n
}

func find(es []Entry, name string) (Entry, bool) {
	for _, e := range es {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

func TestLogin(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		srvUser string
		srvPass string
		user    string
		pass    string
		wantErr bool
	}{
		{"anonymous by default", "", "", "", "", false},
		{"explicit anonymous", "", "", "anonymous", "me@example.com", false},
		{"ftp alias", "", "", "ftp", "", false},
		{"user and password", "peter", "secret", "peter", "secret", false},
		{"wrong password", "peter", "secret", "peter", "nope", true},
		{"anonymous refused", "peter", "secret", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, func(s *testServer) { s.user, s.pass = tt.srvUser, tt.srvPass })
			cfg := srv.cfg()
			cfg.User, cfg.Password = tt.user, tt.pass
			c, err := Dial(ctx, cfg)
			if tt.wantErr {
				if err == nil {
					c.Close()
					t.Fatal("expected login failure")
				}
				if !errors.Is(err, ErrLoginFailed) || ReplyCode(err) != 530 {
					t.Fatalf("error = %v, want ErrLoginFailed with 530", err)
				}
				if strings.Contains(err.Error(), tt.pass) && tt.pass != "" {
					t.Fatalf("password leaked into error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if c.System() != "UNIX Type: L8" || !strings.Contains(c.Welcome(), "ready") {
				t.Errorf("System=%q Welcome=%q", c.System(), c.Welcome())
			}
			if !c.HasFeature("mlst") || !c.HasFeature("UTF8") {
				t.Errorf("features = %v", c.Features())
			}
		})
	}
}

func TestLogMasksPassword(t *testing.T) {
	srv := newTestServer(t, func(s *testServer) { s.user, s.pass = "u", "topsecret" })
	var mu sync.Mutex
	var log strings.Builder
	cfg := srv.cfg()
	cfg.User, cfg.Password = "u", "topsecret"
	cfg.Logf = func(f string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		log.WriteString(strings.TrimSpace(fmt.Sprintf(f, a...)) + "\n")
	}
	c := dialT(t, cfg)
	c.NoOp(context.Background())
	mu.Lock()
	defer mu.Unlock()
	if strings.Contains(log.String(), "topsecret") || !strings.Contains(log.String(), "PASS ****") {
		t.Fatalf("log does not mask the password:\n%s", log.String())
	}
}

func TestConnectErrors(t *testing.T) {
	if _, err := Dial(context.Background(), Config{}); err == nil {
		t.Error("empty host accepted")
	}
	// A port nobody listens on.
	srv := newTestServer(t)
	cfg := srv.cfg()
	srv.ln.Close()
	srv.wg.Wait()
	if _, err := Dial(context.Background(), cfg); err == nil {
		t.Error("connect to closed port succeeded")
	}
}

func TestNavigation(t *testing.T) {
	srv := newTestServer(t)
	srv.write("pub/sub dir/a.txt", "a")
	srv.write(`odd "quote"/x`, "x")
	c := dialT(t, srv.cfg())
	ctx := context.Background()

	steps := []struct {
		do   func() error
		want string
	}{
		{func() error { return nil }, "/"},
		{func() error { return c.Cd(ctx, "pub") }, "/pub"},
		{func() error { return c.Cd(ctx, "sub dir") }, "/pub/sub dir"},
		{func() error { return c.CdUp(ctx) }, "/pub"},
		{func() error { return c.Cd(ctx, "/") }, "/"},
		{func() error { return c.Cd(ctx, `odd "quote"`) }, `/odd "quote"`},
	}
	for i, s := range steps {
		if err := s.do(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		got, err := c.Pwd(ctx)
		if err != nil || got != s.want {
			t.Fatalf("step %d: Pwd = %q, %v; want %q", i, got, err, s.want)
		}
	}
	if err := c.Cd(ctx, "/missing"); ReplyCode(err) != 550 {
		t.Fatalf("Cd missing: %v", err)
	}
	if err := c.Cd(ctx, "a\r\nDELE x"); !errors.Is(err, ErrBadLine) {
		t.Fatalf("command injection not rejected: %v", err)
	}
}

func TestListFormats(t *testing.T) {
	ctx := context.Background()
	setup := func(s *testServer) {
		s.write("dir/file one.txt", "12345")
		s.write("dir/sub/inner", "")
		os.Symlink("file one.txt", filepath.Join(s.root, "dir", "link"))
	}
	tests := []struct {
		name   string
		opt    func(*testServer)
		cfg    func(*Config)
		format ListFormat
	}{
		{"mlsd", func(*testServer) {}, nil, FormatMLSD},
		{"unix list (no MLSD on server)", func(s *testServer) { s.noMLSD = true }, nil, FormatUnix},
		{"unix list (MLSD disabled by user)", func(*testServer) {}, func(c *Config) { c.DisableMLSD = true }, FormatUnix},
		{"dos list", func(s *testServer) { s.noMLSD = true; s.listStyle = "dos" }, nil, FormatDOS},
		{"custom list command", func(s *testServer) { s.noMLSD = true }, func(c *Config) { c.ListCommand = "LIST -la" }, FormatUnix},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, tt.opt)
			setup(srv)
			cfg := srv.cfg()
			if tt.cfg != nil {
				tt.cfg(&cfg)
			}
			c := dialT(t, cfg)
			es, err := c.List(ctx, "dir")
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"file one.txt", "link", "sub"}
			if got := names(es); strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("names = %q, want %q", got, want)
			}
			f, _ := find(es, "file one.txt")
			if f.Format != tt.format || f.Type != TypeFile || f.Size != 5 {
				t.Errorf("file = %+v", f)
			}
			if d, _ := find(es, "sub"); !d.IsDir() || d.Mode&fs.ModeDir == 0 {
				t.Errorf("sub = %+v", d)
			}
			if tt.format != FormatDOS {
				if l, _ := find(es, "link"); l.Type != TypeLink {
					t.Errorf("link = %+v", l)
				}
			}
			if f.ModTime.IsZero() || time.Since(f.ModTime) > time.Hour || time.Until(f.ModTime) > time.Hour {
				t.Errorf("modtime = %v", f.ModTime)
			}
			// LIST must not have changed the current folder.
			if p, _ := c.Pwd(ctx); p != "/" {
				t.Errorf("Pwd after List = %q", p)
			}
			if strings.HasPrefix(cfg.ListCommand, "LIST -la") && srv.sent("LIST -la") == 0 {
				t.Errorf("custom list command not used: %v", srv.commands())
			}
		})
	}
}

func TestListCannedVMS(t *testing.T) {
	srv := newTestServer(t, func(s *testServer) {
		s.noMLSD = true
		s.canned = map[string]string{"/": "Directory DISK$USER:[SMITH]\r\n\r\n" +
			"LOGIN.COM;3          2/3      12-JAN-2020 10:15:22  [GROUP,SMITH]  (RWED,RWED,RE,)\r\n" +
			"A_VERY_LONG_FILE_NAME_INDEED.TXT;1\r\n" +
			"                     5/6       3-MAR-2021 11:00:00  [GROUP,SMITH]  (RWED,RWED,,)\r\n" +
			"PROJECTS.DIR;1       1/3       2-FEB-2021 08:00     [GROUP,SMITH]  (RWE,RWE,RE,E)\r\n" +
			"\r\nTotal of 3 files, 8/12 blocks.\r\n"}
	})
	c := dialT(t, srv.cfg())
	es, err := c.List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(names(es), "|"); got != "A_VERY_LONG_FILE_NAME_INDEED.TXT|LOGIN.COM|PROJECTS" {
		t.Fatalf("names = %s", got)
	}
	if d, _ := find(es, "PROJECTS"); !d.IsDir() {
		t.Errorf("PROJECTS = %+v", d)
	}
	if f, _ := find(es, "LOGIN.COM"); f.Size != 1024 || f.Owner != "SMITH" || f.ModTime.Year() != 2020 {
		t.Errorf("LOGIN.COM = %+v", f)
	}
}

func TestStatAndNameList(t *testing.T) {
	ctx := context.Background()
	for _, noMLSD := range []bool{false, true} {
		t.Run(map[bool]string{false: "mlst", true: "list fallback"}[noMLSD], func(t *testing.T) {
			srv := newTestServer(t, func(s *testServer) { s.noMLSD = noMLSD })
			srv.write("d/f.bin", "hello")
			srv.write("top.txt", "x")
			c := dialT(t, srv.cfg())
			e, err := c.Stat(ctx, "/d/f.bin")
			if err != nil || e.Name != "f.bin" || e.Size != 5 || e.Type != TypeFile {
				t.Fatalf("Stat file = %+v, %v", e, err)
			}
			e, err = c.Stat(ctx, "/top.txt")
			if err != nil || e.Name != "top.txt" || e.Size != 1 {
				t.Fatalf("Stat root-level file = %+v, %v", e, err)
			}
			e, err = c.Stat(ctx, "d")
			if err != nil || !e.IsDir() {
				t.Fatalf("Stat dir = %+v, %v", e, err)
			}
			if e, err := c.Stat(ctx, "/"); err != nil || !e.IsDir() {
				t.Fatalf("Stat root = %+v, %v", e, err)
			}
			if _, err := c.Stat(ctx, "/d/missing"); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("Stat missing: %v", err)
			}
			nl, err := c.NameList(ctx, "d")
			if err != nil || len(nl) != 1 || nl[0] != "f.bin" {
				t.Fatalf("NameList = %q, %v", nl, err)
			}
		})
	}
}

func TestFileOperations(t *testing.T) {
	srv := newTestServer(t)
	srv.write("a.txt", "abc")
	c := dialT(t, srv.cfg())
	ctx := context.Background()

	if err := c.Mkdir(ctx, "/new"); err != nil {
		t.Fatal(err)
	}
	if err := c.Rename(ctx, "/a.txt", "/new/b.txt"); err != nil {
		t.Fatal(err)
	}
	if !srv.exists("new/b.txt") || srv.exists("a.txt") {
		t.Fatal("rename did not move the file")
	}
	if err := c.Rename(ctx, "/missing", "/x"); ReplyCode(err) != 550 {
		t.Fatalf("rename missing: %v", err)
	}
	if err := c.Chmod(ctx, "/new/b.txt", 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(filepath.Join(srv.root, "new", "b.txt"))
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v", fi.Mode())
		}
	}
	if srv.sent("SITE CHMOD 600 /new/b.txt") != 1 {
		t.Errorf("SITE CHMOD not sent as expected: %v", srv.commands())
	}
	if n, err := c.Size(ctx, "/new/b.txt"); err != nil || n != 3 {
		t.Fatalf("Size = %d, %v", n, err)
	}
	want := time.Date(2021, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := c.SetModTime(ctx, "/new/b.txt", want); err != nil {
		t.Fatal(err)
	}
	if got, err := c.ModTime(ctx, "/new/b.txt"); err != nil || !got.Equal(want) {
		t.Fatalf("ModTime = %v, %v", got, err)
	}
	if err := c.ServerCopy(ctx, "/new/b.txt", "/new/c.txt"); err != nil || srv.read("new/c.txt") != "abc" {
		t.Fatalf("ServerCopy: %v", err)
	}
	if err := c.Rmdir(ctx, "/new"); err == nil {
		t.Fatal("removed a non-empty folder")
	}
	if err := c.Delete(ctx, "/new/b.txt"); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, "/new/c.txt"); err != nil {
		t.Fatal(err)
	}
	if err := c.Rmdir(ctx, "/new"); err != nil {
		t.Fatal(err)
	}
	if srv.exists("new") {
		t.Fatal("folder still exists")
	}
	r, err := c.Quote(ctx, "SITE NONSENSE")
	if err != nil || r.Code != 500 {
		t.Fatalf("Quote = %+v, %v", r, err)
	}
}

func TestServerCopyAndModTimeUnsupported(t *testing.T) {
	srv := newTestServer(t, func(s *testServer) { s.noCopy = true; s.noMFMT = true })
	srv.write("a", "1")
	c := dialT(t, srv.cfg())
	ctx := context.Background()
	if err := c.ServerCopy(ctx, "a", "b"); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("ServerCopy: %v", err)
	}
	// Without MFMT the two-argument MDTM is used.
	want := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := c.SetModTime(ctx, "a", want); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.ModTime(ctx, "a"); !got.Equal(want) {
		t.Fatalf("ModTime = %v", got)
	}
}

func TestTransfers(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789abcdef\x00\xff\r\n"), 20000) // ~400 KB, binary-unsafe bytes
	tests := []struct {
		name string
		opt  func(*testServer)
		cfg  func(*Config)
	}{
		{"passive EPSV", nil, nil},
		{"passive PASV fallback", func(s *testServer) { s.noEPSV = true }, nil},
		{"passive EPSV disabled", nil, func(c *Config) { c.DisableEPSV = true }},
		{"PASV with 0.0.0.0", func(s *testServer) { s.noEPSV = true; s.pasvIP = "0.0.0.0" }, nil},
		{"PASV forced control host", func(s *testServer) { s.noEPSV = true; s.pasvIP = "10.255.255.1" }, func(c *Config) { c.PassiveHost = PassiveHostControl }},
		{"active PORT", nil, func(c *Config) { c.DataMode = Active }},
		{"active port range", nil, func(c *Config) { c.DataMode = Active; c.ActivePortMin, c.ActivePortMax = 50100, 50200 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []func(*testServer)
			if tt.opt != nil {
				opts = append(opts, tt.opt)
			}
			srv := newTestServer(t, opts...)
			cfg := srv.cfg()
			if tt.cfg != nil {
				tt.cfg(&cfg)
			}
			c := dialT(t, cfg)
			ctx := context.Background()
			dir := t.TempDir()
			src := filepath.Join(dir, "src.bin")
			os.WriteFile(src, payload, 0o644)

			var last int64
			n, err := c.Upload(ctx, src, "/up.bin", TransferOptions{Progress: func(d, total int64) { last = d }})
			if err != nil || n != int64(len(payload)) || last != int64(len(payload)) {
				t.Fatalf("Upload = %d, %v (progress %d)", n, err, last)
			}
			if srv.read("up.bin") != string(payload) {
				t.Fatal("uploaded content differs")
			}
			dst := filepath.Join(dir, "dst.bin")
			n, err = c.Download(ctx, "/up.bin", dst, TransferOptions{})
			if err != nil || n != int64(len(payload)) {
				t.Fatalf("Download = %d, %v", n, err)
			}
			if got, _ := os.ReadFile(dst); !bytes.Equal(got, payload) {
				t.Fatal("downloaded content differs")
			}
			// The connection is still usable after transfers.
			if err := c.NoOp(ctx); err != nil {
				t.Fatal(err)
			}
			switch {
			case cfg.DataMode == Active && srv.sent("PORT ") == 0:
				t.Error("active mode did not send PORT")
			case cfg.DataMode == Passive && (srv.opts().noEPSV || cfg.DisableEPSV) && srv.sent("PASV") == 0:
				t.Error("PASV not used")
			}
			if cfg.DisableEPSV && srv.sent("EPSV") != 0 {
				t.Error("EPSV sent although disabled")
			}
			if cfg.ActivePortMin > 0 {
				for _, cmd := range srv.commands() {
					if strings.HasPrefix(cmd, "PORT ") {
						f := strings.Split(cmd[5:], ",")
						p := atoiT(f[4])<<8 | atoiT(f[5])
						if p < 50100 || p > 50200 {
							t.Errorf("port %d outside range", p)
						}
					}
				}
			}
		})
	}
}

func (s *testServer) opts() *testServer { return s }

func atoiT(s string) int { n, _ := atoi(s); return n }

func TestEmptyFileTransfer(t *testing.T) {
	srv := newTestServer(t)
	c := dialT(t, srv.cfg())
	ctx := context.Background()
	src := filepath.Join(t.TempDir(), "empty")
	os.WriteFile(src, nil, 0o644)
	if _, err := c.Upload(ctx, src, "empty", TransferOptions{}); err != nil {
		t.Fatal(err)
	}
	if !srv.exists("empty") {
		t.Fatal("empty file not created")
	}
	dst := filepath.Join(t.TempDir(), "back")
	if n, err := c.Download(ctx, "empty", dst, TransferOptions{}); err != nil || n != 0 {
		t.Fatalf("Download = %d, %v", n, err)
	}
}

func TestResume(t *testing.T) {
	full := strings.Repeat("resume-me-", 5000)
	for _, noREST := range []bool{false, true} {
		t.Run(map[bool]string{false: "REST", true: "APPE"}[noREST], func(t *testing.T) {
			srv := newTestServer(t, func(s *testServer) { s.noREST = noREST })
			c := dialT(t, srv.cfg())
			ctx := context.Background()
			dir := t.TempDir()

			// Upload resume: the server has the first 1000 bytes.
			srv.write("big.txt.part", full[:1000])
			src := filepath.Join(dir, "big.bin")
			os.WriteFile(src, []byte(full), 0o644)
			n, err := c.Upload(ctx, src, "big.txt.part", TransferOptions{Resume: true, Mode: TransferBinary})
			if err != nil || n != int64(len(full)-1000) {
				t.Fatalf("resumed upload = %d, %v", n, err)
			}
			if srv.read("big.txt.part") != full {
				t.Fatal("resumed upload produced wrong content")
			}
			wantCmd := "REST 1000"
			if noREST {
				wantCmd = "APPE"
			}
			if srv.sent(wantCmd) == 0 {
				t.Errorf("%s not used: %v", wantCmd, srv.commands())
			}
			// Nothing left to do.
			if n, err := c.Upload(ctx, src, "big.txt.part", TransferOptions{Resume: true, Mode: TransferBinary}); err != nil || n != 0 {
				t.Fatalf("complete upload resumed again: %d, %v", n, err)
			}

			if noREST {
				return // downloads need REST
			}
			// Download resume: the local file has the first 1234 bytes.
			dst := filepath.Join(dir, "dl.bin")
			os.WriteFile(dst, []byte(full[:1234]), 0o644)
			n, err = c.Download(ctx, "big.txt.part", dst, TransferOptions{Resume: true, Mode: TransferBinary})
			if err != nil || n != int64(len(full)-1234) {
				t.Fatalf("resumed download = %d, %v", n, err)
			}
			if got, _ := os.ReadFile(dst); string(got) != full {
				t.Fatal("resumed download produced wrong content")
			}
			if n, err := c.Download(ctx, "big.txt.part", dst, TransferOptions{Resume: true, Mode: TransferBinary}); err != nil || n != 0 {
				t.Fatalf("complete download resumed again: %d, %v", n, err)
			}
			// Without Resume the file is replaced.
			os.WriteFile(dst, []byte("garbage that is longer than nothing"), 0o644)
			if _, err := c.Download(ctx, "big.txt.part", dst, TransferOptions{Mode: TransferBinary}); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(dst); string(got) != full {
				t.Fatal("overwrite produced wrong content")
			}
		})
	}
}

func TestRetrieveOffsetAndEarlyClose(t *testing.T) {
	srv := newTestServer(t)
	big := strings.Repeat("x", 4<<20)
	srv.write("big", big)
	srv.write("small", "0123456789")
	c := dialT(t, srv.cfg())
	ctx := context.Background()

	rc, err := c.Retrieve(ctx, "small", 4, TransferBinary)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	if err := rc.Close(); err != nil || string(b) != "456789" {
		t.Fatalf("Retrieve offset = %q, %v", b, err)
	}
	// Reading only the start of a big file (viewer preview) and closing
	// must leave the connection usable.
	rc, err = c.Retrieve(ctx, "big", 0, TransferBinary)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 100)
	io.ReadFull(rc, buf)
	if err := rc.Close(); err != nil {
		t.Fatalf("early close: %v", err)
	}
	if err := c.NoOp(ctx); err != nil {
		t.Fatalf("connection unusable after early close: %v", err)
	}
	if _, err := c.Retrieve(ctx, "missing", 0, TransferBinary); ReplyCode(err) != 550 {
		t.Fatalf("Retrieve missing: %v", err)
	}
	if _, err := c.Retrieve(ctx, "small", 3, TransferASCII); !errors.Is(err, ErrResumeASCII) {
		t.Fatalf("ASCII resume: %v", err)
	}
	if err := c.NoOp(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTransferModes(t *testing.T) {
	ctx := context.Background()
	unixText := "line1\nline2\n"
	localDownloaded := unixText
	if runtime.GOOS == "windows" {
		localDownloaded = "line1\r\nline2\r\n"
	}
	tests := []struct {
		name        string
		connMode    TransferMode
		fileMode    TransferMode
		file        string
		wantType    string
		wantLocal   string
		uploadLocal string
		wantServer  string
	}{
		{"auto picks ascii for .txt", TransferAuto, TransferDefault, "a.txt", "TYPE A", localDownloaded, "x\ny\n", "x\ny\n"},
		{"auto picks binary for .bin", TransferAuto, TransferDefault, "a.bin", "TYPE I", unixText, "x\r\ny\n", "x\r\ny\n"},
		{"forced binary", TransferBinary, TransferDefault, "a.txt", "TYPE I", unixText, "x\r\ny\n", "x\r\ny\n"},
		{"forced ascii", TransferASCII, TransferDefault, "a.bin", "TYPE A", localDownloaded, "x\r\ny\n", "x\ny\n"},
		{"per-file override", TransferBinary, TransferASCII, "a.bin", "TYPE A", localDownloaded, "x\ny", "x\ny"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			srv.write(tt.file, unixText)
			cfg := srv.cfg()
			cfg.TransferMode = tt.connMode
			c := dialT(t, cfg)
			dst := filepath.Join(t.TempDir(), tt.file)
			if _, err := c.Download(ctx, tt.file, dst, TransferOptions{Mode: tt.fileMode}); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(dst); string(got) != tt.wantLocal {
				t.Errorf("downloaded %q, want %q", got, tt.wantLocal)
			}
			if srv.sent(tt.wantType) == 0 {
				t.Errorf("%s not sent: %v", tt.wantType, srv.commands())
			}
			os.WriteFile(dst, []byte(tt.uploadLocal), 0o644)
			if _, err := c.Upload(ctx, dst, "up-"+tt.file, TransferOptions{Mode: tt.fileMode}); err != nil {
				t.Fatal(err)
			}
			if got := srv.read("up-" + tt.file); got != tt.wantServer {
				t.Errorf("server got %q, want %q", got, tt.wantServer)
			}
		})
	}
}

func TestPreserveTime(t *testing.T) {
	srv := newTestServer(t)
	srv.write("f.txt", "x")
	when := time.Date(2019, 3, 4, 5, 6, 7, 0, time.UTC)
	os.Chtimes(filepath.Join(srv.root, "f.txt"), when, when)
	c := dialT(t, srv.cfg())
	ctx := context.Background()
	dst := filepath.Join(t.TempDir(), "f.txt")
	if _, err := c.Download(ctx, "f.txt", dst, TransferOptions{PreserveTime: true}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(dst); !fi.ModTime().Equal(when) {
		t.Errorf("local time = %v", fi.ModTime())
	}
	when2 := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(dst, when2, when2)
	if _, err := c.Upload(ctx, dst, "g.txt", TransferOptions{PreserveTime: true}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(srv.root, "g.txt")); !fi.ModTime().Equal(when2) {
		t.Errorf("remote time = %v", fi.ModTime())
	}
}

func TestTLS(t *testing.T) {
	serverTLS, clientTLS := testCert(t)
	ctx := context.Background()
	tests := []struct {
		name     string
		implicit bool
		sec      Security
		prot     bool
		active   bool
	}{
		{"explicit", false, SecurityExplicitTLS, true, false},
		{"explicit active", false, SecurityExplicitTLS, true, true},
		{"explicit if available", false, SecurityExplicitTLSIfAvailable, true, false},
		{"implicit", true, SecurityImplicitTLS, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, func(s *testServer) {
				s.tlsConf = serverTLS
				s.implicit = tt.implicit
				s.requireTLS = true
			})
			srv.write("secret.txt", "classified")
			cfg := srv.cfg()
			cfg.Security = tt.sec
			cfg.TLSConfig = clientTLS
			if tt.active {
				cfg.DataMode = Active
			}
			c := dialT(t, cfg)
			if !c.Secure() {
				t.Fatal("connection not secure")
			}
			es, err := c.List(ctx, "")
			if err != nil || len(es) != 1 {
				t.Fatalf("List = %v, %v", es, err)
			}
			dst := filepath.Join(t.TempDir(), "s")
			if _, err := c.Download(ctx, "secret.txt", dst, TransferOptions{}); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(dst); string(got) != "classified" {
				t.Fatalf("got %q", got)
			}
			if _, err := c.Upload(ctx, dst, "back.txt", TransferOptions{}); err != nil {
				t.Fatal(err)
			}
			if srv.read("back.txt") != "classified" {
				t.Fatal("upload over TLS failed")
			}
			if srv.sent("PROT P") != 1 || srv.sent("PBSZ 0") != 1 {
				t.Errorf("PBSZ/PROT not negotiated: %v", srv.commands())
			}
		})
	}

	t.Run("untrusted certificate is rejected", func(t *testing.T) {
		srv := newTestServer(t, func(s *testServer) { s.tlsConf = serverTLS })
		cfg := srv.cfg()
		cfg.Security = SecurityExplicitTLS
		if _, err := Dial(ctx, cfg); err == nil {
			t.Fatal("self-signed certificate accepted without trust")
		}
	})
	t.Run("server without TLS", func(t *testing.T) {
		srv := newTestServer(t)
		cfg := srv.cfg()
		cfg.Security = SecurityExplicitTLS
		if _, err := Dial(ctx, cfg); !errors.Is(err, ErrTLSRefused) {
			t.Fatalf("err = %v, want ErrTLSRefused", err)
		}
		cfg.Security = SecurityExplicitTLSIfAvailable
		c := dialT(t, cfg)
		if c.Secure() {
			t.Fatal("claims to be secure")
		}
	})
}

func TestEncodingLatin1(t *testing.T) {
	// The server speaks Latin-1 on the wire ("café" with é = 0xE9) while
	// storing UTF-8 names on disk.
	srv := newTestServer(t, func(s *testServer) { s.utf8 = false; s.noMLSD = true; s.latin1Names = true })
	srv.write("café.txt", "x")
	cfg := srv.cfg()
	cfg.Encoding = EncodingLatin1
	c := dialT(t, cfg)
	ctx := context.Background()
	es, err := c.List(ctx, "")
	if err != nil || len(es) != 1 || es[0].Name != "café.txt" {
		t.Fatalf("List = %+v, %v", es, err)
	}
	if n, err := c.Size(ctx, "café.txt"); err != nil || n != 1 {
		t.Fatalf("Size via Latin-1 name = %d, %v", n, err)
	}
	if err := c.Mkdir(ctx, "Grüße"); err != nil || !srv.exists("Grüße") {
		t.Fatalf("Mkdir with Latin-1 name: %v", err)
	}
	if srv.sent("OPTS UTF8") != 0 {
		t.Error("OPTS UTF8 sent in Latin-1 mode")
	}
	// Without the Latin-1 setting the same server's names arrive garbled,
	// which is exactly why the setting exists.
	c2 := dialT(t, srv.cfg())
	es, err = c2.List(ctx, "")
	if err != nil || len(es) != 2 {
		t.Fatalf("List without Latin-1 = %+v, %v", es, err)
	}
	if _, ok := find(es, "café.txt"); ok {
		t.Error("Latin-1 name decoded correctly without the setting?")
	}
}

func TestUTF8Negotiation(t *testing.T) {
	srv := newTestServer(t)
	srv.write("名前.txt", "x")
	c := dialT(t, srv.cfg())
	if srv.sent("OPTS UTF8 ON") != 1 {
		t.Errorf("OPTS UTF8 ON not sent: %v", srv.commands())
	}
	es, err := c.List(context.Background(), "")
	if err != nil || len(es) != 1 || es[0].Name != "名前.txt" {
		t.Fatalf("List = %+v, %v", es, err)
	}
}

func TestTimeOffset(t *testing.T) {
	srv := newTestServer(t, func(s *testServer) { s.noMLSD = true })
	srv.write("f", "x")
	when := time.Date(2020, 6, 1, 10, 0, 0, 0, time.UTC)
	os.Chtimes(filepath.Join(srv.root, "f"), when, when)
	cfg := srv.cfg()
	cfg.TimeOffset = 2 * time.Hour
	c := dialT(t, cfg)
	es, err := c.List(context.Background(), "")
	if err != nil || len(es) != 1 {
		t.Fatal(err)
	}
	// The year-style UNIX listing has no time of day: 2020-06-01 00:00 + 2h.
	if want := time.Date(2020, 6, 1, 2, 0, 0, 0, time.UTC); !es[0].ModTime.Equal(want) {
		t.Fatalf("ModTime = %v, want %v", es[0].ModTime, want)
	}
}

func TestKeepalive(t *testing.T) {
	srv := newTestServer(t)
	cfg := srv.cfg()
	cfg.Keepalive = 40 * time.Millisecond
	c := dialT(t, cfg)
	deadline := time.Now().Add(3 * time.Second)
	for srv.sent("NOOP") < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if srv.sent("NOOP") < 2 {
		t.Fatal("keepalive sent no NOOPs")
	}
	c.StopKeepalive()
	time.Sleep(60 * time.Millisecond)
	n := srv.sent("NOOP")
	time.Sleep(200 * time.Millisecond)
	if srv.sent("NOOP") != n {
		t.Fatal("NOOPs continue after StopKeepalive")
	}
	if err := c.NoOp(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCancel(t *testing.T) {
	srv := newTestServer(t)
	srv.write("big", strings.Repeat("z", 8<<20))
	c := dialT(t, srv.cfg())
	ctx, cancel := context.WithCancel(context.Background())
	dst := filepath.Join(t.TempDir(), "big")
	_, err := c.Download(ctx, "big", dst, TransferOptions{Progress: func(done, total int64) {
		if done > 1<<20 {
			cancel()
		}
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if fi, err := os.Stat(dst); err != nil || fi.Size() == 0 {
		t.Fatal("partial file should be kept for resume")
	}
	if _, err := c.List(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("List with cancelled context: %v", err)
	}
}

func TestInitialDirAndJoin(t *testing.T) {
	srv := newTestServer(t)
	srv.write("start/here/f", "1")
	cfg := srv.cfg()
	cfg.InitialDir = "/start/here"
	c := dialT(t, cfg)
	if p, _ := c.Pwd(context.Background()); p != "/start/here" {
		t.Fatalf("Pwd = %q", p)
	}
	for _, tc := range [][]string{{"/", "a", "/a"}, {"/x/", "b", "/x/b"}, {"", "c", "c"}, {"", "", ""}} {
		if got := Join(tc[0], tc[1]); got != tc[2] {
			t.Errorf("Join(%q,%q) = %q, want %q", tc[0], tc[1], got, tc[2])
		}
	}
}

func TestTreeOperations(t *testing.T) {
	srv := newTestServer(t)
	c := dialT(t, srv.cfg())
	ctx := context.Background()

	local := t.TempDir()
	files := map[string]string{
		"a.txt":            "A",
		"sub/b.bin":        "BB",
		"sub/deep/c.txt":   "CCC",
		"empty/.keep":      "",
		"with space/d e f": "D",
	}
	for p, content := range files {
		fp := filepath.Join(local, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(fp), 0o755)
		os.WriteFile(fp, []byte(content), 0o644)
	}
	res, err := c.UploadTree(ctx, local, "/remote/tree", TreeOptions{TransferOptions: TransferOptions{Mode: TransferBinary}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != len(files) || res.Bytes != 7 {
		t.Fatalf("UploadTree result = %+v", res)
	}
	for p, content := range files {
		if got := srv.read("remote/tree/" + p); got != content {
			t.Errorf("%s = %q", p, got)
		}
	}

	var walked []string
	err = c.Walk(ctx, "/remote/tree", func(p string, e Entry, err error) error {
		if err != nil {
			return err
		}
		walked = append(walked, strings.TrimPrefix(p, "/remote/tree/"))
		if e.Name == "deep" {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(walked)
	want := "a.txt|empty|empty/.keep|sub|sub/b.bin|sub/deep|with space|with space/d e f"
	if got := strings.Join(walked, "|"); got != want {
		t.Fatalf("walked %s\nwant   %s", got, want)
	}

	back := t.TempDir()
	skipped := 0
	res, err = c.DownloadTree(ctx, "/remote/tree", back, TreeOptions{OnFile: func(l, r string, size int64) bool {
		if strings.HasSuffix(r, "b.bin") {
			skipped++
			return false
		}
		return true
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != len(files)-1 || skipped != 1 {
		t.Fatalf("DownloadTree result = %+v, skipped %d", res, skipped)
	}
	if b, _ := os.ReadFile(filepath.Join(back, "sub", "deep", "c.txt")); string(b) != "CCC" {
		t.Fatalf("deep file = %q", b)
	}
	if _, err := os.Stat(filepath.Join(back, "sub", "b.bin")); err == nil {
		t.Fatal("skipped file was downloaded")
	}

	if err := c.MkdirAll(ctx, "/remote/tree/sub/deep/new/er"); err != nil {
		t.Fatal(err)
	}
	if err := c.MkdirAll(ctx, "/remote/tree/sub"); err != nil {
		t.Fatal("MkdirAll on existing folder:", err)
	}
	if err := c.MkdirAll(ctx, "/remote/tree/a.txt"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("MkdirAll over a file: %v", err)
	}
	if err := c.RemoveAll(ctx, "/remote"); err != nil {
		t.Fatal(err)
	}
	if srv.exists("remote") {
		t.Fatal("RemoveAll left the folder")
	}
	if err := c.RemoveAll(ctx, "/remote"); err != nil {
		t.Fatalf("RemoveAll missing: %v", err)
	}
}

func TestPool(t *testing.T) {
	srv := newTestServer(t)
	for i := 0; i < 6; i++ {
		srv.write("f"+string(rune('0'+i)), strings.Repeat("p", 1000*(i+1)))
	}
	p := NewPool(srv.cfg(), 3)
	defer p.Close()
	ctx := context.Background()
	dir := t.TempDir()
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := p.Get(ctx)
			if err != nil {
				errs <- err
				return
			}
			defer p.Put(c)
			name := "f" + string(rune('0'+i))
			_, err = c.Download(ctx, name, filepath.Join(dir, name), TransferOptions{})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if logins := srv.sent("PASS"); logins > 3 {
		t.Errorf("%d connections opened, max is 3", logins)
	}
	if p.Idle() == 0 || p.Idle() > 3 {
		t.Errorf("idle = %d", p.Idle())
	}
	if n := p.CloseIdle(0); n == 0 || p.Idle() != 0 {
		t.Errorf("CloseIdle closed %d, idle now %d", n, p.Idle())
	}
	// Waiting for a free slot honours the context.
	one := NewPool(srv.cfg(), 1)
	defer one.Close()
	c, err := one.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := one.Get(tctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Get on full pool: %v", err)
	}
	one.Put(c)
}

func TestCloseIsIdempotent(t *testing.T) {
	srv := newTestServer(t)
	c, err := Dial(context.Background(), srv.cfg())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c.Close()
	if err := c.NoOp(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("NoOp after Close: %v", err)
	}
	if srv.sent("QUIT") != 1 {
		t.Error("QUIT not sent")
	}
}
