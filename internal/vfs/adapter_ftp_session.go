// This file adds the per-session controls Total Commander's FTP toolbar
// offers while a server is shown in a panel: the transfer mode button
// (binary, ASCII or automatic by file mask, Ctrl+Shift+M) and "Send FTP
// command". Both act on the live session of a panel URL. The transfer mode
// of a session is swapped by replacing the session record (sharing its
// connection pool), so transfers that already run keep the mode they
// started with and no lock is needed on the hot path.
package vfs

import (
	"context"
	"errors"
	"strconv"

	"mormicommander/internal/ftp"
)

// ErrNoSession is returned for a URL whose server is not connected.
var ErrNoSession = errors.New("vfs: not connected to this server")

// TransferMode returns the transfer mode of the session p belongs to.
func (f *FTPFS) TransferMode(p string) (ftp.TransferMode, error) {
	u, err := parseRemote(p)
	if err != nil {
		return ftp.TransferDefault, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.sessions[u.key()]
	if s == nil {
		return ftp.TransferDefault, ErrNoSession
	}
	m := s.cfg.TransferMode
	if m == ftp.TransferDefault {
		m = ftp.TransferAuto
	}
	return m, nil
}

// SetTransferMode changes the transfer mode used by Download and Upload for
// the session p belongs to.
func (f *FTPFS) SetTransferMode(p string, m ftp.TransferMode) error {
	u, err := parseRemote(p)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.sessions[u.key()]
	if s == nil {
		return ErrNoSession
	}
	ns := *s
	ns.cfg.TransferMode = m
	f.sessions[u.key()] = &ns
	return nil
}

// Quote sends a raw command on one of the session's connections and returns
// the server's reply as "code text" (TC's "Send FTP command").
func (f *FTPFS) Quote(ctx context.Context, p, line string) (string, error) {
	var out string
	err := f.with(ctx, p, func(c *ftp.Client, _ string) error {
		r, err := c.Quote(ctx, line)
		if r.Code != 0 {
			out = strconv.Itoa(r.Code) + " " + r.Msg
		}
		return err
	})
	return out, err
}
