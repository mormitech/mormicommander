package webdav

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFolderWithoutSlashFollowsApacheRedirect plays Apache mod_dav: a PROPFIND for a folder named without its trailing
// slash gets "301 Moved Permanently" to the address with one. The client must send the PROPFIND again to that address
// (not turn it into a GET, as Go's own redirect handling does, which answered "200 OK" with a web page; issue #106).
func TestFolderWithoutSlashFollowsApacheRedirect(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/dav":
			http.Redirect(w, r, "/dav/", http.StatusMovedPermanently)
		case r.URL.Path == "/dav/" && r.Method == "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			w.Write([]byte(`<?xml version="1.0"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>/dav/</D:href>` +
				`<D:propstat><D:prop><D:resourcetype><D:collection/></D:resourcetype></D:prop>` +
				`<D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response></D:multistatus>`))
		default:
			w.Write([]byte("<html>a web page</html>"))
		}
	}))
	defer srv.Close()
	c, err := New(Config{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	e, err := c.Stat("/dav")
	if err != nil {
		t.Fatalf("Stat /dav: %v (requests %v)", err, methods)
	}
	if !e.IsDir {
		t.Errorf("entry %+v is not a folder", e)
	}
	if len(methods) != 2 || methods[0] != "PROPFIND /dav" || methods[1] != "PROPFIND /dav/" {
		t.Errorf("requests %v, want PROPFIND /dav then PROPFIND /dav/", methods)
	}
}

// TestOtherRedirectsAreNotFollowed: a redirect somewhere else is not followed (and so never turns into a GET).
func TestOtherRedirectsAreNotFollowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere/", http.StatusFound)
	}))
	defer srv.Close()
	c, _ := New(Config{URL: srv.URL})
	if _, err := c.Stat("/dav"); err == nil {
		t.Fatal("a redirect to another place was taken as an answer")
	}
}
