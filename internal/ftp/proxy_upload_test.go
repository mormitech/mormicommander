package ftp

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestUploadThroughHTTPProxyArrivesWhole is TestUploadWithUnreadServerDataArrivesWhole through an HTTP CONNECT
// tunnel. The tunnel's connection is wrapped (to give back bytes read behind the proxy's "200"), and the wrapper hid
// the TCP half-close, so the upload's end was a plain close of a socket with unread data: a reset, and the server lost
// the part of the file it had not read yet.
func TestUploadThroughHTTPProxyArrivesWhole(t *testing.T) {
	srv := newTestServer(t, func(s *testServer) {
		s.storSends = true
		s.storDelay = 200 * time.Millisecond
	})
	px := newTestProxy(t, ProxyHTTP, "", "")
	cfg := srv.cfg()
	cfg.Proxy = &Proxy{Type: ProxyHTTP, Host: "127.0.0.1", Port: px.port()}
	c := dialT(t, cfg)
	data := make([]byte, 3<<20+11)
	rand.Read(data)
	src := filepath.Join(t.TempDir(), "big.bin")
	os.WriteFile(src, data, 0o644)
	n, err := c.Upload(context.Background(), src, "big.bin", TransferOptions{Mode: TransferBinary})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if got := []byte(srv.read("big.bin")); n != int64(len(data)) || !bytes.Equal(got, data) {
		t.Fatalf("%d bytes sent, %d arrived of %d", n, len(got), len(data))
	}
}
