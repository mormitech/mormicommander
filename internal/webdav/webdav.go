// Package webdav is a small WebDAV client for mormiCommander. Total Commander
// reaches WebDAV servers (Nextcloud, ownCloud, SharePoint, NAS boxes) only
// through a file system plugin; FEATURES.md lists that as a later built-in
// feature (F-645), so this package provides the engine for it now. It speaks
// just the parts of RFC 4918 a file manager needs: PROPFIND to list and stat,
// GET (with Range, so downloads can resume), PUT, MKCOL, DELETE, and the
// server-side MOVE and COPY methods. The Client implements the same
// remotefs.FS interface as the SFTP client, so the panel and job engine can
// treat a WebDAV share exactly like an SFTP connection. Only the standard
// library is used.
package webdav

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"mormicommander/internal/remotefs"
)

// Config describes a WebDAV share.
type Config struct {
	// URL is the share's base URL, e.g. https://cloud.example.org/remote.php/dav/files/bob/.
	// Remote paths given to the Client are relative to it ("/" is the share root).
	URL      string
	User     string // basic auth user, optional
	Password string // basic auth password, optional
	// Timeout bounds each metadata request. Transfers are bounded by their
	// context instead. Default 30s.
	Timeout time.Duration
	// HTTPClient replaces the default http.Client (custom TLS, proxies).
	HTTPClient *http.Client
}

// Client talks to one WebDAV share. It is safe for concurrent use.
type Client struct {
	base    *url.URL // with trailing slash
	cfg     Config
	http    *http.Client
	timeout time.Duration
}

var _ remotefs.FS = (*Client)(nil)

// StatusError is returned for unexpected HTTP responses.
type StatusError struct {
	Method string
	Path   string
	Code   int
	Status string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("webdav: %s %s: %s", e.Method, e.Path, e.Status)
}

// Is maps HTTP status codes to the standard fs errors, so callers can use
// errors.Is(err, fs.ErrNotExist) and friends like with local files.
func (e *StatusError) Is(target error) bool {
	switch target {
	case fs.ErrNotExist:
		return e.Code == http.StatusNotFound || e.Code == http.StatusGone
	case fs.ErrPermission:
		return e.Code == http.StatusUnauthorized || e.Code == http.StatusForbidden
	case fs.ErrExist:
		return e.Code == http.StatusPreconditionFailed || e.Code == http.StatusMethodNotAllowed
	}
	return false
}

// New creates a client. It does not contact the server; call Ping (or any
// operation) to check the connection and credentials.
func New(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("webdav: bad URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	case "webdav", "dav":
		u.Scheme = "http"
	case "webdavs", "davs":
		u.Scheme = "https"
	default:
		return nil, fmt.Errorf("webdav: unsupported URL scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("webdav: URL has no host")
	}
	if u.User != nil {
		if cfg.User == "" {
			cfg.User = u.User.Username()
		}
		if pw, ok := u.User.Password(); ok && cfg.Password == "" {
			cfg.Password = pw
		}
		u.User = nil
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.RawPath = ""
	c := &Client{base: u, cfg: cfg, http: cfg.HTTPClient, timeout: cfg.Timeout}
	if c.http == nil {
		c.http = &http.Client{}
	}
	if c.timeout <= 0 {
		c.timeout = 30 * time.Second
	}
	return c, nil
}

// URL returns the base URL without credentials.
func (c *Client) URL() string { return c.base.String() }

// Close releases idle connections. The client stays usable.
func (c *Client) Close() error {
	c.http.CloseIdleConnections()
	return nil
}

// clean turns a remote path into a canonical absolute share path.
func clean(p string) string {
	return path.Clean("/" + p)
}

// urlFor builds the request URL for a share path; folders get a trailing
// slash when dir is true (some servers redirect otherwise).
func (c *Client) urlFor(p string, dir bool) string {
	p = clean(p)
	u := *c.base
	u.Path = c.base.Path + strings.TrimPrefix(p, "/")
	if dir && !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	return u.String()
}

func (c *Client) newRequest(ctx context.Context, method, p string, dir bool, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.urlFor(p, dir), body)
	if err != nil {
		return nil, err
	}
	if c.cfg.User != "" || c.cfg.Password != "" {
		req.SetBasicAuth(c.cfg.User, c.cfg.Password)
	}
	return req, nil
}

func (c *Client) do(ctx context.Context, method, p string, dir bool, body io.Reader, hdr map[string]string, ok ...int) (*http.Response, error) {
	req, err := c.newRequest(ctx, method, p, dir, body)
	if err != nil {
		return nil, err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webdav: %s %s: %w", method, p, err)
	}
	for _, code := range ok {
		if resp.StatusCode == code {
			return resp, nil
		}
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	resp.Body.Close()
	return nil, &StatusError{Method: method, Path: clean(p), Code: resp.StatusCode, Status: resp.Status}
}

// quick runs a metadata request with the per-request timeout and discards
// the response body.
func (c *Client) quick(method, p string, dir bool, hdr map[string]string, ok ...int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	resp, err := c.do(ctx, method, p, dir, nil, hdr, ok...)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	return resp.Body.Close()
}

// ---- PROPFIND -----------------------------------------------------------

const propfindBody = `<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:"><D:prop>
<D:resourcetype/><D:getcontentlength/><D:getlastmodified/><D:displayname/>
</D:prop></D:propfind>`

type multistatus struct {
	Responses []davResponse `xml:"DAV: response"`
}

type davResponse struct {
	Href     string        `xml:"DAV: href"`
	Propstat []davPropstat `xml:"DAV: propstat"`
}

type davPropstat struct {
	Status string  `xml:"DAV: status"`
	Prop   davProp `xml:"DAV: prop"`
}

type davProp struct {
	ResourceType struct {
		Collection *struct{} `xml:"DAV: collection"`
	} `xml:"DAV: resourcetype"`
	ContentLength string `xml:"DAV: getcontentlength"`
	LastModified  string `xml:"DAV: getlastmodified"`
	DisplayName   string `xml:"DAV: displayname"`
}

func (c *Client) propfind(p string, depth string) ([]remotefs.Entry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	resp, err := c.do(ctx, "PROPFIND", p, depth == "1", strings.NewReader(propfindBody),
		map[string]string{"Depth": depth, "Content-Type": "application/xml; charset=utf-8"},
		http.StatusMultiStatus)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var ms multistatus
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&ms); err != nil {
		return nil, fmt.Errorf("webdav: PROPFIND %s: bad response: %w", p, err)
	}
	out := make([]remotefs.Entry, 0, len(ms.Responses))
	for _, r := range ms.Responses {
		e, ok := c.entryFrom(r)
		if ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// sharePath converts an href (absolute URL or absolute path, percent
// encoded) to a share path, or reports false when it lies outside the share.
func (c *Client) sharePath(href string) (string, bool) {
	u, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	p := u.Path
	base := c.base.Path
	if p+"/" == base {
		return "/", true
	}
	if !strings.HasPrefix(p, base) {
		return "", false
	}
	return clean(strings.TrimPrefix(p, base)), true
}

func (c *Client) entryFrom(r davResponse) (remotefs.Entry, bool) {
	sp, ok := c.sharePath(r.Href)
	if !ok {
		return remotefs.Entry{}, false
	}
	var prop *davProp
	for i := range r.Propstat {
		if strings.Contains(r.Propstat[i].Status, " 200") {
			prop = &r.Propstat[i].Prop
			break
		}
	}
	if prop == nil {
		return remotefs.Entry{}, false
	}
	e := remotefs.Entry{Name: path.Base(sp), Path: sp}
	if sp == "/" {
		e.Name = "/"
	}
	if prop.ResourceType.Collection != nil {
		e.IsDir = true
		e.Mode = fs.ModeDir | 0o755
	} else {
		e.Mode = 0o644
		e.Size, _ = strconv.ParseInt(strings.TrimSpace(prop.ContentLength), 10, 64)
	}
	if t, err := http.ParseTime(strings.TrimSpace(prop.LastModified)); err == nil {
		e.ModTime = t
	}
	return e, true
}

// Ping checks that the share is reachable and the credentials work.
func (c *Client) Ping() error {
	_, err := c.Stat("/")
	return err
}

// ReadDir lists a folder (PROPFIND, Depth 1).
func (c *Client) ReadDir(dir string) ([]remotefs.Entry, error) {
	dir = clean(dir)
	all, err := c.propfind(dir, "1")
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, e := range all {
		if e.Path == dir {
			continue // the folder itself
		}
		out = append(out, e)
	}
	return out, nil
}

// Stat returns information about one path (PROPFIND, Depth 0).
func (c *Client) Stat(p string) (remotefs.Entry, error) {
	p = clean(p)
	es, err := c.propfind(p, "0")
	if err != nil {
		return remotefs.Entry{}, err
	}
	for _, e := range es {
		if e.Path == p {
			return e, nil
		}
	}
	if len(es) == 1 {
		return es[0], nil
	}
	return remotefs.Entry{}, &StatusError{Method: "PROPFIND", Path: p, Code: http.StatusNotFound, Status: "404 Not Found"}
}

// Mkdir creates a folder (MKCOL). Creating an existing folder fails.
func (c *Client) Mkdir(p string) error {
	return c.quick("MKCOL", p, true, nil, http.StatusCreated)
}

// MkdirAll creates a folder and any missing parents.
func (c *Client) MkdirAll(p string) error {
	p = clean(p)
	if p == "/" {
		return nil
	}
	if e, err := c.Stat(p); err == nil {
		if e.IsDir {
			return nil
		}
		return fmt.Errorf("webdav: %s exists and is not a folder", p)
	}
	if err := c.MkdirAll(path.Dir(p)); err != nil {
		return err
	}
	return c.Mkdir(p)
}

// Remove deletes a file or an empty folder. WebDAV's DELETE on a folder is
// always recursive, so emptiness is checked first to keep the FS contract.
func (c *Client) Remove(p string) error {
	e, err := c.Stat(p)
	if err != nil {
		return err
	}
	if e.IsDir {
		children, err := c.ReadDir(p)
		if err != nil {
			return err
		}
		if len(children) > 0 {
			return fmt.Errorf("webdav: folder %s is not empty", clean(p))
		}
	}
	return c.quick(http.MethodDelete, p, e.IsDir, nil, http.StatusNoContent, http.StatusOK)
}

// RemoveAll deletes a path and everything below it in one request.
func (c *Client) RemoveAll(p string) error {
	err := c.quick(http.MethodDelete, p, false, nil, http.StatusNoContent, http.StatusOK)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (c *Client) moveCopy(method, src, dst string, overwrite bool) error {
	ow := "F"
	if overwrite {
		ow = "T"
	}
	return c.quick(method, src, false, map[string]string{
		"Destination": c.urlFor(dst, false),
		"Overwrite":   ow,
		"Depth":       "infinity",
	}, http.StatusCreated, http.StatusNoContent)
}

// Rename moves a file or folder on the server (MOVE). It fails with an
// error matching fs.ErrExist when the destination exists.
func (c *Client) Rename(oldPath, newPath string) error {
	return c.moveCopy("MOVE", oldPath, newPath, false)
}

// RenameOverwrite is Rename replacing an existing destination.
func (c *Client) RenameOverwrite(oldPath, newPath string) error {
	return c.moveCopy("MOVE", oldPath, newPath, true)
}

// Copy copies a file or whole folder on the server itself (COPY), without
// moving any data through this machine.
func (c *Client) Copy(src, dst string, overwrite bool) error {
	return c.moveCopy("COPY", src, dst, overwrite)
}

// ---- contents -----------------------------------------------------------

// Open starts a GET of a remote file.
func (c *Client) Open(p string) (io.ReadCloser, error) {
	resp, err := c.do(context.Background(), http.MethodGet, p, false, nil, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// pipeWriter uploads what is written to it with a streaming PUT; Close
// waits for the server's answer.
type pipeWriter struct {
	pw   *io.PipeWriter
	done chan error
}

func (w *pipeWriter) Write(b []byte) (int, error) { return w.pw.Write(b) }
func (w *pipeWriter) Close() error {
	w.pw.Close()
	return <-w.done
}

// Create returns a writer that uploads to p; the upload completes on Close.
func (c *Client) Create(p string) (io.WriteCloser, error) {
	pr, pw := io.Pipe()
	w := &pipeWriter{pw: pw, done: make(chan error, 1)}
	go func() {
		resp, err := c.do(context.Background(), http.MethodPut, p, false, pr, nil,
			http.StatusCreated, http.StatusNoContent, http.StatusOK)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		pr.CloseWithError(err)
		w.done <- err
	}()
	return w, nil
}

// Download fetches a remote file. With Resume, an HTTP Range request
// fetches only the missing tail; a server that ignores Range (answers 200)
// is handled by starting over.
func (c *Client) Download(ctx context.Context, remote, local string, opts remotefs.TransferOptions) (int64, error) {
	remote = clean(remote)
	st, err := c.Stat(remote)
	if err != nil {
		return 0, err
	}
	if st.IsDir {
		return 0, fmt.Errorf("webdav: %s is a folder", remote)
	}
	var offset int64
	if lst, err := os.Stat(local); err == nil {
		switch {
		case lst.IsDir():
			return 0, fmt.Errorf("webdav: destination %s is a folder", local)
		case opts.Resume:
			offset = lst.Size()
		case opts.NoOverwrite:
			return 0, remotefs.ErrExists
		}
	}
	if opts.Resume && offset >= st.Size {
		if offset > st.Size {
			return 0, fmt.Errorf("webdav: cannot resume %s: local file is larger", local)
		}
		return 0, finishLocal(local, st, opts)
	}
	hdr := map[string]string{}
	if offset > 0 {
		hdr["Range"] = fmt.Sprintf("bytes=%d-", offset)
	}
	resp, err := c.do(ctx, http.MethodGet, remote, false, nil, hdr, http.StatusOK, http.StatusPartialContent)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if offset > 0 && resp.StatusCode == http.StatusPartialContent {
		flags = os.O_WRONLY | os.O_APPEND
	} else {
		offset = 0
	}
	f, err := os.OpenFile(local, flags, 0o644)
	if err != nil {
		return 0, err
	}
	pw := &remotefs.ProgressWriter{Ctx: ctx, W: f, Path: remote, Done: offset, Total: st.Size, Progress: opts.Progress}
	n, err := io.Copy(pw, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		return n, fmt.Errorf("webdav: downloading %s: %w", remote, err)
	}
	if offset+n != st.Size {
		return n, fmt.Errorf("webdav: downloading %s: got %d of %d bytes", remote, offset+n, st.Size)
	}
	return n, finishLocal(local, st, opts)
}

func finishLocal(local string, st remotefs.Entry, opts remotefs.TransferOptions) error {
	if opts.PreserveTimes && !st.ModTime.IsZero() {
		return os.Chtimes(local, st.ModTime, st.ModTime)
	}
	return nil
}

// Upload sends a local file with PUT. HTTP has no standard partial upload,
// so Resume only skips files that are already complete and otherwise
// returns remotefs.ErrResumeUnsupported, letting the UI offer "overwrite"
// instead. PreserveTimes and PreserveMode are not supported by WebDAV and
// are ignored.
func (c *Client) Upload(ctx context.Context, local, remote string, opts remotefs.TransferOptions) (int64, error) {
	remote = clean(remote)
	f, err := os.Open(local)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	lst, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if lst.IsDir() {
		return 0, fmt.Errorf("webdav: %s is a folder", local)
	}
	if st, err := c.Stat(remote); err == nil {
		switch {
		case st.IsDir:
			return 0, fmt.Errorf("webdav: destination %s is a folder", remote)
		case opts.Resume && st.Size == lst.Size():
			return 0, nil
		case opts.Resume:
			return 0, remotefs.ErrResumeUnsupported
		case opts.NoOverwrite:
			return 0, remotefs.ErrExists
		}
	}
	pr := &remotefs.ProgressReader{Ctx: ctx, R: f, Path: remote, Total: lst.Size(), Progress: opts.Progress}
	req, err := c.newRequest(ctx, http.MethodPut, remote, false, pr)
	if err != nil {
		return 0, err
	}
	req.ContentLength = lst.Size()
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return pr.Done, ctx.Err()
		}
		return pr.Done, fmt.Errorf("webdav: uploading %s: %w", remote, err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return pr.Done, nil
	}
	return pr.Done, &StatusError{Method: http.MethodPut, Path: remote, Code: resp.StatusCode, Status: resp.Status}
}
