// This file implements the classic SCP protocol (the "rcp over ssh" wire
// format spoken by "scp -t" and "scp -f" on the server). Total Commander's
// SFTP plugin falls back to SCP for accounts whose server has no SFTP
// subsystem, and SCP is still faster on some embedded devices and routers.
// SCP can only copy files and folders, not list or rename them, so it
// complements rather than replaces the SFTP client. The download side checks
// every name the server sends and refuses ones containing "/" or "..": a
// malicious server must not be able to write outside the chosen folder (the
// mistake behind CVE-2019-6111 in the original scp). It also accepts only
// the one item that was asked for at the top level, under its own name, so
// a hostile server cannot drop extra files (say a ".bashrc") into the
// target folder next to the requested one.
package sftp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"mormicommander/internal/remotefs"
)

// SCPOptions controls an SCP transfer.
type SCPOptions struct {
	// Preserve copies modification/access times and permission bits (scp -p).
	Preserve bool
	// Progress receives per-file progress.
	Progress remotefs.ProgressFunc
	// OnFile is called before each file with its source and destination.
	OnFile func(src, dst string)
	// Command is the remote scp program; default "scp".
	Command string
}

func (o *SCPOptions) command() string {
	if o.Command != "" {
		return o.Command
	}
	return "scp"
}

// SCPError is a warning or fatal error reported by the remote scp.
type SCPError struct {
	Fatal   bool
	Message string
}

func (e *SCPError) Error() string { return "scp: " + e.Message }

// shellQuote quotes s for a POSIX shell, which is what the server runs the
// scp command line through.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// scpSession bundles a running remote scp with its pipes.
type scpSession struct {
	s      *ssh.Session
	in     io.WriteCloser
	out    *bufio.Reader
	stderr lockedBuffer
}

// lockedBuffer collects the remote stderr, which x/crypto/ssh copies from
// its own goroutine while we may read it.
type lockedBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func (c *Client) startSCP(ctx context.Context, cmd string) (*scpSession, func(), error) {
	s, err := c.conn.NewSession()
	if err != nil {
		return nil, nil, fmt.Errorf("scp: opening session: %w", err)
	}
	ss := &scpSession{s: s}
	ss.in, err = s.StdinPipe()
	if err != nil {
		s.Close()
		return nil, nil, err
	}
	stdout, err := s.StdoutPipe()
	if err != nil {
		s.Close()
		return nil, nil, err
	}
	ss.out = bufio.NewReader(stdout)
	s.Stderr = &ss.stderr
	if err := s.Start(cmd); err != nil {
		s.Close()
		return nil, nil, fmt.Errorf("scp: starting %q: %w", cmd, err)
	}
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			s.Close()
		case <-stop:
		}
	}()
	return ss, func() { close(stop); s.Close() }, nil
}

// readAck reads one response byte: 0 = ok, 1 = warning + message, 2 = fatal
// + message.
func (ss *scpSession) readAck() error {
	b, err := ss.out.ReadByte()
	if err != nil {
		// The remote scp went away; its exit status and stderr (complete
		// only after Wait) say why.
		if ferr := ss.finish(); ferr != nil {
			return ferr
		}
		return fmt.Errorf("scp: reading response: %w", err)
	}
	switch b {
	case 0:
		return nil
	case 1, 2:
		msg, _ := ss.out.ReadString('\n')
		return &SCPError{Fatal: b == 2, Message: strings.TrimSpace(msg)}
	default:
		return fmt.Errorf("scp: unexpected response byte %d", b)
	}
}

func (ss *scpSession) send(format string, args ...any) error {
	_, err := fmt.Fprintf(ss.in, format, args...)
	return err
}

// finish closes stdin and waits for the remote scp, turning a non-zero exit
// into an error carrying its stderr.
func (ss *scpSession) finish() error {
	ss.in.Close()
	err := ss.s.Wait()
	if err != nil {
		var exit *ssh.ExitError
		if errors.As(err, &exit) {
			msg := strings.TrimSpace(ss.stderr.String())
			if msg == "" {
				msg = fmt.Sprintf("remote scp exited with status %d", exit.ExitStatus())
			}
			return &SCPError{Fatal: true, Message: msg}
		}
		var missing *ssh.ExitMissingError
		if errors.As(err, &missing) {
			return nil
		}
		return err
	}
	return nil
}

// SCPUpload sends a local file or folder (recursively) to remoteTarget using
// SCP. As with the scp command, when remoteTarget is an existing folder the
// item is created inside it under its own name; otherwise remoteTarget is the
// new name. It returns the bytes sent.
func (c *Client) SCPUpload(ctx context.Context, local, remoteTarget string, opts SCPOptions) (int64, error) {
	end, err := c.beginAny()
	if err != nil {
		return 0, err
	}
	defer end()
	fi, err := os.Stat(local)
	if err != nil {
		return 0, err
	}
	cmd := opts.command() + " -t"
	if fi.IsDir() {
		cmd += " -r"
	}
	if opts.Preserve {
		cmd += " -p"
	}
	cmd += " -- " + shellQuote(c.abs(remoteTarget))
	ss, stop, err := c.startSCP(ctx, cmd)
	if err != nil {
		return 0, err
	}
	defer stop()
	if err := ss.readAck(); err != nil {
		return 0, c.scpErr(ctx, err)
	}
	var total int64
	if err := c.scpSend(ctx, ss, local, fi, remoteTarget, opts, &total); err != nil {
		return total, c.scpErr(ctx, err)
	}
	if err := ss.finish(); err != nil {
		return total, c.scpErr(ctx, err)
	}
	return total, nil
}

func (c *Client) scpErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (c *Client) scpSend(ctx context.Context, ss *scpSession, local string, fi fs.FileInfo, remote string, opts SCPOptions, total *int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name := fi.Name()
	if strings.ContainsAny(name, "\n") {
		return fmt.Errorf("scp: cannot send file name with a newline: %q", name)
	}
	if opts.Preserve {
		mt := fi.ModTime().Unix()
		if err := ss.send("T%d 0 %d 0\n", mt, mt); err != nil {
			return err
		}
		if err := ss.readAck(); err != nil {
			return err
		}
	}
	mode := fi.Mode().Perm()
	if fi.IsDir() {
		if err := ss.send("D%04o 0 %s\n", mode, name); err != nil {
			return err
		}
		if err := ss.readAck(); err != nil {
			return err
		}
		entries, err := os.ReadDir(local)
		if err != nil {
			return err
		}
		for _, de := range entries {
			p := filepath.Join(local, de.Name())
			cfi, err := os.Stat(p) // follow links like scp does
			if err != nil {
				continue
			}
			if err := c.scpSend(ctx, ss, p, cfi, path.Join(remote, de.Name()), opts, total); err != nil {
				return err
			}
		}
		if err := ss.send("E\n"); err != nil {
			return err
		}
		return ss.readAck()
	}
	if !fi.Mode().IsRegular() {
		return nil // devices, sockets: skipped like scp does
	}
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	if opts.OnFile != nil {
		opts.OnFile(local, remote)
	}
	size := fi.Size()
	if err := ss.send("C%04o %d %s\n", mode, size, name); err != nil {
		return err
	}
	if err := ss.readAck(); err != nil {
		return err
	}
	pw := &remotefs.ProgressWriter{Ctx: ctx, W: ss.in, Path: remote, Total: size, Progress: c.progressFor(opts.Progress)}
	n, err := io.CopyN(pw, f, size)
	*total += n
	if err != nil {
		return err
	}
	if _, err := ss.in.Write([]byte{0}); err != nil {
		return err
	}
	return ss.readAck()
}

// SCPDownload fetches a remote file or folder (recursively) to local using
// SCP. When local is an existing folder the item is created inside it under
// its own name; otherwise local is the new name. It returns the bytes
// received.
func (c *Client) SCPDownload(ctx context.Context, remote, local string, opts SCPOptions) (int64, error) {
	end, err := c.beginAny()
	if err != nil {
		return 0, err
	}
	defer end()
	cmd := opts.command() + " -f -r"
	if opts.Preserve {
		cmd += " -p"
	}
	cmd += " -- " + shellQuote(c.abs(remote))
	ss, stop, err := c.startSCP(ctx, cmd)
	if err != nil {
		return 0, err
	}
	defer stop()

	ack := func() error { _, err := ss.in.Write([]byte{0}); return err }
	if err := ack(); err != nil {
		return 0, err
	}

	targetIsDir := false
	if fi, err := os.Stat(local); err == nil && fi.IsDir() {
		targetIsDir = true
	}
	type dirFrame struct {
		path         string
		mtime, atime time.Time
		hasTimes     bool
		mode         fs.FileMode
	}
	var stack []dirFrame
	var total int64
	var pendTimes bool
	var pmtime, patime time.Time
	var warnings []error
	first := true
	// The top level may carry exactly one item: the one requested, with
	// its own base name. Anything else is the second half of
	// CVE-2019-6111 (a server sending files nobody asked for).
	wantName := path.Base(c.abs(remote))
	topSeen := false
	checkTop := func(name string) error {
		if len(stack) > 0 {
			return nil
		}
		if topSeen {
			return fmt.Errorf("scp: server sent %q in addition to the requested item", name)
		}
		if targetIsDir && name != wantName {
			return fmt.Errorf("scp: server sent %q instead of the requested %q", name, wantName)
		}
		topSeen = true
		return nil
	}

	// destFor picks the local path for a received item name.
	destFor := func(name string) string {
		if len(stack) > 0 {
			return filepath.Join(stack[len(stack)-1].path, name)
		}
		if targetIsDir {
			return filepath.Join(local, name)
		}
		return local
	}

	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		line, err := ss.out.ReadString('\n')
		if err == io.EOF && line == "" {
			break
		}
		if err != nil {
			return total, c.scpErr(ctx, fmt.Errorf("scp: reading from server: %w", err))
		}
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			return total, errors.New("scp: empty protocol line")
		}
		switch line[0] {
		case 1, 2:
			e := &SCPError{Fatal: line[0] == 2, Message: line[1:]}
			if e.Fatal {
				return total, e
			}
			warnings = append(warnings, e)
		case 'T':
			var mt, mu, at, au int64
			if _, err := fmt.Sscanf(line, "T%d %d %d %d", &mt, &mu, &at, &au); err != nil {
				return total, fmt.Errorf("scp: bad time line %q", line)
			}
			pendTimes, pmtime, patime = true, time.Unix(mt, 0), time.Unix(at, 0)
			if err := ack(); err != nil {
				return total, err
			}
		case 'D':
			mode, _, name, err := parseSCPHeader(line)
			if err != nil {
				return total, err
			}
			if err := checkTop(name); err != nil {
				return total, err
			}
			dst := destFor(name)
			first = false
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return total, err
			}
			stack = append(stack, dirFrame{path: dst, mtime: pmtime, atime: patime, hasTimes: pendTimes, mode: mode})
			pendTimes = false
			if err := ack(); err != nil {
				return total, err
			}
		case 'E':
			if len(stack) == 0 {
				return total, errors.New("scp: unbalanced end-of-folder")
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if opts.Preserve {
				os.Chmod(top.path, top.mode|0o700)
				if top.hasTimes {
					os.Chtimes(top.path, top.atime, top.mtime)
				}
			}
			if err := ack(); err != nil {
				return total, err
			}
		case 'C':
			mode, size, name, err := parseSCPHeader(line)
			if err != nil {
				return total, err
			}
			if err := checkTop(name); err != nil {
				return total, err
			}
			dst := destFor(name)
			first = false
			if opts.OnFile != nil {
				opts.OnFile(path.Join(remote, name), dst)
			}
			f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
			if err != nil {
				return total, err
			}
			if err := ack(); err != nil {
				f.Close()
				return total, err
			}
			pw := &remotefs.ProgressWriter{Ctx: ctx, W: f, Path: path.Join(remote, name), Total: size,
				Progress: c.progressFor(opts.Progress)}
			n, err := io.CopyN(pw, ss.out, size)
			total += n
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return total, c.scpErr(ctx, err)
			}
			if err := ss.readAck(); err != nil {
				return total, err
			}
			if opts.Preserve {
				os.Chmod(dst, mode)
				if pendTimes {
					os.Chtimes(dst, patime, pmtime)
				}
			}
			pendTimes = false
			if err := ack(); err != nil {
				return total, err
			}
		default:
			return total, fmt.Errorf("scp: unexpected protocol line %q", line)
		}
	}
	ferr := ss.finish()
	if len(warnings) > 0 {
		// The server's own messages explain more than its exit status.
		return total, errors.Join(warnings...)
	}
	if ferr != nil {
		return total, c.scpErr(ctx, ferr)
	}
	if first {
		return total, errors.New("scp: server sent nothing")
	}
	return total, nil
}

// parseSCPHeader parses "C0644 123 name" / "D0755 0 name" and validates the
// name so the server cannot escape the destination folder.
func parseSCPHeader(line string) (fs.FileMode, int64, string, error) {
	parts := strings.SplitN(line[1:], " ", 3)
	if len(parts) != 3 {
		return 0, 0, "", fmt.Errorf("scp: bad header %q", line)
	}
	mode, err := strconv.ParseUint(parts[0], 8, 32)
	if err != nil {
		return 0, 0, "", fmt.Errorf("scp: bad mode in %q", line)
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || size < 0 {
		return 0, 0, "", fmt.Errorf("scp: bad size in %q", line)
	}
	name := parts[2]
	// filepath.IsLocal also rejects names this OS cannot store safely,
	// such as "x:stream" or "CON" on Windows.
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || !filepath.IsLocal(name) {
		return 0, 0, "", fmt.Errorf("scp: server sent unsafe file name %q", name)
	}
	return fs.FileMode(mode).Perm(), size, name, nil
}
