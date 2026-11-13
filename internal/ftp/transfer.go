// This file moves file contents: streaming primitives (Retrieve and Store)
// for the viewer, editor and job engine, and the convenient Download and
// Upload calls that copy between a local file and the server with resume,
// progress callbacks and optional preservation of the modification time.
// Transfers run in binary mode, ASCII mode (line endings converted between
// the CRLF the protocol uses and the local convention), or Total
// Commander's automatic mode, which picks ASCII only for names matching a
// list of text-file masks. Resuming uses REST and so only works in binary
// mode, where byte offsets on both sides agree.
package ftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// TransferOptions tune Download and Upload.
type TransferOptions struct {
	// Mode overrides the connection's transfer mode for this file.
	Mode TransferMode
	// Resume continues a partial target instead of overwriting it: a
	// download appends to the shorter local file, an upload to the shorter
	// remote one. Ignored in ASCII mode.
	Resume bool
	// PreserveTime copies the source's modification time to the target
	// (MDTM on download, MFMT on upload when the server supports it).
	PreserveTime bool
	// Progress, when set, is called with the bytes of the target that now
	// exist and the total (-1 when unknown).
	Progress func(done, total int64)
}

// IsASCIIName reports whether name matches one of masks (case-insensitive
// shell patterns such as "*.txt").
func IsASCIIName(name string, masks []string) bool {
	base := strings.ToLower(path.Base(strings.ReplaceAll(name, "\\", "/")))
	for _, m := range masks {
		if ok, _ := path.Match(strings.ToLower(strings.TrimSpace(m)), base); ok {
			return true
		}
	}
	return false
}

// typeFor resolves a transfer mode to the TYPE letter for this file.
func (c *Client) typeFor(mode TransferMode, name string) byte {
	if mode == TransferDefault {
		mode = c.cfg.TransferMode
	}
	switch mode {
	case TransferASCII:
		return 'A'
	case TransferBinary:
		return 'I'
	}
	if IsASCIIName(name, c.cfg.ASCIIMasks) {
		return 'A'
	}
	return 'I'
}

// TransferTypeFor tells which mode ("binary" or "ascii") a file will use.
func (c *Client) TransferTypeFor(mode TransferMode, name string) TransferMode {
	if c.typeFor(mode, name) == 'A' {
		return TransferASCII
	}
	return TransferBinary
}

func modeOf(t byte) TransferMode {
	if t == 'A' {
		return TransferASCII
	}
	return TransferBinary
}

// transferReader is the stream returned by Retrieve. It holds the control
// lock until it is closed.
type transferReader struct {
	c      *Client
	conn   io.ReadCloser
	r      io.Reader
	done   func()
	stop   func() bool
	cmd    string
	eof    bool
	closed bool
	ctx    context.Context
}

func (t *transferReader) Read(p []byte) (int, error) {
	if t.closed {
		return 0, ErrClosed
	}
	n, err := t.r.Read(p)
	if err == io.EOF {
		t.eof = true
	} else if err != nil {
		err = ctxErr(t.ctx, err)
	}
	return n, err
}

func (t *transferReader) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true
	if t.stop != nil {
		t.stop()
	}
	t.conn.Close()
	defer t.done()
	err := t.c.finishData(t.cmd)
	if !t.eof {
		// Closing early makes the server abort with 426/451; that is the
		// expected outcome, not an error.
		var fe *Error
		if errors.As(err, &fe) && (fe.Code == 426 || fe.Code == 451 || fe.Code == 450) {
			return nil
		}
	}
	return ctxErr(t.ctx, err)
}

// Retrieve opens a remote file for reading, starting at offset (binary
// mode only). The client is busy until the returned stream is closed.
func (c *Client) Retrieve(ctx context.Context, p string, offset int64, mode TransferMode) (io.ReadCloser, error) {
	done, err := c.begin(ctx)
	if err != nil {
		return nil, err
	}
	t := c.typeFor(mode, p)
	if t == 'A' && offset > 0 {
		done()
		return nil, ErrResumeASCII
	}
	if err := c.setType(t); err != nil {
		done()
		return nil, ctxErr(ctx, err)
	}
	cmd := "RETR " + c.enc(p)
	conn, err := c.openData(ctx, cmd, offset)
	if err != nil {
		done()
		return nil, ctxErr(ctx, err)
	}
	if conn == nil {
		done()
		return io.NopCloser(strings.NewReader("")), nil
	}
	tr := &transferReader{c: c, conn: conn, r: conn, done: done, cmd: cmd, ctx: ctx}
	tr.stop = context.AfterFunc(ctx, func() { conn.Close() })
	if t == 'A' && asciiDownloadStripsCR {
		tr.r = &crlfToLF{r: conn}
	}
	return tr, nil
}

// Store uploads r to a remote file. With offset > 0 the server is asked to
// continue at that byte (REST+STOR, or APPE when REST STREAM is missing; r
// must then already be positioned at offset). It returns the number of
// bytes read from r.
func (c *Client) Store(ctx context.Context, p string, r io.Reader, offset int64, mode TransferMode) (int64, error) {
	done, err := c.begin(ctx)
	if err != nil {
		return 0, err
	}
	defer done()
	t := c.typeFor(mode, p)
	if t == 'A' && offset > 0 {
		return 0, ErrResumeASCII
	}
	n, err := c.store(ctx, "STOR", p, r, offset, t)
	return n, ctxErr(ctx, err)
}

// Append uploads r and appends it to the remote file (APPE).
func (c *Client) Append(ctx context.Context, p string, r io.Reader, mode TransferMode) (int64, error) {
	done, err := c.begin(ctx)
	if err != nil {
		return 0, err
	}
	defer done()
	n, err := c.store(ctx, "APPE", p, r, 0, c.typeFor(mode, p))
	return n, ctxErr(ctx, err)
}

func (c *Client) store(ctx context.Context, verb, p string, r io.Reader, offset int64, t byte) (int64, error) {
	if err := c.setType(t); err != nil {
		return 0, err
	}
	if offset > 0 && verb == "STOR" && !c.hasFeature("REST") {
		verb, offset = "APPE", 0
	}
	cmd := verb + " " + c.enc(p)
	conn, err := c.openData(ctx, cmd, offset)
	if err != nil {
		return 0, err
	}
	if conn == nil {
		return 0, nil
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	src := r
	if t == 'A' {
		src = &lfToCRLF{r: r}
	}
	cr := &countingReader{r: src}
	_, cerr := io.Copy(conn, cr)
	stop()
	closeErr := conn.Close()
	if cerr == nil {
		cerr = closeErr
	}
	ferr := c.finishData(cmd)
	n := cr.n
	if lf, ok := src.(*lfToCRLF); ok {
		n = lf.in
	}
	if cerr != nil {
		return n, cerr
	}
	return n, ferr
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

type progressWriter struct {
	w        io.Writer
	done     int64
	total    int64
	progress func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.progress != nil && n > 0 {
		p.progress(p.done, p.total)
	}
	return n, err
}

type progressReader struct {
	r        io.Reader
	done     int64
	total    int64
	progress func(done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.progress != nil && n > 0 {
		p.progress(p.done, p.total)
	}
	return n, err
}

// Download copies a remote file to a local path and returns the number of
// bytes transferred in this call. A partial local file is kept on failure
// so that a later call with Resume can continue it.
func (c *Client) Download(ctx context.Context, remote, local string, opt TransferOptions) (int64, error) {
	t := c.typeFor(opt.Mode, remote)
	total := int64(-1)
	if t == 'I' {
		if n, err := c.Size(ctx, remote); err == nil {
			total = n
		} else if ctx.Err() != nil {
			return 0, ctx.Err()
		}
	}
	var offset int64
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if opt.Resume && t == 'I' {
		if fi, err := os.Stat(local); err == nil && fi.Mode().IsRegular() && fi.Size() > 0 {
			if total >= 0 && fi.Size() > total {
				// A larger local file cannot be a partial copy of the
				// remote one; it is stale or different. Leave it alone
				// and say so instead of reporting success.
				return 0, fmt.Errorf("%w: %s", ErrResumeTooLarge, local)
			}
			if total >= 0 && fi.Size() == total {
				if opt.Progress != nil {
					opt.Progress(fi.Size(), total)
				}
				return 0, c.preserveLocal(ctx, opt, remote, local)
			}
			offset = fi.Size()
			flags = os.O_WRONLY
		}
	}
	rc, err := c.Retrieve(ctx, remote, offset, modeOf(t))
	if err != nil {
		return 0, err
	}
	f, err := os.OpenFile(local, flags, 0o666)
	if err != nil {
		rc.Close()
		return 0, err
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			rc.Close()
			return 0, err
		}
	}
	pw := &progressWriter{w: f, done: offset, total: total, progress: opt.Progress}
	n, cerr := io.Copy(pw, rc)
	rerr := rc.Close()
	ferr := f.Close()
	for _, e := range []error{cerr, rerr, ferr} {
		if e != nil {
			return n, ctxErr(ctx, e)
		}
	}
	return n, c.preserveLocal(ctx, opt, remote, local)
}

func (c *Client) preserveLocal(ctx context.Context, opt TransferOptions, remote, local string) error {
	if !opt.PreserveTime {
		return nil
	}
	mt, err := c.ModTime(ctx, remote)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil // the server cannot tell; keep the download time
	}
	return os.Chtimes(local, mt, mt)
}

// Upload copies a local file to the server and returns the number of
// bytes of the local file sent in this call.
func (c *Client) Upload(ctx context.Context, local, remote string, opt TransferOptions) (int64, error) {
	f, err := os.Open(local)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if fi.IsDir() {
		return 0, fmt.Errorf("ftp: %s is a folder", local)
	}
	t := c.typeFor(opt.Mode, local)
	var offset int64
	if opt.Resume && t == 'I' {
		if rs, err := c.Size(ctx, remote); err == nil && rs > 0 {
			if rs > fi.Size() {
				// The remote file is larger than the local source, so it
				// is not a partial upload of it; refuse rather than hide
				// a stale or different file behind a "success".
				return 0, fmt.Errorf("%w: %s", ErrResumeTooLarge, remote)
			}
			if rs == fi.Size() {
				if opt.Progress != nil {
					opt.Progress(fi.Size(), fi.Size())
				}
				return 0, c.preserveRemote(ctx, opt, fi, remote)
			}
			offset = rs
			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				return 0, err
			}
		} else if ctx.Err() != nil {
			return 0, ctx.Err()
		}
	}
	pr := &progressReader{r: f, done: offset, total: fi.Size(), progress: opt.Progress}
	n, err := c.Store(ctx, remote, pr, offset, modeOf(t))
	if err != nil {
		return n, err
	}
	return n, c.preserveRemote(ctx, opt, fi, remote)
}

func (c *Client) preserveRemote(ctx context.Context, opt TransferOptions, fi os.FileInfo, remote string) error {
	if !opt.PreserveTime {
		return nil
	}
	err := c.SetModTime(ctx, remote, fi.ModTime())
	if errors.Is(err, ErrNotSupported) || ReplyCode(err) != 0 {
		return nil // optional nicety; the upload itself succeeded
	}
	return err
}

func isEOF(err error) bool { return errors.Is(err, io.EOF) }

// crlfToLF turns CRLF into LF for ASCII downloads on systems whose native
// line ending is LF. A lone CR is kept.
type crlfToLF struct {
	r       io.Reader
	pending bool // a CR was read at the end of the previous chunk
	buf     []byte
	err     error
}

func (c *crlfToLF) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if c.err != nil && len(c.buf) == 0 {
			if c.pending {
				c.pending = false
				p[0] = '\r'
				return 1, nil
			}
			return 0, c.err
		}
		if len(c.buf) == 0 {
			tmp := make([]byte, len(p))
			n, err := c.r.Read(tmp)
			c.buf = tmp[:n]
			c.err = err
			if n == 0 {
				continue
			}
		}
		out := 0
		for len(c.buf) > 0 && out < len(p) {
			b := c.buf[0]
			if c.pending {
				c.pending = false
				if b != '\n' {
					p[out] = '\r'
					out++
					continue // re-examine b on the next pass
				}
			}
			c.buf = c.buf[1:]
			if b == '\r' {
				c.pending = true
				continue
			}
			p[out] = b
			out++
		}
		if out > 0 {
			return out, nil
		}
	}
}

// lfToCRLF turns bare LF into CRLF for ASCII uploads; existing CRLF pairs
// are left alone, so Windows text files are not doubled.
type lfToCRLF struct {
	r     io.Reader
	prev  byte
	extra bool // an LF is owed after an inserted CR
	in    int64
}

func (l *lfToCRLF) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	out := 0
	if l.extra {
		p[0] = '\n'
		l.extra = false
		l.prev = '\n'
		out = 1
	}
	if out == len(p) {
		return out, nil
	}
	tmp := make([]byte, (len(p)-out+1)/2)
	n, err := l.r.Read(tmp)
	l.in += int64(n)
	for _, b := range tmp[:n] {
		if b == '\n' && l.prev != '\r' {
			p[out] = '\r'
			out++
			if out == len(p) {
				l.extra = true
				l.prev = '\r'
				continue
			}
		}
		p[out] = b
		out++
		l.prev = b
	}
	if err == io.EOF && l.extra {
		err = nil
	}
	return out, err
}
