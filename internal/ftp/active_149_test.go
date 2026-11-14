package ftp

import (
	"context"
	"net"
	"testing"
	"time"
)

// In active mode a connection from another address than the server's is held back: when the server connects after
// it, the server's connection is used (port stealing, #149); when only the other one comes, it is used after the
// grace time (servers behind NAT or with several addresses keep working).
func TestAcceptActivePrefersServerAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	stranger := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.2")}}
	sc, err := stranger.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Skip("no second loopback address here:", err)
	}
	defer sc.Close()
	go func() {
		time.Sleep(200 * time.Millisecond)
		if c, err := net.Dial("tcp", ln.Addr().String()); err == nil {
			defer c.Close()
			time.Sleep(2 * time.Second)
		}
	}()
	conn, err := acceptActive(context.Background(), ln, net.ParseIP("127.0.0.1"), 5*time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if ip := conn.RemoteAddr().(*net.TCPAddr).IP; !ip.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("took the connection from %v, want the server's 127.0.0.1", ip)
	}
}

func TestAcceptActiveFallsBackAfterGrace(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	start := time.Now()
	conn, err := acceptActive(context.Background(), ln, net.ParseIP("192.0.2.1"), 5*time.Second, 300*time.Millisecond)
	if err != nil {
		t.Fatalf("a server connecting from another address was refused: %v", err)
	}
	conn.Close()
	if d := time.Since(start); d < 250*time.Millisecond || d > 3*time.Second {
		t.Fatalf("fallback after %v, want about the grace time", d)
	}
}
