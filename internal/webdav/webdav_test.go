// Tests for the WebDAV client against golang.org/x/net/webdav's server,
// running in-process on 127.0.0.1 with an in-memory file system behind basic
// authentication and a sub-path prefix (like Nextcloud's /remote.php/dav/).
package webdav

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	xwebdav "golang.org/x/net/webdav"

	"mormicommander/internal/remotefs"
)

func newServer(t *testing.T) (*Client, *httptest.Server) {
	t.Helper()
	h := &xwebdav.Handler{Prefix: "/dav/files", FileSystem: xwebdav.NewMemFS(), LockSystem: xwebdav.NewMemLS()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "bob" || p != "pw" {
			w.Header().Set("WWW-Authenticate", `Basic realm="t"`)
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	c, err := New(Config{URL: srv.URL + "/dav/files", User: "bob", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c, srv
}

func put(t *testing.T, c *Client, p, content string) {
	t.Helper()
	w, err := c.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(w, content)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, c *Client, p string) string {
	t.Helper()
	r, err := c.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestNew(t *testing.T) {
	tests := []struct {
		url     string
		base    string
		user    string
		wantErr bool
	}{
		{"https://h/dav", "https://h/dav/", "", false},
		{"http://u:p@h:8080/x/", "http://h:8080/x/", "u", false},
		{"davs://h/a%20b", "https://h/a%20b/", "", false},
		{"webdav://h", "http://h/", "", false},
		{"ftp://h", "", "", true},
		{"http:///nohost", "", "", true},
		{"://", "", "", true},
	}
	for _, tc := range tests {
		c, err := New(Config{URL: tc.url})
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v", tc.url, err)
			continue
		}
		if err == nil && (c.URL() != tc.base || c.cfg.User != tc.user) {
			t.Errorf("%s: base %q user %q", tc.url, c.URL(), c.cfg.User)
		}
	}
}

func TestAuth(t *testing.T) {
	_, srv := newServer(t)
	bad, _ := New(Config{URL: srv.URL + "/dav/files", User: "bob", Password: "wrong"})
	err := bad.Ping()
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("bad password err = %v", err)
	}
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 401 || !strings.Contains(se.Error(), "401") {
		t.Errorf("status error %v", err)
	}
	good, _ := New(Config{URL: srv.URL + "/dav/files", User: "bob", Password: "pw"})
	if err := good.Ping(); err != nil {
		t.Fatal(err)
	}
}

func TestOperations(t *testing.T) {
	c, _ := newServer(t)
	if err := c.Mkdir("/docs"); err != nil {
		t.Fatal(err)
	}
	if err := c.Mkdir("/docs"); err == nil {
		t.Error("Mkdir on existing succeeded")
	}
	if err := c.MkdirAll("/a/b/c"); err != nil {
		t.Fatal(err)
	}
	if err := c.MkdirAll("/a/b"); err != nil {
		t.Fatal(err)
	}
	put(t, c, "/docs/hello world.txt", "hello")
	put(t, c, "/docs/ünïcode.txt", "u")
	if err := c.MkdirAll("/docs/hello world.txt"); err == nil {
		t.Error("MkdirAll over a file succeeded")
	}

	list, err := c.ReadDir("/docs")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "hello world.txt,ünïcode.txt" {
		t.Fatalf("ReadDir = %v", names)
	}
	root, err := c.ReadDir("/")
	if err != nil || len(root) != 2 {
		t.Fatalf("root = %v, %v", root, err)
	}

	st, err := c.Stat("/docs/hello world.txt")
	if err != nil || st.Size != 5 || st.IsDir || st.Path != "/docs/hello world.txt" || st.ModTime.IsZero() {
		t.Fatalf("Stat = %+v, %v", st, err)
	}
	if d, err := c.Stat("docs"); err != nil || !d.IsDir || d.Name != "docs" {
		t.Errorf("Stat dir = %+v, %v", d, err)
	}
	if r, err := c.Stat("/"); err != nil || !r.IsDir {
		t.Errorf("Stat root = %+v, %v", r, err)
	}
	if _, err := c.Stat("/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat missing = %v", err)
	}
	if got := get(t, c, "/docs/hello world.txt"); got != "hello" {
		t.Errorf("Open = %q", got)
	}
	if _, err := c.Open("/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open missing = %v", err)
	}

	// MOVE without and with overwrite.
	put(t, c, "/docs/other.txt", "other")
	if err := c.Rename("/docs/other.txt", "/docs/hello world.txt"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Rename onto existing = %v", err)
	}
	if err := c.RenameOverwrite("/docs/other.txt", "/docs/hello world.txt"); err != nil {
		t.Fatal(err)
	}
	if got := get(t, c, "/docs/hello world.txt"); got != "other" {
		t.Errorf("after overwrite = %q", got)
	}
	if err := c.Rename("/docs", "/papers"); err != nil {
		t.Fatal(err)
	}

	// Server-side COPY of a folder.
	if err := c.Copy("/papers", "/papers2", false); err != nil {
		t.Fatal(err)
	}
	if got := get(t, c, "/papers2/hello world.txt"); got != "other" {
		t.Errorf("copied = %q", got)
	}

	// Remove: file ok, non-empty folder refused, RemoveAll recursive.
	if err := c.Remove("/papers2/ünïcode.txt"); err != nil {
		t.Fatal(err)
	}
	if err := c.Remove("/papers2"); err == nil {
		t.Error("Remove non-empty folder succeeded")
	}
	if err := c.Remove("/a/b/c"); err != nil {
		t.Errorf("Remove empty folder: %v", err)
	}
	if err := c.RemoveAll("/papers2"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Stat("/papers2"); !errors.Is(err, fs.ErrNotExist) {
		t.Error("RemoveAll left the folder")
	}
	if err := c.RemoveAll("/never-existed"); err != nil {
		t.Errorf("RemoveAll missing = %v", err)
	}
	if err := c.Remove("/never-existed"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove missing = %v", err)
	}
}

func TestTransfers(t *testing.T) {
	c, _ := newServer(t)
	local := t.TempDir()
	data := bytes.Repeat([]byte("0123456789abcdef"), 40000)
	src := filepath.Join(local, "src.bin")
	os.WriteFile(src, data, 0o644)

	var last remotefs.Progress
	n, err := c.Upload(t.Context(), src, "/up.bin", remotefs.TransferOptions{Progress: func(p remotefs.Progress) { last = p }})
	if err != nil || n != int64(len(data)) || last.Done != int64(len(data)) {
		t.Fatalf("Upload = %d, %v, %+v", n, err, last)
	}
	dst := filepath.Join(local, "dst.bin")
	n, err = c.Download(t.Context(), "/up.bin", dst, remotefs.TransferOptions{PreserveTimes: true})
	if err != nil || n != int64(len(data)) {
		t.Fatalf("Download = %d, %v", n, err)
	}
	if b, _ := os.ReadFile(dst); !bytes.Equal(b, data) {
		t.Fatal("download differs")
	}

	// Resume download with Range.
	os.WriteFile(dst, data[:1000], 0o644)
	n, err = c.Download(t.Context(), "/up.bin", dst, remotefs.TransferOptions{Resume: true})
	if err != nil || n != int64(len(data)-1000) {
		t.Fatalf("resume = %d, %v", n, err)
	}
	if b, _ := os.ReadFile(dst); !bytes.Equal(b, data) {
		t.Fatal("resumed download differs")
	}
	if n, err := c.Download(t.Context(), "/up.bin", dst, remotefs.TransferOptions{Resume: true}); n != 0 || err != nil {
		t.Errorf("complete resume = %d %v", n, err)
	}

	// Guards.
	if _, err := c.Download(t.Context(), "/up.bin", dst, remotefs.TransferOptions{NoOverwrite: true}); !errors.Is(err, remotefs.ErrExists) {
		t.Errorf("download NoOverwrite = %v", err)
	}
	if _, err := c.Upload(t.Context(), src, "/up.bin", remotefs.TransferOptions{NoOverwrite: true}); !errors.Is(err, remotefs.ErrExists) {
		t.Errorf("upload NoOverwrite = %v", err)
	}
	if n, err := c.Upload(t.Context(), src, "/up.bin", remotefs.TransferOptions{Resume: true}); n != 0 || err != nil {
		t.Errorf("upload resume of complete file = %d, %v", n, err)
	}
	put(t, c, "/partial.bin", "0123")
	if _, err := c.Upload(t.Context(), src, "/partial.bin", remotefs.TransferOptions{Resume: true}); !errors.Is(err, remotefs.ErrResumeUnsupported) {
		t.Errorf("upload resume of partial = %v", err)
	}
	c.Mkdir("/dir")
	if _, err := c.Download(t.Context(), "/dir", dst, remotefs.TransferOptions{}); err == nil {
		t.Error("download of folder succeeded")
	}
	if _, err := c.Upload(t.Context(), src, "/dir", remotefs.TransferOptions{}); err == nil {
		t.Error("upload onto folder succeeded")
	}
	if _, err := c.Download(t.Context(), "/missing", dst, remotefs.TransferOptions{}); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("download missing = %v", err)
	}

	// Cancellation.
	ctx, cancel := context.WithCancel(t.Context())
	_, err = c.Download(ctx, "/up.bin", filepath.Join(local, "c.bin"), remotefs.TransferOptions{
		Progress: func(p remotefs.Progress) {
			if p.Done > 1000 {
				cancel()
			}
		}})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled download = %v", err)
	}
	ctx, cancel = context.WithCancel(t.Context())
	_, err = c.Upload(ctx, src, "/c.bin", remotefs.TransferOptions{
		Progress: func(p remotefs.Progress) {
			if p.Done > 1000 {
				cancel()
			}
		}})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled upload = %v", err)
	}
}

func TestStatusErrorIs(t *testing.T) {
	tests := []struct {
		code   int
		target error
		want   bool
	}{
		{404, fs.ErrNotExist, true},
		{410, fs.ErrNotExist, true},
		{403, fs.ErrPermission, true},
		{412, fs.ErrExist, true},
		{500, fs.ErrNotExist, false},
		{404, fs.ErrExist, false},
	}
	for _, tc := range tests {
		e := &StatusError{Code: tc.code}
		if errors.Is(e, tc.target) != tc.want {
			t.Errorf("%d is %v: want %v", tc.code, tc.target, tc.want)
		}
	}
}
