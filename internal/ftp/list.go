// This file fetches folder listings for the panel. When the server offers
// MLSD (and the user has not switched it off) the exact, machine-readable
// listing is used; otherwise the client falls back to LIST, which it sends
// after changing into the folder because many servers misread a path with
// spaces as ls options. The text is handed to the parsers in parse.go with
// the connection's time zone and time offset. Stat answers "what is this
// one path" for the properties dialog and for copy conflicts, using MLST
// when possible and a listing of the parent folder otherwise.
package ftp

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

func (c *Client) parseOptions() ParseOptions {
	return ParseOptions{Location: c.cfg.ServerLocation, Offset: c.cfg.TimeOffset}
}

func (c *Client) useMLSD() bool {
	return !c.cfg.DisableMLSD && !c.noMLSD && c.hasFeature("MLST")
}

// List returns the entries of a remote folder ("" means the current one),
// without "." and "..".
func (c *Client) List(ctx context.Context, dir string) ([]Entry, error) {
	done, err := c.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	es, err := c.list(ctx, dir)
	return es, ctxErr(ctx, err)
}

func (c *Client) list(ctx context.Context, dir string) ([]Entry, error) {
	if c.useMLSD() {
		cmd := "MLSD"
		if dir != "" {
			cmd += " " + c.enc(dir)
		}
		lines, err := c.readDataLines(ctx, cmd)
		if err == nil {
			es, _ := ParseList(lines, c.parseOptions())
			return es, nil
		}
		if code := ReplyCode(err); code != 500 && code != 502 && code != 504 {
			return nil, err
		}
		c.noMLSD = true
	}
	if dir != "" {
		old, err := c.pwd()
		if err != nil {
			return nil, err
		}
		if err := c.expect2("CWD " + c.enc(dir)); err != nil {
			return nil, err
		}
		defer c.raw("CWD " + c.enc(old))
	}
	lines, err := c.readDataLines(ctx, c.cfg.ListCommand)
	if err != nil {
		// An empty folder is reported as an error by a few servers.
		if code := ReplyCode(err); code == 450 || code == 550 {
			if strings.Contains(strings.ToLower(err.Error()), "no files") {
				return nil, nil
			}
		}
		return nil, err
	}
	es, _ := ParseList(lines, c.parseOptions())
	return es, nil
}

// ListRaw returns the unparsed lines of LIST (or MLSD with mlsd=true), for
// diagnostics and for the "show raw listing" option.
func (c *Client) ListRaw(ctx context.Context, dir string, mlsd bool) ([]string, error) {
	done, err := c.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	cmd := c.cfg.ListCommand
	if mlsd {
		cmd = "MLSD"
	}
	if dir != "" {
		cmd += " " + c.enc(dir)
	}
	lines, err := c.readDataLines(ctx, cmd)
	return lines, ctxErr(ctx, err)
}

// NameList returns the bare names reported by NLST.
func (c *Client) NameList(ctx context.Context, dir string) ([]string, error) {
	done, err := c.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	cmd := "NLST"
	if dir != "" {
		cmd += " " + c.enc(dir)
	}
	lines, err := c.readDataLines(ctx, cmd)
	if err != nil {
		return nil, ctxErr(ctx, err)
	}
	for i, l := range lines {
		lines[i] = path.Base(strings.TrimSpace(l))
	}
	return lines, nil
}

// Stat describes one remote path. A missing path gives an error matching
// fs.ErrNotExist.
func (c *Client) Stat(ctx context.Context, p string) (Entry, error) {
	done, err := c.begin(ctx)
	if err != nil {
		return Entry{}, err
	}
	defer done()
	e, err := c.stat(ctx, p)
	return e, ctxErr(ctx, err)
}

func (c *Client) stat(ctx context.Context, p string) (Entry, error) {
	clean := path.Clean("/" + p)
	if clean == "/" && strings.HasPrefix(p, "/") || p == "" {
		return Entry{Name: "/", Type: TypeDir, Mode: fs.ModeDir, Size: -1}, nil
	}
	if c.hasFeature("MLST") && !c.cfg.DisableMLSD {
		r, err := c.raw("MLST " + c.enc(p))
		if err != nil {
			return Entry{}, err
		}
		if r.Code/100 == 2 {
			for _, l := range strings.Split(r.Msg, "\n") {
				if !strings.HasPrefix(l, " ") {
					continue
				}
				if e, ok := parseMLSx(l, c.parseOptions()); ok {
					e.Raw = strings.TrimSpace(l)
					e.Name = path.Base(e.Name)
					return e, nil
				}
			}
		} else if r.Code == 550 {
			return Entry{}, fmt.Errorf("ftp: %s: %w", p, fs.ErrNotExist)
		}
	}
	dir, base := path.Split(strings.TrimSuffix(p, "/"))
	parent := strings.TrimSuffix(dir, "/")
	if parent == "" && strings.HasPrefix(dir, "/") {
		parent = "/"
	}
	es, err := c.list(ctx, parent)
	if err != nil {
		return Entry{}, err
	}
	for _, e := range es {
		if e.Name == base {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("ftp: %s: %w", p, fs.ErrNotExist)
}
