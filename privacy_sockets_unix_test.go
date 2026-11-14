//go:build darwin || linux

// The socket sampler of the run-time zero-network test (privacy_session_test.go), for macOS and Linux. It asks each
// file descriptor of the test process for its socket address every couple of milliseconds; a descriptor that is an
// internet (IPv4 or IPv6) socket and was not there when the watch started is written down with its local and remote
// address, and marked "[loopback]" when it only talks to 127.0.0.1 or ::1 (which never leaves the machine; the session
// test lets those pass, so a stray loopback server of another test in the package cannot make it fail). Unix-domain
// sockets are ignored for the same reason. A connection that opens and closes between
// two samples can slip through; the recording HTTP transport and resolver, and privacy_test.go's reading of the
// source, cover that gap.
package main

import (
	"fmt"
	"net"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// socketsWatched says that this system samples sockets (the watch test checks the sampler only where it runs).
const socketsWatched = true

// maxWatchedFD is how many descriptors are looked at; a test process uses only a handful.
const maxWatchedFD = 1024

// inetSockets returns the open internet sockets of the process, by descriptor, described as "local -> remote".
func inetSockets() map[int]string {
	out := map[int]string{}
	for fd := 0; fd < maxWatchedFD; fd++ {
		sa, err := syscall.Getsockname(fd)
		if err != nil {
			continue
		}
		local := sockaddrString(sa)
		if local == "" {
			continue // not an internet socket
		}
		remote := "(not connected)"
		if pa, err := syscall.Getpeername(fd); err == nil {
			if s := sockaddrString(pa); s != "" {
				remote = s
			}
		}
		desc := local + " -> " + remote
		if loopbackOnly(sa) && (remote == "(not connected)" || loopbackPeer(fd)) {
			desc += " [loopback]"
		}
		out[fd] = desc
	}
	return out
}

// loopbackOnly says whether a socket address is 127.0.0.0/8 or ::1.
func loopbackOnly(sa syscall.Sockaddr) bool {
	switch a := sa.(type) {
	case *syscall.SockaddrInet4:
		return net.IP(a.Addr[:]).IsLoopback()
	case *syscall.SockaddrInet6:
		return net.IP(a.Addr[:]).IsLoopback()
	}
	return false
}

// loopbackPeer says whether the descriptor's peer is a loopback address.
func loopbackPeer(fd int) bool {
	pa, err := syscall.Getpeername(fd)
	return err == nil && loopbackOnly(pa)
}

func sockaddrString(sa syscall.Sockaddr) string {
	switch a := sa.(type) {
	case *syscall.SockaddrInet4:
		return net.JoinHostPort(net.IP(a.Addr[:]).String(), strconv.Itoa(a.Port))
	case *syscall.SockaddrInet6:
		return net.JoinHostPort(net.IP(a.Addr[:]).String(), strconv.Itoa(a.Port))
	}
	return ""
}

// watchSockets samples the sockets until the returned function is called, recording each new one once.
func watchSockets(w *netWatch) func() {
	before := inetSockets()
	seen := map[string]bool{}
	done := make(chan struct{})
	finished := make(chan struct{})
	sample := func() {
		for fd, desc := range inetSockets() {
			if before[fd] == desc {
				continue
			}
			key := fmt.Sprintf("%d %s", fd, desc)
			if !seen[key] {
				seen[key] = true
				w.add("internet socket opened (fd %d): %s", fd, desc)
			}
		}
	}
	go func() {
		defer close(finished)
		tick := time.NewTicker(2 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				sample()
				return
			case <-tick.C:
				sample()
			}
		}
	}()
	return func() {
		close(done)
		<-finished
	}
}

// Only 127.0.0.0/8 and ::1 count as loopback; any other address is reported as outbound.
func TestZeroNetworkLoopbackMark(t *testing.T) {
	cases := map[syscall.Sockaddr]bool{
		&syscall.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}:             true,
		&syscall.SockaddrInet4{Addr: [4]byte{127, 1, 2, 3}}:             true,
		&syscall.SockaddrInet4{Addr: [4]byte{192, 168, 1, 1}}:           false,
		&syscall.SockaddrInet4{Addr: [4]byte{93, 184, 216, 34}}:         false,
		&syscall.SockaddrInet6{Addr: [16]byte{15: 1}}:                   true,
		&syscall.SockaddrInet6{Addr: [16]byte{0: 0x20, 1: 0x01, 15: 1}}: false,
		&syscall.SockaddrUnix{Name: "/tmp/x.sock"}:                      false,
	}
	for sa, want := range cases {
		if got := loopbackOnly(sa); got != want {
			t.Errorf("loopbackOnly(%s) = %v, want %v", sockaddrString(sa), got, want)
		}
	}
}
