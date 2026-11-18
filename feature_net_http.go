package main

// HTTP and HTTPS downloads (F-644, TC: a URL typed in "FTP connect"): an http:// or https:// URL given to a transfer
// (quick connect, Ctrl+N, hands it over) is downloaded into a folder on this computer with the progress box of the
// server transfers. The file name comes from the last part of the URL path ("index.html" when there is none); a file
// of that name in the way is asked about like any other transfer, with "Resume" (an HTTP range request) when the file
// there is shorter. Nothing is fetched unless the user starts it.

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"

	"mormicommander/internal/httpdl"
	"mormicommander/internal/remotefs"
	"mormicommander/internal/vfs"
)

// netIsHTTP says whether p is a web address a transfer can download.
func netIsHTTP(p string) bool {
	l := strings.ToLower(strings.TrimSpace(p))
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

// netHTTPName is the local file name for a web address: the last part of its path, or "index.html".
func netHTTPName(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", errors.New("this is not a web address")
	}
	name := path.Base(u.Path)
	if name == "/" || name == "." || name == "" {
		name = "index.html"
	}
	if !netPlainName(name) || strings.ContainsRune(name, '\\') {
		return "", fmt.Errorf("%q is not a plain file name", name)
	}
	if _, err := remotefs.LocalRel(name); err != nil {
		return "", fmt.Errorf("%q can't be stored safely on this disk", name)
	}
	return name, nil
}

// planHTTP adds the download of a web address to the items of a transfer.
func (x *netXfer) planHTTP(r *vfs.Router, src, target string, i int, items *[]netItem) error {
	if x.req.Kind != "copy" {
		return errors.New("a web file can only be downloaded (copied), not moved")
	}
	if !netIsLocalPath(r, target) {
		return errors.New("a web file can only be downloaded to a folder on this computer")
	}
	name, err := netHTTPName(src)
	if err != nil {
		return err
	}
	if x.req.Rename != "" && len(x.req.Sources) == 1 {
		name = x.req.Rename
		if !netPlainName(name) || strings.ContainsRune(name, '\\') {
			return fmt.Errorf("%q is not a plain file name", name)
		}
	}
	*items = append(*items, netItem{src: strings.TrimSpace(src), dst: r.Join(target, name), top: i, http: true})
	return nil
}

// httpGet downloads one web address. It reports whether the file is done (or skipped by choice).
func (x *netXfer) httpGet(r *vfs.Router, it netItem) bool {
	name := netBase(it.dst)
	x.mu.Lock()
	x.p.Current = it.src
	base := x.p.BytesDone
	x.mu.Unlock()
	x.emit("net-job", true)
	var offset int64
	for {
		fi, err := os.Lstat(it.dst)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			x.fail(name, err)
			return false
		}
		if fi.IsDir() {
			x.fail(name, errors.New("a folder of that name is in the target folder"))
			return false
		}
		de := vfs.Entry{Name: fi.Name(), Size: fi.Size(), MTime: fi.ModTime().UnixMilli()}
		choice := x.decide(it, de, fi.Size() > 0)
		switch {
		case choice == "overwrite":
		case choice == "resume":
			offset = fi.Size()
		case choice == "skip":
			x.mu.Lock()
			x.p.FilesDone++
			x.mu.Unlock()
			return true
		case choice == "cancel":
			x.cancel()
			return false
		case strings.HasPrefix(choice, "rename:"):
			it.dst = r.Join(r.Parent(it.dst), strings.TrimPrefix(choice, "rename:"))
			name = netBase(it.dst)
			continue
		}
		break
	}
	if err := x.httpFetch(it, offset, base); err != nil {
		if x.ctx.Err() == nil {
			x.fail(name, err)
		}
		return false
	}
	x.mu.Lock()
	x.p.FilesDone++
	x.mu.Unlock()
	return true
}

// httpFetch downloads it.src to it.dst, from offset on when the server sends that range.
func (x *netXfer) httpFetch(it netItem, offset, base int64) error {
	resp, err := httpdl.Get(x.ctx, it.src, offset)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.Complete {
		return nil // the file there is complete already
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if resp.Partial {
		flags = os.O_WRONLY | os.O_APPEND
	} else {
		offset = 0 // no range: the whole file again
	}
	total := int64(-1)
	if resp.Length >= 0 {
		total = offset + resp.Length
		x.mu.Lock()
		x.p.Bytes += total
		x.mu.Unlock()
	}
	f, err := os.OpenFile(it.dst, flags, 0o644)
	if err != nil {
		return err
	}
	done := offset
	buf := make([]byte, 64<<10)
	for {
		nr, rerr := resp.Body.Read(buf)
		if nr > 0 {
			if _, werr := f.Write(buf[:nr]); werr != nil {
				f.Close()
				return werr
			}
			done += int64(nr)
			x.mu.Lock()
			x.p.BytesDone = base + done
			x.mu.Unlock()
			x.emit("net-job", false)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return rerr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if total >= 0 && done < total {
		return fmt.Errorf("the download stopped after %d of %d bytes; copy it again and choose Resume", done, total)
	}
	if !resp.Modified.IsZero() {
		os.Chtimes(it.dst, resp.Modified, resp.Modified)
	}
	return nil
}
