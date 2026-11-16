// The zero-network test: the promise in the README ("no telemetry, no update checks, no connections of its own") checked
// against the Go source, so anyone can run `go test -run ZeroNetwork .` and see it hold. Every place in the Go code that
// can open a connection, listen on a port or look up a host name is found by reading the syntax tree (not by grepping),
// and each one has to be in a known, explained place:
//
//   - the FTP, SFTP and WebDAV clients (internal/ftp, internal/sftp, internal/webdav), which only run when the user
//     connects to a server they typed in (Net > FTP connect and friends), and which only the network feature and the
//     remote file system adapters may import;
//   - four loopback-only listeners: control mode (control.go, off unless started with --control), the "one app,
//     several windows" hand-over (feature_config_start.go), the AI layer's MCP endpoint (feature_ai_mcp.go, only
//     from the first AI run the user starts, for the Claude CLI that run starts) and the Lister's media player
//     (feature_lister_stream.go, only from the first sound or film the user plays in F3, for the app's own web view).
//     Their address must be 127.0.0.1, written in the source;
//   - one outbound HTTPS client: the AI layer's chat completions client (the function send in internal/ai/chat_http.go,
//     issue #146 step 2), which talks to the API address the user set in the AI settings (OpenRouter by default). It is
//     allowed as that one function, not as the file or the package, and TestZeroNetworkAIClient checks the only ways
//     to reach it: an AI run the user starts with that provider, and the model list the user opens.
//
// Anything else, a new http.Get or net.Dial anywhere, fails the test with the file and line. The frontend half of the
// promise is checked by frontend/test/privacy.test.js.
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// networkSinks lists, per import path, the names that open a connection, listen, or ask a name server.
var networkSinks = map[string]map[string]bool{
	"net": set("Dial", "DialTimeout", "DialTCP", "DialUDP", "DialIP", "DialUnix", "Listen", "ListenPacket", "ListenTCP",
		"ListenUDP", "ListenIP", "ListenUnix", "ListenUnixgram", "ListenMulticastUDP", "LookupHost", "LookupIP",
		"LookupAddr", "LookupCNAME", "LookupMX", "LookupTXT", "LookupSRV", "LookupNS", "LookupPort", "LookupNetIP",
		"Dialer", "ListenConfig", "Resolver", "DefaultResolver", "FileConn", "FileListener"),
	"net/http": set("Get", "Post", "Head", "PostForm", "NewRequest", "NewRequestWithContext", "Client", "DefaultClient",
		"Transport", "DefaultTransport", "ListenAndServe", "ListenAndServeTLS", "Serve", "ServeTLS"),
	"crypto/tls":                 set("Dial", "DialWithDialer", "Listen", "Client", "Dialer"),
	"golang.org/x/crypto/ssh":    set("Dial"),
	"net/smtp":                   set("Dial", "SendMail", "NewClient"),
	"net/rpc":                    set("Dial", "DialHTTP", "DialHTTPPath"),
	"net/rpc/jsonrpc":            set("Dial"),
	"github.com/coder/websocket": set("Dial"),
}

func set(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

// remoteClientDirs are the packages that talk to servers the user connects to on purpose.
var remoteClientDirs = []string{"internal/ftp/", "internal/sftp/", "internal/webdav/"}

// loopbackFiles may listen or dial, but only on 127.0.0.1.
var loopbackFiles = map[string]bool{"control.go": true, "feature_config_start.go": true, "feature_ai_mcp.go": true, "feature_lister_stream.go": true}

// outboundFuncs are the functions (by file) that may make a request to an address the user configured: the AI chat
// completions client's send, and the SMB login that lists a server's shares when the user picks or types a server in
// the network neighbourhood (#148).
var outboundFuncs = map[string]map[string]bool{"internal/ai/chat_http.go": set("send"), "internal/netbrowse/shares.go": set("smbDial")}

// sinkUse is one place a network name is used.
type sinkUse struct {
	file string // slash path relative to the repository
	line int
	name string // "net.Listen"
	addr string // the address argument when it could be read ("127.0.0.1:" for "127.0.0.1:"+port), or ""
	call bool   // used as a call (not only named, as in a type)
	fn   string // the function it is in ("Type.Method" for a method; "" outside any function)
}

// funcName is a function declaration's name, "Type.Method" for a method.
func funcName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	t := d.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + d.Name.Name
	}
	return d.Name.Name
}

// enclosingFunc is the name of the function of f that contains pos ("" when none does).
func enclosingFunc(f *ast.File, pos token.Pos) string {
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Pos() <= pos && pos < fd.End() {
			return funcName(fd)
		}
	}
	return ""
}

// goSources returns the repository's non-test Go files (slash paths), leaving out the frontend, builds and worktrees.
func goSources(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", "frontend", "build", "node_modules", "testdata", "vendor", "dist":
				if p != "." {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			files = append(files, filepath.ToSlash(p))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 50 {
		t.Fatalf("found only %d Go files; is the test running from the repository root?", len(files))
	}
	return files
}

// findSinks parses one file and returns every use of a network name in it, plus the import paths it uses.
func findSinks(fset *token.FileSet, path string, src any) ([]sinkUse, []string, error) {
	f, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}
	alias := map[string]string{} // local name → import path
	var imports []string
	for _, im := range f.Imports {
		ip, _ := strconv.Unquote(im.Path.Value)
		imports = append(imports, ip)
		name := ip[strings.LastIndex(ip, "/")+1:]
		if im.Name != nil {
			name = im.Name.Name
		}
		alias[name] = ip
	}
	var uses []sinkUse
	called := map[*ast.SelectorExpr]*ast.CallExpr{}
	ast.Inspect(f, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if s, ok := c.Fun.(*ast.SelectorExpr); ok {
				called[s] = c
			}
		}
		return true
	})
	ast.Inspect(f, func(n ast.Node) bool {
		s, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := s.X.(*ast.Ident)
		if !ok {
			return true
		}
		ip, ok := alias[id.Name]
		if !ok || !networkSinks[ip][s.Sel.Name] {
			return true
		}
		u := sinkUse{file: path, line: fset.Position(s.Pos()).Line, name: id.Name + "." + s.Sel.Name, fn: enclosingFunc(f, s.Pos())}
		if c := called[s]; c != nil {
			u.call = true
			if len(c.Args) >= 2 {
				u.addr = leadingLiteral(c.Args[1])
			}
		}
		uses = append(uses, u)
		return true
	})
	return uses, imports, nil
}

// leadingLiteral returns the fixed text an address expression starts with: a string literal, the left end of a
// concatenation, or the format of an fmt.Sprintf call.
func leadingLiteral(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			s, _ := strconv.Unquote(v.Value)
			return s
		}
	case *ast.BinaryExpr:
		if v.Op == token.ADD {
			return leadingLiteral(v.X)
		}
	case *ast.CallExpr:
		if s, ok := v.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "Sprintf" && len(v.Args) > 0 {
			return leadingLiteral(v.Args[0])
		}
	case *ast.ParenExpr:
		return leadingLiteral(v.X)
	}
	return ""
}

// checkSink says what is wrong with one use, or "" when it is in an allowed place.
func checkSink(u sinkUse) string {
	for _, d := range remoteClientDirs {
		if strings.HasPrefix(u.file, d) {
			return ""
		}
	}
	if outboundFuncs[u.file][u.fn] {
		return ""
	}
	if loopbackFiles[u.file] {
		if !u.call {
			return ""
		}
		if strings.HasPrefix(u.addr, "127.0.0.1:") {
			return ""
		}
		return u.name + " in a loopback-only file must use a literal 127.0.0.1 address"
	}
	return u.name + " outside the FTP/SFTP/WebDAV clients and the loopback listeners"
}

// remoteImportAllowed says whether a file may import one of the remote client packages.
func remoteImportAllowed(file string) bool {
	base := file[strings.LastIndex(file, "/")+1:]
	if !strings.Contains(file, "/") && strings.HasPrefix(base, "feature_net") {
		return true // the network feature (Net menu, connect dialogs, transfers)
	}
	if strings.HasPrefix(file, "internal/vfs/adapter_ftp") || strings.HasPrefix(file, "internal/vfs/adapter_sftp") {
		return true // the remote panels the network feature opens
	}
	for _, d := range remoteClientDirs {
		if strings.HasPrefix(file, d) {
			return true // the clients among themselves
		}
	}
	return false
}

func TestZeroNetwork(t *testing.T) {
	fset := token.NewFileSet()
	found := 0
	for _, file := range goSources(t) {
		uses, imports, err := findSinks(fset, file, nil)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		found += len(uses)
		for _, u := range uses {
			if msg := checkSink(u); msg != "" {
				t.Errorf("%s:%d: %s", u.file, u.line, msg)
			}
		}
		for _, ip := range imports {
			for _, d := range remoteClientDirs {
				if ip == "mormicommander/"+strings.TrimSuffix(d, "/") && !remoteImportAllowed(file) {
					t.Errorf("%s imports %s; only the network feature may start a remote connection", file, ip)
				}
			}
		}
	}
	// The scan has to see the known places, or it is not looking at all.
	if found < 5 {
		t.Fatalf("only %d network uses found; the scan is broken", found)
	}
}

// The checker itself, on made-up files: it must catch what the real test is there to catch.
func TestZeroNetworkChecker(t *testing.T) {
	cases := []struct {
		file, src string
		bad       int // number of uses checkSink rejects
	}{
		{"feature_x.go", `package main; import "net/http"; func f() { http.Get("https://example.com/ping") }`, 1},
		{"feature_x.go", `package main; import h "net/http"; var c = &h.Client{}`, 1},
		{"feature_x.go", `package main; import "net"; func f() { net.Dial("tcp", "example.com:80") }`, 1},
		{"feature_x.go", `package main; import "net"; func f() { var d net.Dialer; _ = d }`, 1},
		{"internal/thing/x.go", `package thing; import "net"; func f() { net.LookupHost("example.com") }`, 1},
		{"feature_x.go", `package main; import "net/http"; func f(b []byte) string { return http.DetectContentType(b) }`, 0},
		{"feature_x.go", `package main; import "net/url"; func f() { url.Parse("https://example.com") }`, 0},
		{"control.go", `package main; import "net"; func f(p string) { net.Listen("tcp", "127.0.0.1:"+p) }`, 0},
		{"control.go", `package main; import "net"; func f(p string) { net.Listen("tcp", ":"+p) }`, 1},
		{"control.go", `package main; import ("fmt"; "net"); func f(p int) { net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p), 0) }`, 0},
		{"feature_config_start.go", `package main; import "net"; func f(a string) { net.Listen("tcp", a) }`, 1},
		{"internal/ftp/x.go", `package ftp; import "net"; func f(a string) { net.Dial("tcp", a) }`, 0},
		{"internal/ai/chat_http.go", `package ai; import "net/http"; func send(u string) { http.NewRequest("GET", u, nil); _ = &http.Client{} }`, 0},
		{"internal/ai/chat_http.go", `package ai; import "net/http"; func other(u string) { http.Get(u) }`, 1},
		{"internal/ai/chat_http.go", `package ai; import "net/http"; var c = &http.Client{}`, 1},
		{"internal/ai/chat.go", `package ai; import "net/http"; func send(u string) { http.Get(u) }`, 1},
		{"internal/ai/chat_http.go", `package ai; import "net/http"; type T struct{}; func (T) send(u string) { http.Get(u) }`, 1},
	}
	for _, c := range cases {
		uses, _, err := findSinks(token.NewFileSet(), c.file, c.src)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		bad := 0
		for _, u := range uses {
			if checkSink(u) != "" {
				bad++
			}
		}
		if bad != c.bad {
			t.Errorf("%s in %s: %d rejected, want %d (uses %+v)", c.src, c.file, bad, c.bad, uses)
		}
	}
	for file, want := range map[string]bool{
		"feature_net.go": true, "feature_net_xfer.go": true, "internal/vfs/adapter_sftp.go": true,
		"internal/sftp/scp.go": true, "feature_fileops.go": false, "internal/vfs/local.go": false,
		"internal/thing/feature_net.go": false,
	} {
		if got := remoteImportAllowed(file); got != want {
			t.Errorf("remoteImportAllowed(%q) = %v, want %v", file, got, want)
		}
	}
}

// callSites lists, per function, the calls in the Go files of dir (tests left out) whose callee is named name: a plain
// call name(...) or a selector call x.name(...).
func callSites(t *testing.T, dir string, name string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	fset := token.NewFileSet()
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := ""
			switch fn := c.Fun.(type) {
			case *ast.Ident:
				callee = fn.Name
			case *ast.SelectorExpr:
				callee = fn.Sel.Name
			}
			if callee == name {
				file := filepath.ToSlash(p)
				out[file] = append(out[file], enclosingFunc(f, c.Pos()))
			}
			return true
		})
	}
	return out
}

// TestZeroNetworkAIClient follows the AI client's one network function back to what the user does: send is called by
// Client.do only, do by Client.Complete (the run's loop) and Client.Models (the model list) only, and a Client is
// made in two places only: startOpenRouter, when the user starts a run with that provider, and ModelsFrom, when the
// user opens the model list of the AI settings.
func TestZeroNetworkAIClient(t *testing.T) {
	check := func(dir, name string, allowed ...string) {
		t.Helper()
		ok := set(allowed...)
		sites := callSites(t, dir, name)
		n := 0
		for file, fns := range sites {
			for _, fn := range fns {
				n++
				if !ok[file+":"+fn] {
					t.Errorf("%s: %s calls %s; only %v may", file, fn, name, allowed)
				}
			}
		}
		if n == 0 {
			t.Errorf("no call of %s found in %s; the check is not looking", name, dir)
		}
	}
	check("internal/ai", "send", "internal/ai/chat.go:Client.do")
	check("internal/ai", "do", "internal/ai/chat.go:Client.Complete", "internal/ai/chat.go:Client.Models")
	check("internal/ai", "Complete", "internal/ai/chat.go:Loop.Run")
	// Every Client made anywhere (ai.Client outside the package, Client inside it), by file and function: the
	// methods above can only be called on one.
	fset := token.NewFileSet()
	made := map[string]bool{}
	for _, file := range goSources(t) {
		inAI := strings.HasPrefix(file, "internal/ai/")
		f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			switch typ := cl.Type.(type) {
			case *ast.SelectorExpr:
				if id, ok := typ.X.(*ast.Ident); ok && id.Name == "ai" && typ.Sel.Name == "Client" {
					made[file+":"+enclosingFunc(f, cl.Pos())] = true
				}
			case *ast.Ident:
				if inAI && typ.Name == "Client" {
					made[file+":"+enclosingFunc(f, cl.Pos())] = true
				}
			}
			return true
		})
	}
	want := map[string]bool{"feature_ai_openrouter.go:AI.startOpenRouter": true, "feature_ai_openrouter.go:AI.ModelsFrom": true}
	for k := range made {
		if !want[k] {
			t.Errorf("an ai.Client is made in %s; only %v may make one", k, want)
		}
	}
	for k := range want {
		if !made[k] {
			t.Errorf("no ai.Client made in %s; is the check still looking?", k)
		}
	}
}
