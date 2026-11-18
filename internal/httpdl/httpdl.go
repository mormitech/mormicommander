// Package httpdl fetches one file over HTTP or HTTPS for the network feature's "download a web address" (FEATURES.md
// F-644, TC: a URL typed in FTP connect). It only runs when the user types an http:// or https:// address in Net >
// New FTP connection and starts the download; nothing here runs by itself. Redirects are followed to http and https
// addresses only, and a range request continues a shorter file that is already there.
package httpdl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Response is an answer to Get.
type Response struct {
	Body     io.ReadCloser
	Partial  bool      // the server sent the range from the asked offset on (else the whole file)
	Complete bool      // the asked offset is at the end already (nothing more to fetch; Body is empty)
	Length   int64     // bytes in Body, -1 when unknown
	Modified time.Time // the Last-Modified time, zero when the server sends none
}

var client = &http.Client{
	Transport: func() http.RoundTripper {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.ResponseHeaderTimeout = time.Minute
		return t
	}(),
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("redirected to an address that is not http or https")
		}
		return nil
	},
}

// IsURL says whether s is an http:// or https:// address with a host.
func IsURL(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Get asks for the file at raw, from offset on when offset > 0. Any answer other than the file (or its range) is an
// error that names the server's status.
func Get(ctx context.Context, raw string, offset int64) (*Response, error) {
	if !IsURL(raw) {
		return nil, errors.New("this is not an http or https address")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(raw), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mormiCommander")
	if offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	out := &Response{Body: resp.Body, Length: resp.ContentLength}
	if t, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		out.Modified = t
	}
	switch {
	case offset > 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		resp.Body.Close()
		out.Body, out.Complete, out.Length = io.NopCloser(strings.NewReader("")), true, 0
	case offset > 0 && resp.StatusCode == http.StatusPartialContent:
		out.Partial = true
	case resp.StatusCode == http.StatusOK:
	default:
		resp.Body.Close()
		return nil, fmt.Errorf("the server answered %s", resp.Status)
	}
	return out, nil
}
