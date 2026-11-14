package webdav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"mormicommander/internal/remotefs"
)

// davListing answers PROPFIND with a multistatus that holds one response per href; hrefs ending in "/" are folders.
func davListing(w http.ResponseWriter, hrefs ...string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusMultiStatus)
	body := `<?xml version="1.0"?><D:multistatus xmlns:D="DAV:">`
	for _, h := range hrefs {
		rt := ""
		if h[len(h)-1] == '/' {
			rt = "<D:collection/>"
		}
		body += `<D:response><D:href>` + h + `</D:href><D:propstat><D:prop><D:resourcetype>` + rt +
			`</D:resourcetype><D:getcontentlength>10</D:getcontentlength></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>`
	}
	w.Write([]byte(body + `</D:multistatus>`))
}

// TestReadDirKeepsOnlyChildren: a Depth 1 answer that also names the share root or an item of another folder must not
// show up in the listing of /dir. An href of the root came out as an entry named "/", which a folder copy then entered
// again and again (the listing of /dir held /dir's parent).
func TestReadDirKeepsOnlyChildren(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		davListing(w, "/dir/", "/dir/a.txt", "/", "/other/b.txt", "/dir/sub/deep.txt", "/dir/../")
	}))
	defer srv.Close()
	c, err := New(Config{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	es, err := c.ReadDir("/dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 1 || es[0].Name != "a.txt" || es[0].Path != "/dir/a.txt" {
		t.Fatalf("ReadDir(/dir) = %+v, want only a.txt", es)
	}
}

// TestDownloadResumeChecksContentRange: a server that answers a Range request with 206 but sends the file from its
// start (Content-Range bytes 0-...) must not have that appended to the partial local file, which turned a good partial
// copy into a larger, broken one.
func TestDownloadResumeChecksContentRange(t *testing.T) {
	const content = "0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			davListing(w, "/f.bin")
		case http.MethodGet:
			w.Header().Set("Content-Range", "bytes 0-9/10")
			w.WriteHeader(http.StatusPartialContent)
			w.Write([]byte(content))
		}
	}))
	defer srv.Close()
	c, err := New(Config{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(t.TempDir(), "f.bin")
	os.WriteFile(local, []byte(content[:4]), 0o644)
	_, err = c.Download(context.Background(), "/f.bin", local, remotefs.TransferOptions{Resume: true})
	got, _ := os.ReadFile(local)
	if err == nil && string(got) == content {
		return // started over: fine
	}
	if string(got) != content[:4] && string(got) != content {
		t.Fatalf("local file is now %q (err %v): the wrong range was appended", got, err)
	}
}
