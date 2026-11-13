// This file is the test fixture: a real SSH server listening on 127.0.0.1
// inside the test process. It authenticates with passwords, public keys and
// keyboard-interactive, serves the SFTP subsystem with pkg/sftp's server over
// the test's temp folder, answers keep-alive requests, runs a tiny "echo" /
// "exit" command set, and speaks the server side of SCP ("scp -t" and
// "scp -f") so the SCP client can be tested without any system binary. Every
// test therefore exercises the full network stack without touching anything
// outside t.TempDir() or talking to any outside host.
package sftp

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkgsftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const (
	testUser     = "tester"
	testPassword = "s3cret"
)

type testServer struct {
	t         *testing.T
	ln        net.Listener
	Addr      string
	Port      int
	Root      string
	HostKey   ssh.Signer
	conf      *ssh.ServerConfig
	mu        sync.Mutex
	conns     []net.Conn
	authKeys  []ssh.PublicKey
	extraKeys []ssh.Signer

	// behaviour switches
	noSFTP      bool
	scpEvil     bool     // scp -f sends a "../evil" file name
	scpSend     []string // scp -f sends these top-level file names, content "evil"
	keepalives  atomic.Int32
	logins      atomic.Int32
	kbdOnly     bool   // no plain password method
	otp         string // when set, keyboard-interactive also asks "Verification code:"
	ignoreAlive atomic.Bool
}

type serverOpt func(*testServer)

func withAuthorizedKey(k ssh.PublicKey) serverOpt {
	return func(s *testServer) { s.authKeys = append(s.authKeys, k) }
}
func withNoSFTP() serverOpt              { return func(s *testServer) { s.noSFTP = true } }
func withKbdOnly(otp string) serverOpt   { return func(s *testServer) { s.kbdOnly = true; s.otp = otp } }
func withHostKey(k ssh.Signer) serverOpt { return func(s *testServer) { s.HostKey = k } }
func withExtraHostKey(k ssh.Signer) serverOpt {
	return func(s *testServer) { s.extraKeys = append(s.extraKeys, k) }
}
func withSCPEvil() serverOpt { return func(s *testServer) { s.scpEvil = true } }
func withSCPSend(names ...string) serverOpt {
	return func(s *testServer) { s.scpSend = names }
}

func newSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// newKeyPEM returns a fresh ed25519 private key in OpenSSH format,
// optionally encrypted, plus its public key.
func newKeyPEM(t *testing.T, passphrase string) ([]byte, ssh.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(priv, "test")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "test", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block), sp
}

func newTestServer(t *testing.T, opts ...serverOpt) *testServer {
	t.Helper()
	s := &testServer{t: t, Root: t.TempDir()}
	for _, o := range opts {
		o(s)
	}
	if s.HostKey == nil {
		s.HostKey = newSigner(t)
	}
	conf := &ssh.ServerConfig{
		KeyboardInteractiveCallback: func(c ssh.ConnMetadata, client ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			qs := []string{"Password: "}
			echos := []bool{false}
			if s.otp != "" {
				qs = append(qs, "Verification code: ")
				echos = append(echos, true)
			}
			ans, err := client(c.User(), "login", qs, echos)
			if err != nil {
				return nil, err
			}
			if len(ans) != len(qs) || c.User() != testUser || ans[0] != testPassword {
				return nil, errors.New("denied")
			}
			if s.otp != "" && ans[1] != s.otp {
				return nil, errors.New("bad otp")
			}
			s.logins.Add(1)
			return nil, nil
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			for _, k := range s.authKeys {
				if string(k.Marshal()) == string(key.Marshal()) && c.User() == testUser {
					s.logins.Add(1)
					return nil, nil
				}
			}
			return nil, errors.New("unknown key")
		},
	}
	if !s.kbdOnly {
		conf.PasswordCallback = func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if c.User() == testUser && string(pw) == testPassword {
				s.logins.Add(1)
				return nil, nil
			}
			return nil, errors.New("denied")
		}
	}
	conf.AddHostKey(s.HostKey)
	for _, k := range s.extraKeys {
		conf.AddHostKey(k)
	}
	s.conf = conf

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.ln = ln
	s.Addr = ln.Addr().String()
	s.Port = ln.Addr().(*net.TCPAddr).Port
	go s.serve()
	t.Cleanup(s.Close)
	return s
}

func (s *testServer) Close() {
	s.ln.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		c.Close()
	}
}

// DropConnections closes all live connections (simulates a server crash).
func (s *testServer) DropConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		c.Close()
	}
	s.conns = nil
}

func (s *testServer) serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns = append(s.conns, c)
		s.mu.Unlock()
		go s.handle(c)
	}
}

func (s *testServer) handle(c net.Conn) {
	sc, chans, reqs, err := ssh.NewServerConn(c, s.conf)
	if err != nil {
		c.Close()
		return
	}
	defer sc.Close()
	go func() {
		for r := range reqs {
			if r.Type == "keepalive@openssh.com" {
				s.keepalives.Add(1)
				if s.ignoreAlive.Load() {
					continue // never reply: simulates a hung server
				}
			}
			if r.WantReply {
				r.Reply(false, nil)
			}
		}
	}()
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, creqs, err := nc.Accept()
		if err != nil {
			continue
		}
		go s.session(ch, creqs)
	}
}

func exitStatus(ch ssh.Channel, code int) {
	ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(code)}))
}

func (s *testServer) session(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	for r := range reqs {
		switch r.Type {
		case "subsystem":
			var p struct{ Name string }
			ssh.Unmarshal(r.Payload, &p)
			if p.Name != "sftp" || s.noSFTP {
				r.Reply(false, nil)
				continue
			}
			r.Reply(true, nil)
			srv, err := pkgsftp.NewServer(ch, pkgsftp.WithServerWorkingDirectory(s.Root))
			if err != nil {
				return
			}
			srv.Serve()
			srv.Close()
			return
		case "exec":
			var p struct{ Cmd string }
			ssh.Unmarshal(r.Payload, &p)
			r.Reply(true, nil)
			code := s.exec(ch, p.Cmd)
			exitStatus(ch, code)
			return
		default:
			if r.WantReply {
				r.Reply(false, nil)
			}
		}
	}
}

func (s *testServer) exec(ch ssh.Channel, cmd string) int {
	args := shellSplit(cmd)
	if len(args) == 0 {
		return 127
	}
	switch args[0] {
	case "echo":
		fmt.Fprintln(ch, strings.Join(args[1:], " "))
		return 0
	case "exit":
		n, _ := strconv.Atoi(args[1])
		fmt.Fprintln(ch.Stderr(), "bye")
		return n
	case "sleep":
		time.Sleep(5 * time.Second)
		return 0
	case "scp":
		return s.scp(ch, args[1:])
	}
	fmt.Fprintf(ch.Stderr(), "%s: command not found\n", args[0])
	return 127
}

// shellSplit splits a command line, honouring single quotes (enough for
// what shellQuote produces).
func shellSplit(s string) []string {
	var out []string
	var cur strings.Builder
	inQ, have := false, false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case inQ && ch == '\'':
			inQ = false
		case inQ:
			cur.WriteByte(ch)
		case ch == '\'':
			inQ, have = true, true
		case ch == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
			have = true
		case ch == ' ':
			if have {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteByte(ch)
			have = true
		}
	}
	if have {
		out = append(out, cur.String())
	}
	return out
}

// ---- server side of SCP -------------------------------------------------

func (s *testServer) scp(ch ssh.Channel, args []string) int {
	var sink, source, preserve bool
	var target string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-t":
			sink = true
		case "-f":
			source = true
		case "-r", "-d":
		case "-p":
			preserve = true
		case "--":
			if i+1 < len(args) {
				target = args[i+1]
			}
			i = len(args)
		default:
			target = args[i]
		}
	}
	br := bufio.NewReader(ch)
	if sink {
		return scpSink(ch, br, target)
	}
	if source {
		return s.scpSource(ch, br, target, preserve)
	}
	return 1
}

func scpSink(ch ssh.Channel, br *bufio.Reader, target string) int {
	ok := func() { ch.Write([]byte{0}) }
	ok()
	targetIsDir := false
	if fi, err := os.Stat(target); err == nil && fi.IsDir() {
		targetIsDir = true
	}
	var stack []string
	var mt time.Time
	haveT := false
	dest := func(name string) string {
		if len(stack) > 0 {
			return filepath.Join(stack[len(stack)-1], name)
		}
		if targetIsDir {
			return filepath.Join(target, name)
		}
		return target
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return 0
		}
		line = strings.TrimSuffix(line, "\n")
		switch line[0] {
		case 'T':
			var a, b, c, d int64
			fmt.Sscanf(line, "T%d %d %d %d", &a, &b, &c, &d)
			mt, haveT = time.Unix(a, 0), true
			ok()
		case 'D':
			mode, _, name, err := parseSCPHeader(line)
			if err != nil {
				fmt.Fprintf(ch, "\x02%v\n", err)
				return 1
			}
			d := dest(name)
			os.MkdirAll(d, 0o755)
			os.Chmod(d, mode|0o700)
			stack = append(stack, d)
			haveT = false
			ok()
		case 'E':
			stack = stack[:len(stack)-1]
			ok()
		case 'C':
			mode, size, name, err := parseSCPHeader(line)
			if err != nil {
				fmt.Fprintf(ch, "\x02%v\n", err)
				return 1
			}
			d := dest(name)
			f, err := os.OpenFile(d, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
			if err != nil {
				fmt.Fprintf(ch, "\x01scp: %s: %v\n", d, err)
				io.CopyN(io.Discard, br, size+1)
				continue
			}
			ok()
			io.CopyN(f, br, size)
			f.Close()
			os.Chmod(d, mode)
			if haveT {
				os.Chtimes(d, mt, mt)
				haveT = false
			}
			br.ReadByte()
			ok()
		default:
			fmt.Fprintf(ch, "\x02bad line\n")
			return 1
		}
	}
}

func (s *testServer) scpSource(ch ssh.Channel, br *bufio.Reader, p string, preserve bool) int {
	readAck := func() bool { b, err := br.ReadByte(); return err == nil && b == 0 }
	if !readAck() {
		return 1
	}
	if s.scpEvil {
		fmt.Fprintf(ch, "C0644 4 ../evil\n")
		if !readAck() {
			return 1
		}
		ch.Write([]byte("evil\x00"))
		readAck()
		return 0
	}
	if len(s.scpSend) > 0 {
		for _, n := range s.scpSend {
			fmt.Fprintf(ch, "C0644 4 %s\n", n)
			if !readAck() {
				return 1
			}
			ch.Write([]byte("evil\x00"))
			if !readAck() {
				return 1
			}
		}
		return 0
	}
	fi, err := os.Stat(p)
	if err != nil {
		fmt.Fprintf(ch, "\x01scp: %s: No such file or directory\n", p)
		return 1
	}
	var send func(p string, fi fs.FileInfo) bool
	send = func(p string, fi fs.FileInfo) bool {
		if preserve {
			fmt.Fprintf(ch, "T%d 0 %d 0\n", fi.ModTime().Unix(), fi.ModTime().Unix())
			if !readAck() {
				return false
			}
		}
		if fi.IsDir() {
			fmt.Fprintf(ch, "D%04o 0 %s\n", fi.Mode().Perm(), fi.Name())
			if !readAck() {
				return false
			}
			ents, _ := os.ReadDir(p)
			for _, e := range ents {
				cfi, err := os.Stat(filepath.Join(p, e.Name()))
				if err != nil {
					continue
				}
				if !send(filepath.Join(p, e.Name()), cfi) {
					return false
				}
			}
			fmt.Fprintf(ch, "E\n")
			return readAck()
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return false
		}
		fmt.Fprintf(ch, "C%04o %d %s\n", fi.Mode().Perm(), len(data), fi.Name())
		if !readAck() {
			return false
		}
		ch.Write(data)
		ch.Write([]byte{0})
		return readAck()
	}
	if !send(p, fi) {
		return 1
	}
	return 0
}

// ---- client helpers ------------------------------------------------------

// baseConfig is a password login to s with the server key pre-trusted in a
// temp known_hosts file.
func (s *testServer) baseConfig(t *testing.T) Config {
	t.Helper()
	kh := filepath.Join(t.TempDir(), "known_hosts")
	if err := AddKnownHost(kh, s.Addr, s.HostKey.PublicKey(), false); err != nil {
		t.Fatal(err)
	}
	return Config{
		Host:            "127.0.0.1",
		Port:            s.Port,
		User:            testUser,
		Password:        testPassword,
		KnownHostsFiles: []string{kh},
		Timeout:         10 * time.Second,
	}
}

func (s *testServer) dial(t *testing.T) *Client {
	t.Helper()
	c, err := Dial(t.Context(), s.baseConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
