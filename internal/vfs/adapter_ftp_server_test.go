// This file is a tiny FTP server used only by the tests. It listens on
// 127.0.0.1, serves a temporary folder, and implements just enough of the
// protocol to exercise every code path of the client: plain, explicit and
// implicit TLS, passive (PASV/EPSV) and active (PORT/EPRT) data connections,
// MLSD/MLST, LIST in UNIX or DOS style or from canned text, REST, APPE,
// ASCII conversion, SITE CHMOD and SITE CPFR/CPTO, and the USER user@host
// login of FTP proxies. Switches on the server turn features off so the
// client's fallbacks can be tested too. Every command is logged so tests
// can assert what the client actually sent.
//
// It is a copy of internal/ftp/server_test.go (identifiers renamed to fit this package,
// and the few unexported engine helpers it needs copied at the end), because
// test files of another package cannot be imported. The VFS adapter tests run
// the real client stack against it.
package vfs

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"mormicommander/internal/ftp"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type ftpServer struct {
	t    *testing.T
	root string
	ln   net.Listener
	port int

	user, pass string // empty user: anonymous only
	tlsConf    *tls.Config
	implicit   bool
	requireTLS bool

	listStyle   string            // "unix" (default) or "dos"
	canned      map[string]string // virtual dir -> raw LIST text
	noMLSD      bool
	noEPSV      bool
	noFeat      bool
	noREST      bool
	noMFMT      bool
	noCopy      bool
	utf8        bool
	pasvIP      string // address announced in PASV replies
	proxyAtHost bool   // expect "USER name@host"
	proxyOpen   bool   // expect "OPEN host" before USER
	latin1Names bool   // speak Latin-1 on the wire
	greeting    string

	mu   sync.Mutex
	cmds []string
	wg   sync.WaitGroup
}

func newFTPServer(t *testing.T, opts ...func(*ftpServer)) *ftpServer {
	t.Helper()
	s := &ftpServer{t: t, root: t.TempDir(), utf8: true, listStyle: "unix"}
	for _, o := range opts {
		o(s)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if s.implicit {
		ln = tls.NewListener(ln, s.tlsConf)
	}
	s.ln = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.wg.Add(1)
	go s.serve()
	t.Cleanup(func() {
		ln.Close()
		s.wg.Wait()
	})
	return s
}

func (s *ftpServer) cfg() ftp.Config {
	return ftp.Config{Host: "127.0.0.1", Port: s.port, Timeout: 5 * time.Second, ServerLocation: time.UTC}
}

func (s *ftpServer) log(cmd string) {
	s.mu.Lock()
	s.cmds = append(s.cmds, cmd)
	s.mu.Unlock()
}

// sent reports how many logged commands start with prefix.
func (s *ftpServer) sent(prefix string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.cmds {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (s *ftpServer) commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.cmds...)
}

func (s *ftpServer) write(rel, content string) {
	s.t.Helper()
	p := filepath.Join(s.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

func (s *ftpServer) read(rel string) string {
	s.t.Helper()
	b, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(rel)))
	if err != nil {
		s.t.Fatal(err)
	}
	return string(b)
}

func (s *ftpServer) exists(rel string) bool {
	_, err := os.Lstat(filepath.Join(s.root, filepath.FromSlash(rel)))
	return err == nil
}

func (s *ftpServer) serve() {
	defer s.wg.Done()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			sess := &ftpSession{srv: s, cwd: "/", typ: 'A'}
			sess.run(c)
		}()
	}
}

type ftpSession struct {
	srv      *ftpServer
	conn     net.Conn
	r        *bufio.Reader
	w        *bufio.Writer
	cwd      string
	typ      byte
	user     string
	authed   bool
	secure   bool
	prot     bool
	rest     int64
	rnfr     string
	cpfr     string
	pasv     net.Listener
	activeTo string
	opened   string
}

func (ss *ftpSession) reply(code int, msg string) {
	lines := strings.Split(msg, "\n")
	for i, l := range lines {
		if i < len(lines)-1 {
			fmt.Fprintf(ss.w, "%d-%s\r\n", code, l)
		} else {
			fmt.Fprintf(ss.w, "%d %s\r\n", code, l)
		}
	}
	ss.w.Flush()
}

func (ss *ftpSession) setConn(c net.Conn) {
	ss.conn = c
	ss.r = bufio.NewReader(c)
	ss.w = bufio.NewWriter(c)
}

func (ss *ftpSession) run(c net.Conn) {
	defer c.Close()
	ss.setConn(c)
	ss.secure = ss.srv.implicit
	defer func() {
		if ss.pasv != nil {
			ss.pasv.Close()
		}
	}()
	g := ss.srv.greeting
	if g == "" {
		g = "mormi test FTP server ready"
	}
	ss.reply(220, g)
	for {
		ss.conn.SetDeadline(time.Now().Add(20 * time.Second))
		line, err := ss.r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if ss.srv.latin1Names {
			line = ftpTestFromLatin1(line) // wire bytes are Latin-1, disk names UTF-8
		}
		verb, arg, _ := strings.Cut(line, " ")
		verb = strings.ToUpper(verb)
		ss.srv.log(strings.TrimSpace(verb + " " + arg))
		if !ss.handle(verb, arg) {
			return
		}
	}
}

// vpath resolves a client path to a cleaned virtual path.
func (ss *ftpSession) vpath(arg string) string {
	if arg == "" {
		return ss.cwd
	}
	if strings.HasPrefix(arg, "/") {
		return path.Clean(arg)
	}
	return path.Clean(path.Join(ss.cwd, arg))
}

func (ss *ftpSession) real(v string) string {
	return filepath.Join(ss.srv.root, filepath.FromSlash(strings.TrimPrefix(v, "/")))
}

func (ss *ftpSession) handle(verb, arg string) bool {
	s := ss.srv
	if !ss.authed {
		switch verb {
		case "USER", "PASS", "AUTH", "QUIT", "FEAT", "PBSZ", "PROT", "SYST", "NOOP", "OPTS", "OPEN":
		default:
			ss.reply(530, "Please login with USER and PASS.")
			return true
		}
	}
	switch verb {
	case "AUTH":
		if s.tlsConf == nil || ss.secure {
			ss.reply(502, "TLS not available")
			return true
		}
		ss.reply(234, "AUTH TLS ok")
		tc := tls.Server(ss.conn, s.tlsConf)
		if err := tc.Handshake(); err != nil {
			return false
		}
		ss.setConn(tc)
		ss.secure = true
	case "PBSZ":
		ss.reply(200, "PBSZ=0")
	case "OPEN": // FTP proxy "OPEN host" firewall
		if !s.proxyOpen {
			ss.reply(502, "no OPEN")
			return true
		}
		ss.opened = arg
		ss.reply(220, "connected to "+arg)
	case "PROT":
		ss.prot = strings.ToUpper(arg) == "P"
		ss.reply(200, "Protection level set")
	case "USER":
		if s.requireTLS && !ss.secure {
			ss.reply(530, "TLS required")
			return true
		}
		u := arg
		if s.proxyOpen && ss.opened == "" {
			ss.reply(530, "OPEN a host first")
			return true
		}
		if s.proxyAtHost {
			name, host, ok := strings.Cut(arg, "@")
			if !ok || host == "" {
				ss.reply(530, "proxy needs user@host")
				return true
			}
			u = name
		}
		ss.user = u
		ss.reply(331, "Password required")
	case "PASS":
		anon := ss.user == "anonymous" || ss.user == "ftp"
		if s.user == "" && anon || s.user != "" && ss.user == s.user && arg == s.pass {
			ss.authed = true
			ss.reply(230, "Logged in")
		} else {
			ss.reply(530, "Login incorrect")
		}
	case "SYST":
		ss.reply(215, "UNIX Type: L8")
	case "FEAT":
		if s.noFeat {
			ss.reply(502, "no FEAT")
			return true
		}
		feats := []string{"Features:", " MDTM", " SIZE", " PASV"}
		if !s.noEPSV {
			feats = append(feats, " EPSV")
		}
		if !s.noMLSD {
			feats = append(feats, " MLST type*;size*;modify*;perm*;unix.mode*;")
		}
		if s.utf8 {
			feats = append(feats, " UTF8")
		}
		if !s.noREST {
			feats = append(feats, " REST STREAM")
		}
		if !s.noMFMT {
			feats = append(feats, " MFMT")
		}
		if s.tlsConf != nil {
			feats = append(feats, " AUTH TLS", " PBSZ", " PROT")
		}
		feats = append(feats, "End")
		ss.reply(211, strings.Join(feats, "\n"))
	case "OPTS":
		ss.reply(200, "OK")
	case "NOOP":
		ss.reply(200, "NOOP ok")
	case "QUIT":
		ss.reply(221, "Bye")
		return false
	case "PWD", "XPWD":
		ss.reply(257, `"`+strings.ReplaceAll(ss.cwd, `"`, `""`)+`" is the current directory`)
	case "CWD", "XCWD":
		v := ss.vpath(arg)
		if fi, err := os.Stat(ss.real(v)); err != nil || !fi.IsDir() {
			ss.reply(550, "No such directory")
			return true
		}
		ss.cwd = v
		ss.reply(250, "CWD ok")
	case "CDUP", "XCUP":
		ss.cwd = path.Dir(ss.cwd)
		ss.reply(250, "CDUP ok")
	case "TYPE":
		switch strings.ToUpper(strings.Fields(arg + " ")[0]) {
		case "A":
			ss.typ = 'A'
		case "I", "L":
			ss.typ = 'I'
		default:
			ss.reply(504, "bad type")
			return true
		}
		ss.reply(200, "Type set")
	case "MODE", "STRU":
		ss.reply(200, "OK")
	case "PASV":
		ss.openPasv()
		if ss.pasv == nil {
			ss.reply(425, "cannot listen")
			return true
		}
		port := ss.pasv.Addr().(*net.TCPAddr).Port
		ip := "127,0,0,1"
		if s.pasvIP != "" {
			ip = strings.ReplaceAll(s.pasvIP, ".", ",")
		}
		ss.reply(227, fmt.Sprintf("Entering Passive Mode (%s,%d,%d).", ip, port>>8, port&0xff))
	case "EPSV":
		if s.noEPSV {
			ss.reply(502, "EPSV not supported")
			return true
		}
		ss.openPasv()
		if ss.pasv == nil {
			ss.reply(425, "cannot listen")
			return true
		}
		ss.reply(229, fmt.Sprintf("Entering Extended Passive Mode (|||%d|)", ss.pasv.Addr().(*net.TCPAddr).Port))
	case "PORT":
		parts := strings.Split(arg, ",")
		if len(parts) != 6 {
			ss.reply(501, "bad PORT")
			return true
		}
		p1, _ := strconv.Atoi(parts[4])
		p2, _ := strconv.Atoi(parts[5])
		ss.activeTo = net.JoinHostPort(strings.Join(parts[:4], "."), strconv.Itoa(p1<<8|p2))
		ss.closePasv()
		ss.reply(200, "PORT ok")
	case "EPRT":
		f := strings.Split(arg, arg[:1])
		if len(f) < 5 {
			ss.reply(501, "bad EPRT")
			return true
		}
		ss.activeTo = net.JoinHostPort(f[2], f[3])
		ss.closePasv()
		ss.reply(200, "EPRT ok")
	case "REST":
		if s.noREST {
			ss.reply(502, "REST not supported")
			return true
		}
		n, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			ss.reply(501, "bad REST")
			return true
		}
		ss.rest = n
		ss.reply(350, "Restarting")
	case "RETR":
		ss.retr(arg)
	case "STOR", "APPE":
		ss.stor(arg, verb == "APPE")
	case "LIST", "NLST", "MLSD":
		ss.list(verb, arg)
	case "MLST":
		if s.noMLSD {
			ss.reply(502, "no MLST")
			return true
		}
		v := ss.vpath(arg)
		fi, err := os.Lstat(ss.real(v))
		if err != nil {
			ss.reply(550, "Not found")
			return true
		}
		ss.reply(250, "Listing "+v+"\n "+ftpMLSDFacts(fi)+" "+v+"\nEnd")
	case "SIZE":
		fi, err := os.Stat(ss.real(ss.vpath(arg)))
		if err != nil || fi.IsDir() {
			ss.reply(550, "Not a plain file")
			return true
		}
		ss.reply(213, strconv.FormatInt(fi.Size(), 10))
	case "MDTM":
		fields := strings.SplitN(arg, " ", 2)
		if len(fields) == 2 && len(fields[0]) == 14 && ftpTestIsDigits(fields[0]) {
			ss.setTime(fields[0], fields[1])
			return true
		}
		fi, err := os.Stat(ss.real(ss.vpath(arg)))
		if err != nil {
			ss.reply(550, "Not found")
			return true
		}
		ss.reply(213, fi.ModTime().UTC().Format("20060102150405"))
	case "MFMT":
		if s.noMFMT {
			ss.reply(502, "no MFMT")
			return true
		}
		stamp, p, _ := strings.Cut(arg, " ")
		ss.setTime(stamp, p)
	case "DELE":
		v := ss.real(ss.vpath(arg))
		if fi, err := os.Lstat(v); err != nil || fi.IsDir() {
			ss.reply(550, "Not a file")
			return true
		}
		os.Remove(v)
		ss.reply(250, "Deleted")
	case "RMD", "XRMD":
		v := ss.real(ss.vpath(arg))
		if err := os.Remove(v); err != nil {
			ss.reply(550, "Cannot remove")
			return true
		}
		ss.reply(250, "Removed")
	case "MKD", "XMKD":
		v := ss.vpath(arg)
		if err := os.Mkdir(ss.real(v), 0o755); err != nil {
			ss.reply(550, "Cannot create")
			return true
		}
		ss.reply(257, `"`+v+`" created`)
	case "RNFR":
		v := ss.vpath(arg)
		if _, err := os.Lstat(ss.real(v)); err != nil {
			ss.reply(550, "Not found")
			return true
		}
		ss.rnfr = v
		ss.reply(350, "Ready for RNTO")
	case "RNTO":
		if ss.rnfr == "" {
			ss.reply(503, "RNFR first")
			return true
		}
		err := os.Rename(ss.real(ss.rnfr), ss.real(ss.vpath(arg)))
		ss.rnfr = ""
		if err != nil {
			ss.reply(550, "Rename failed")
			return true
		}
		ss.reply(250, "Renamed")
	case "SITE":
		sub, rest, _ := strings.Cut(arg, " ")
		switch strings.ToUpper(sub) {
		case "CHMOD":
			modeText, p, _ := strings.Cut(rest, " ")
			m, err := strconv.ParseUint(modeText, 8, 32)
			if err != nil {
				ss.reply(501, "bad mode")
				return true
			}
			if err := os.Chmod(ss.real(ss.vpath(p)), fs.FileMode(m&0o777)); err != nil {
				ss.reply(550, "chmod failed")
				return true
			}
			ss.reply(200, "SITE CHMOD ok")
		case "CPFR":
			if s.noCopy {
				ss.reply(500, "unknown SITE command")
				return true
			}
			ss.cpfr = ss.vpath(rest)
			ss.reply(350, "Ready for CPTO")
		case "CPTO":
			b, err := os.ReadFile(ss.real(ss.cpfr))
			if err == nil {
				err = os.WriteFile(ss.real(ss.vpath(rest)), b, 0o644)
			}
			if err != nil {
				ss.reply(550, "copy failed")
				return true
			}
			ss.reply(250, "Copied")
		default:
			ss.reply(500, "unknown SITE command")
		}
	default:
		ss.reply(502, "Command not implemented")
	}
	return true
}

func (ss *ftpSession) setTime(stamp, p string) {
	t, err := time.ParseInLocation("20060102150405", stamp, time.UTC)
	if err != nil {
		ss.reply(501, "bad time")
		return
	}
	if err := os.Chtimes(ss.real(ss.vpath(p)), t, t); err != nil {
		ss.reply(550, "not found")
		return
	}
	ss.reply(213, "Modify="+stamp+"; "+p)
}

func (ss *ftpSession) openPasv() {
	ss.closePasv()
	ss.activeTo = ""
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		ss.pasv = ln
	}
}

func (ss *ftpSession) closePasv() {
	if ss.pasv != nil {
		ss.pasv.Close()
		ss.pasv = nil
	}
}

// dataConn opens the data connection after the 150 reply.
func (ss *ftpSession) dataConn() (net.Conn, bool) {
	var c net.Conn
	var err error
	switch {
	case ss.pasv != nil:
		ss.pasv.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
		c, err = ss.pasv.Accept()
		ss.closePasv()
	case ss.activeTo != "":
		c, err = net.DialTimeout("tcp", ss.activeTo, 5*time.Second)
		ss.activeTo = ""
	default:
		ss.reply(425, "Use PORT or PASV first")
		return nil, false
	}
	if err != nil {
		ss.reply(425, "Cannot open data connection")
		return nil, false
	}
	ss.reply(150, "Opening data connection")
	if ss.prot {
		tc := tls.Server(c, ss.srv.tlsConf)
		tc.SetDeadline(time.Now().Add(5 * time.Second))
		if err := tc.Handshake(); err != nil {
			c.Close()
			ss.reply(425, "TLS failed")
			return nil, false
		}
		tc.SetDeadline(time.Time{})
		c = tc
	}
	return c, true
}

func (ss *ftpSession) retr(arg string) {
	rest := ss.rest
	ss.rest = 0
	f, err := os.Open(ss.real(ss.vpath(arg)))
	if err != nil {
		ss.closePasv()
		ss.reply(550, "File not found")
		return
	}
	defer f.Close()
	if fi, _ := f.Stat(); fi.IsDir() {
		ss.closePasv()
		ss.reply(550, "Is a directory")
		return
	}
	c, ok := ss.dataConn()
	if !ok {
		return
	}
	f.Seek(rest, io.SeekStart)
	var src io.Reader = f
	if ss.typ == 'A' {
		b, _ := io.ReadAll(f)
		src = strings.NewReader(strings.ReplaceAll(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n", "\r\n"))
	}
	_, err = io.Copy(c, src)
	c.Close()
	if err != nil {
		ss.reply(426, "Connection closed; transfer aborted")
		return
	}
	ss.reply(226, "Transfer complete")
}

func (ss *ftpSession) stor(arg string, appendMode bool) {
	rest := ss.rest
	ss.rest = 0
	p := ss.real(ss.vpath(arg))
	flags := os.O_CREATE | os.O_WRONLY
	switch {
	case appendMode:
		flags |= os.O_APPEND
	case rest == 0:
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(p, flags, 0o644)
	if err != nil {
		ss.closePasv()
		ss.reply(553, "Cannot create file")
		return
	}
	defer f.Close()
	if rest > 0 && !appendMode {
		f.Truncate(rest)
		f.Seek(rest, io.SeekStart)
	}
	c, ok := ss.dataConn()
	if !ok {
		return
	}
	b, err := io.ReadAll(c)
	c.Close()
	if ss.typ == 'A' {
		b = []byte(strings.ReplaceAll(string(b), "\r\n", "\n"))
	}
	f.Write(b)
	if err != nil {
		ss.reply(426, "aborted")
		return
	}
	ss.reply(226, "Transfer complete")
}

func ftpModeString(fi fs.FileInfo) string {
	m := fi.Mode()
	b := []byte("----------")
	switch {
	case m&fs.ModeSymlink != 0:
		b[0] = 'l'
	case m.IsDir():
		b[0] = 'd'
	}
	const rwx = "rwxrwxrwx"
	for i := 0; i < 9; i++ {
		if m&(1<<uint(8-i)) != 0 {
			b[i+1] = rwx[i]
		}
	}
	return string(b)
}

func ftpMLSDFacts(fi fs.FileInfo) string {
	typ := "file"
	if fi.IsDir() {
		typ = "dir"
	} else if fi.Mode()&fs.ModeSymlink != 0 {
		typ = "OS.unix=symlink"
	}
	return fmt.Sprintf("type=%s;size=%d;modify=%s;unix.mode=%04o;perm=adfrw;",
		typ, fi.Size(), fi.ModTime().UTC().Format("20060102150405"), fi.Mode().Perm())
}

func (ss *ftpSession) list(verb, arg string) {
	s := ss.srv
	if verb == "MLSD" && s.noMLSD {
		ss.closePasv()
		ss.reply(500, "MLSD not understood")
		return
	}
	// Strip ls-style options such as "-la".
	for strings.HasPrefix(arg, "-") {
		_, arg, _ = strings.Cut(arg, " ")
	}
	v := ss.vpath(arg)
	var out strings.Builder
	if text, ok := s.canned[v]; ok && verb == "LIST" {
		out.WriteString(text)
	} else {
		fi, err := os.Lstat(ss.real(v))
		if err != nil {
			ss.closePasv()
			ss.reply(550, "No such file or directory")
			return
		}
		var infos []fs.FileInfo
		if fi.IsDir() {
			des, _ := os.ReadDir(ss.real(v))
			for _, d := range des {
				if i, err := d.Info(); err == nil {
					infos = append(infos, i)
				}
			}
		} else {
			infos = []fs.FileInfo{fi}
		}
		sort.Slice(infos, func(i, j int) bool { return infos[i].Name() < infos[j].Name() })
		for _, i := range infos {
			switch {
			case verb == "NLST":
				out.WriteString(i.Name() + "\r\n")
			case verb == "MLSD":
				out.WriteString(ftpMLSDFacts(i) + " " + i.Name() + "\r\n")
			case s.listStyle == "dos":
				t := i.ModTime().UTC()
				sz := fmt.Sprintf("%14d", i.Size())
				if i.IsDir() {
					sz = fmt.Sprintf("%-14s", "<DIR>")
				}
				fmt.Fprintf(&out, "%s  %s %s %s\r\n", t.Format("01-02-06"), t.Format("03:04PM"), sz, i.Name())
			default:
				t := i.ModTime().UTC()
				stamp := t.Format("Jan _2 15:04")
				if time.Since(t) > 180*24*time.Hour || time.Until(t) > 24*time.Hour {
					stamp = t.Format("Jan _2  2006")
				}
				name := i.Name()
				if i.Mode()&fs.ModeSymlink != 0 {
					if target, err := os.Readlink(filepath.Join(ss.real(v), name)); err == nil {
						name += " -> " + target
					}
				}
				fmt.Fprintf(&out, "%s   1 owner    group %10d %s %s\r\n", ftpModeString(i), i.Size(), stamp, name)
			}
		}
	}
	text := out.String()
	if s.latin1Names {
		text = ftpTestToLatin1(text)
	}
	c, ok := ss.dataConn()
	if !ok {
		return
	}
	io.WriteString(c, text)
	c.Close()
	ss.reply(226, "Transfer complete")
}

// ftpTestCert creates a self-signed certificate for 127.0.0.1 and returns
// server and client TLS configurations that trust it.
func ftpTestCert(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mormi test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	server := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	client := &tls.Config{RootCAs: pool}
	return server, client
}

// ftpTestToLatin1 and ftpTestFromLatin1 are copies of the ftp package's
// unexported Latin-1 helpers, needed by the copied test server.
func ftpTestToLatin1(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r < 256 {
			b = append(b, byte(r))
		} else {
			b = append(b, '?')
		}
	}
	return string(b)
}
func ftpTestFromLatin1(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		b.WriteRune(rune(s[i]))
	}
	return b.String()
}
func ftpTestIsDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
