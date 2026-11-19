// Package rclone runs the user's own rclone program (https://rclone.org) to reach the cloud and object-storage remotes
// (S3, R2, B2, Azure, GCS, WebDAV, Drive, Dropbox, OneDrive, SMB …) defined in the user's rclone.conf (issue #18,
// F-866). It only starts rclone as a subprocess with an argument list (lsjson, cat, rcat, copyto, moveto, deletefile,
// rmdir, purge, mkdir, listremotes); it never downloads rclone, never writes rclone's configuration (rclone itself
// reads its own config file the usual way, and --ask-password=false keeps an encrypted config from waiting for a
// password on a stdin nobody types into), and it starts nothing by itself: the callers run it only for a remote the
// user opened. All network traffic is rclone's, to the remotes the user configured.
package rclone

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ErrNotFound means no rclone program was found (none configured and none on PATH).
var ErrNotFound = errors.New("rclone was not found: install rclone or set its path in Options > FTP")

// Find returns the rclone program to run: the configured path when one is set, else "rclone" looked up on PATH.
func Find(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		fi, err := os.Stat(configured)
		if err != nil || fi.IsDir() {
			return "", fmt.Errorf("the rclone program %s was not found (Options > FTP)", configured)
		}
		return configured, nil
	}
	p, err := exec.LookPath("rclone")
	if err != nil {
		return "", ErrNotFound
	}
	return p, nil
}

// Client runs one rclone program.
type Client struct {
	Bin     string          // the rclone program
	Env     []string        // extra environment entries (tests); the user's environment is always passed on
	Prepare func(*exec.Cmd) // called on every command before it starts (Windows: hide the console window)
	Timeout time.Duration   // longest run of a short command (listing, stat, delete …); 0 = 10 minutes
}

// common are the flags every command gets: never ask for a config password on stdin.
var common = []string{"--ask-password=false"}

// ValidRemote says whether name can be a remote of rclone.conf. Connection strings (":s3,…:"), paths and flags are
// refused, so a panel address can only ever name a configured remote.
func ValidRemote(name string) bool {
	if name == "" || len(name) > 200 || strings.ContainsAny(name, ":/\\\x00\r\n\t") {
		return false
	}
	if name[0] == '-' || name[0] == ' ' || name[len(name)-1] == ' ' {
		return false
	}
	return true
}

// Arg is the rclone argument for path p ("/a/b" or "a/b") on remote: "remote:a/b" (paths are relative to the
// remote's root, as rclone users write them).
func Arg(remote, p string) string {
	return remote + ":" + strings.TrimLeft(strings.ReplaceAll(p, `\`, "/"), "/")
}

func (c *Client) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Bin, append(append([]string{}, common...), args...)...)
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	if c.Prepare != nil {
		c.Prepare(cmd)
	}
	return cmd
}

func (c *Client) timeout(ctx context.Context) (context.Context, context.CancelFunc) {
	d := c.Timeout
	if d <= 0 {
		d = 10 * time.Minute
	}
	return context.WithTimeout(ctx, d)
}

// run runs a short command and returns its standard output.
func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	if c.Bin == "" {
		return nil, ErrNotFound
	}
	ctx, cancel := c.timeout(ctx)
	defer cancel()
	cmd := c.command(ctx, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), c.fail(ctx, args, err, errb.Bytes())
	}
	return out.Bytes(), nil
}

// Error is a failed rclone command.
type Error struct {
	Op   string // the rclone command (lsjson, copyto …)
	Code int    // the exit code, -1 when it did not run to the end
	Msg  string // rclone's own error message
	err  error  // fs.ErrNotExist for "not found" (exit codes 3 and 4), else nil
}

func (e *Error) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("rclone %s failed (exit code %d)", e.Op, e.Code)
	}
	return "rclone: " + e.Msg
}

func (e *Error) Unwrap() error { return e.err }

func (c *Client) fail(ctx context.Context, args []string, err error, stderr []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	op := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			op = a
			break
		}
	}
	e := &Error{Op: op, Code: -1, Msg: ErrorText(stderr)}
	var xe *exec.ExitError
	if errors.As(err, &xe) {
		e.Code = xe.ExitCode()
	} else if e.Msg == "" {
		e.Msg = err.Error()
	}
	low := strings.ToLower(e.Msg)
	if e.Code == 3 || e.Code == 4 || strings.Contains(low, "directory not found") || strings.Contains(low, "object not found") {
		e.err = fs.ErrNotExist
	}
	return e
}

var logPrefix = regexp.MustCompile(`^\d{4}/\d\d/\d\d \d\d:\d\d:\d\d (?:[A-Z]+\s*:\s*)?`)

// ErrorText picks rclone's error message from its standard error: the last ERROR or CRITICAL line (plain or
// --use-json-log), else the last line, without the date prefix.
func ErrorText(stderr []byte) string {
	var last, lastErr string
	for _, line := range strings.Split(string(stderr), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "{") {
			var j struct {
				Level string `json:"level"`
				Msg   string `json:"msg"`
			}
			if json.Unmarshal([]byte(line), &j) == nil && j.Msg != "" {
				if j.Level == "error" || j.Level == "critical" || j.Level == "fatal" {
					lastErr = j.Msg
				}
				continue
			}
		}
		up := strings.ToUpper(line)
		if strings.Contains(up, "ERROR :") || strings.Contains(up, "CRITICAL:") || strings.HasPrefix(up, "FAILED") {
			lastErr = logPrefix.ReplaceAllString(line, "")
		}
		last = logPrefix.ReplaceAllString(line, "")
	}
	if lastErr != "" {
		return strings.TrimSpace(strings.TrimPrefix(lastErr, "Failed to "))
	}
	return last
}

// Remote is one remote of rclone.conf.
type Remote struct {
	Name string `json:"name"`
	Type string `json:"type"` // s3, drive, onedrive … ("" when rclone did not say)
}

// Remotes lists the configured remotes (rclone listremotes --long; this reads the config file only).
func (c *Client) Remotes(ctx context.Context) ([]Remote, error) {
	out, err := c.run(ctx, "listremotes", "--long")
	if err != nil {
		return nil, err
	}
	return ParseRemotes(out), nil
}

// ParseRemotes reads the output of rclone listremotes [--long]: "name: type" or "name:" per line.
func ParseRemotes(out []byte) []Remote {
	var rs []Remote
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		i := strings.Index(line, ":")
		if i <= 0 {
			continue
		}
		r := Remote{Name: line[:i], Type: strings.TrimSpace(line[i+1:])}
		if ValidRemote(r.Name) {
			rs = append(rs, r)
		}
	}
	return rs
}

// Item is one entry of rclone lsjson.
type Item struct {
	Path     string    `json:"Path"`
	Name     string    `json:"Name"`
	Size     int64     `json:"Size"`
	MimeType string    `json:"MimeType"`
	ModTime  time.Time `json:"ModTime"`
	IsDir    bool      `json:"IsDir"`
}

// List reads the folder p of remote (rclone lsjson). A path that is a file lists as that one file, as rclone does.
func (c *Client) List(ctx context.Context, remote, p string) ([]Item, error) {
	out, err := c.run(ctx, "lsjson", "--no-mimetype", Arg(remote, p))
	if err != nil {
		return nil, err
	}
	var items []Item
	if err := json.Unmarshal(bytes.TrimSpace(out), &items); err != nil {
		return nil, fmt.Errorf("rclone lsjson: unreadable answer: %w", err)
	}
	return items, nil
}

// Stat describes the path p of remote (rclone lsjson --stat).
func (c *Client) Stat(ctx context.Context, remote, p string) (Item, error) {
	out, err := c.run(ctx, "lsjson", "--stat", "--no-mimetype", Arg(remote, p))
	if err != nil {
		return Item{}, err
	}
	var it Item
	if err := json.Unmarshal(bytes.TrimSpace(out), &it); err != nil {
		return Item{}, fmt.Errorf("rclone lsjson: unreadable answer: %w", err)
	}
	return it, nil
}

// Mkdir makes the folder p (and the folders above it).
func (c *Client) Mkdir(ctx context.Context, remote, p string) error {
	_, err := c.run(ctx, "mkdir", Arg(remote, p))
	return err
}

// DeleteFile deletes one file.
func (c *Client) DeleteFile(ctx context.Context, remote, p string) error {
	_, err := c.run(ctx, "deletefile", Arg(remote, p))
	return err
}

// Rmdir deletes an empty folder.
func (c *Client) Rmdir(ctx context.Context, remote, p string) error {
	_, err := c.run(ctx, "rmdir", Arg(remote, p))
	return err
}

// Purge deletes a folder with everything in it.
func (c *Client) Purge(ctx context.Context, remote, p string) error {
	_, err := c.run(ctx, "purge", Arg(remote, p))
	return err
}

// MoveTo moves (renames) one file or folder; src and dst are rclone arguments ("remote:path" or an absolute local
// path). rclone moves on the server when the provider can.
func (c *Client) MoveTo(ctx context.Context, src, dst string) error {
	_, err := c.run(ctx, "moveto", src, dst)
	return err
}

// CopyTo copies one file; src and dst are rclone arguments ("remote:path" or an absolute local path). Between two
// paths of the same provider rclone copies on the server. progress (may be nil) gets the bytes done and the total
// from rclone's statistics, about twice a second.
func (c *Client) CopyTo(ctx context.Context, src, dst string, progress func(done, total int64)) error {
	if c.Bin == "" {
		return ErrNotFound
	}
	args := []string{"copyto", src, dst}
	if progress != nil {
		args = append(args, "--use-json-log", "-v", "--stats=500ms")
	}
	cmd := c.command(ctx, args...)
	var errb bytes.Buffer
	pr, pw := io.Pipe()
	cmd.Stderr = pw
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Bytes()
			errb.Write(line)
			errb.WriteByte('\n')
			if progress != nil {
				if d, t, ok := statsOf(line); ok {
					progress(d, t)
				}
			}
		}
		io.Copy(io.Discard, pr)
	}()
	err := cmd.Run()
	pw.Close()
	<-done
	if err != nil {
		return c.fail(ctx, args, err, errb.Bytes())
	}
	return nil
}

// statsOf reads the transfer statistics of one --use-json-log line.
func statsOf(line []byte) (done, total int64, ok bool) {
	if !bytes.Contains(line, []byte(`"stats"`)) {
		return 0, 0, false
	}
	var j struct {
		Stats *struct {
			Bytes      int64 `json:"bytes"`
			TotalBytes int64 `json:"totalBytes"`
		} `json:"stats"`
	}
	if json.Unmarshal(line, &j) != nil || j.Stats == nil {
		return 0, 0, false
	}
	return j.Stats.Bytes, j.Stats.TotalBytes, true
}

// Cat opens the file p for reading (rclone cat streams it).
func (c *Client) Cat(ctx context.Context, remote, p string) (io.ReadCloser, error) {
	if c.Bin == "" {
		return nil, ErrNotFound
	}
	ctx, cancel := context.WithCancel(ctx)
	args := []string{"cat", Arg(remote, p)}
	cmd := c.command(ctx, args...)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	return &catReader{c: c, ctx: ctx, cancel: cancel, cmd: cmd, out: out, errb: &errb, args: args}, nil
}

type catReader struct {
	c      *Client
	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd
	out    io.ReadCloser
	errb   *bytes.Buffer
	args   []string
	eof    bool
	closed bool
	werr   error
}

func (r *catReader) Read(b []byte) (int, error) {
	n, err := r.out.Read(b)
	if err == io.EOF && !r.eof {
		r.eof = true
		if werr := r.cmd.Wait(); werr != nil {
			r.werr = r.c.fail(r.ctx, r.args, werr, r.errb.Bytes())
		}
		r.closed = true
		if r.werr != nil {
			return n, r.werr
		}
	}
	return n, err
}

func (r *catReader) Close() error {
	if !r.closed {
		r.closed = true
		r.cancel()
		r.out.Close()
		r.cmd.Wait()
	}
	r.cancel()
	return nil
}

// Rcat creates or replaces the file p with what is written; the file is complete when Close returns nil.
func (c *Client) Rcat(ctx context.Context, remote, p string) (io.WriteCloser, error) {
	if c.Bin == "" {
		return nil, ErrNotFound
	}
	args := []string{"rcat", Arg(remote, p)}
	cmd := c.command(ctx, args...)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &rcatWriter{c: c, ctx: ctx, cmd: cmd, in: in, errb: &errb, args: args}, nil
}

type rcatWriter struct {
	c    *Client
	ctx  context.Context
	cmd  *exec.Cmd
	in   io.WriteCloser
	errb *bytes.Buffer
	args []string
	done bool
}

func (w *rcatWriter) Write(b []byte) (int, error) { return w.in.Write(b) }

func (w *rcatWriter) Close() error {
	if w.done {
		return nil
	}
	w.done = true
	w.in.Close()
	if err := w.cmd.Wait(); err != nil {
		return w.c.fail(w.ctx, w.args, err, w.errb.Bytes())
	}
	return nil
}
