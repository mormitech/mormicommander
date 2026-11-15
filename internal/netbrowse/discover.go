package netbrowse

// Finding servers. Each system's own tool is run for a few seconds and its output parsed; the parsers are plain
// functions so they are tested without a network.

import (
	"bufio"
	"context"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Discovery is how long a search for servers runs.
const Discovery = 3 * time.Second

// serviceKinds maps the Bonjour/Avahi service types to the protocol.
var serviceKinds = map[string]string{"_smb._tcp": "smb", "_afpovertcp._tcp": "afp", "_nfs._tcp": "nfs"}

// Discover finds the servers on the local network with this system's tool. It never fails: a missing tool or a
// network without servers gives an empty list (and the reason, for the page to show).
func Discover(ctx context.Context) ([]Server, string) {
	ctx, cancel := context.WithTimeout(ctx, Discovery+2*time.Second)
	defer cancel()
	list, note := discover(ctx)
	return Dedup(list), note
}

// Dedup keeps one entry per kind and host (or name), sorted by name.
func Dedup(list []Server) []Server {
	seen := map[string]bool{}
	var out []Server
	for _, s := range list {
		k := s.Kind + "\x00" + strings.ToLower(s.Host)
		if s.Host == "" {
			k = s.Kind + "\x00name:" + strings.ToLower(s.Name)
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name); a != b {
			return a < b
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// runFor runs a tool that keeps running (dns-sd -B) for d and returns what it printed.
func runFor(ctx context.Context, d time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	hide(cmd)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		err = nil // stopped on purpose
	}
	return string(out), err
}

// --- macOS: dns-sd ---

// dnssdAdd matches a line of `dns-sd -B`: time, Add, flags, interface, domain, type, instance name. Before 10 in the
// morning the time starts with a blank (" 7:17:44.781").
var dnssdAdd = regexp.MustCompile(`^\s*\S+\s+(Add|Rmv)\s+\d+\s+\d+\s+(\S+)\s+(\S+)\s+(.+?)\s*$`)

// ParseDNSSDBrowse reads the instances `dns-sd -B <type>` found (removed ones left out).
func ParseDNSSDBrowse(out string) (names []string) {
	alive := map[string]bool{}
	var order []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		m := dnssdAdd.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		name := m[4]
		if m[1] == "Add" {
			if _, ok := alive[name]; !ok {
				order = append(order, name)
			}
			alive[name] = true
		} else {
			alive[name] = false
		}
	}
	for _, n := range order {
		if alive[n] {
			names = append(names, n)
		}
	}
	return names
}

// dnssdReach matches the answer of `dns-sd -L`: "... can be reached at host.local.:445 (interface 4)".
var dnssdReach = regexp.MustCompile(`can be reached at (\S+?)\.?:(\d+)`)

// ParseDNSSDLookup reads the host and port `dns-sd -L` resolved an instance to.
func ParseDNSSDLookup(out string) (host string, port int) {
	m := dnssdReach.FindStringSubmatch(out)
	if m == nil {
		return "", 0
	}
	port, _ = strconv.Atoi(m[2])
	return strings.TrimSuffix(m[1], "."), port
}

// discoverDNSSD browses every service type with dns-sd, then resolves each instance to its host.
func discoverDNSSD(ctx context.Context) ([]Server, string) {
	if _, err := exec.LookPath("dns-sd"); err != nil {
		return nil, "dns-sd is missing, so servers can't be found; type a server instead."
	}
	var mu sync.Mutex
	var out []Server
	var wg sync.WaitGroup
	for typ, kind := range serviceKinds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text, _ := runFor(ctx, Discovery, "dns-sd", "-B", typ, "local.")
			for _, name := range ParseDNSSDBrowse(text) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					res, _ := runFor(ctx, 1500*time.Millisecond, "dns-sd", "-L", name, typ, "local.")
					host, port := ParseDNSSDLookup(res)
					if host == "" {
						return
					}
					mu.Lock()
					out = append(out, Server{Name: name, Host: host, Port: nonDefaultPort(kind, port), Kind: kind, Source: "bonjour"})
					mu.Unlock()
				}()
			}
		}()
	}
	wg.Wait()
	if len(out) == 0 {
		// macOS shows an app nothing on the local network until it is allowed there (Local Network privacy); the
		// question comes once, and a "Don't allow" is changed only in System Settings
		return out, "No servers found. If your servers are missing, allow mormiCommander in System Settings > Privacy & Security > Local Network, then try again; or type a server."
	}
	return out, ""
}

// nonDefaultPort keeps a port only when it is not the protocol's usual one, so URLs stay short.
func nonDefaultPort(kind string, port int) int {
	if port == map[string]int{"smb": 445, "afp": 548, "nfs": 2049}[kind] {
		return 0
	}
	return port
}

// --- Linux: avahi-browse ---

// ParseAvahi reads `avahi-browse -tpr` (resolved, parsable): "=;eth0;IPv4;NAS;_smb._tcp;local;nas.local;192.168.1.5;445;...".
func ParseAvahi(out string) []Server {
	var list []Server
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		f := strings.Split(sc.Text(), ";")
		if len(f) < 9 || f[0] != "=" {
			continue
		}
		kind := serviceKinds[f[4]]
		if kind == "" {
			continue
		}
		port, _ := strconv.Atoi(f[8])
		host := strings.TrimSuffix(f[6], ".")
		if host == "" {
			host = f[7]
		}
		list = append(list, Server{Name: avahiUnescape(f[3]), Host: host, Port: nonDefaultPort(kind, port), Kind: kind, Source: "avahi"})
	}
	return list
}

// avahiUnescape undoes avahi-browse's escapes: "\032" is a byte in decimal, "\." a dot.
func avahiUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b []byte
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			b = append(b, s[i])
			i++
			continue
		}
		if i+3 < len(s) {
			if n, err := strconv.Atoi(s[i+1 : i+4]); err == nil && n < 256 {
				b = append(b, byte(n))
				i += 4
				continue
			}
		}
		if i+1 < len(s) {
			b = append(b, s[i+1])
			i += 2
			continue
		}
		b = append(b, s[i])
		i++
	}
	return string(b)
}

// discoverAvahi runs avahi-browse for every service type.
func discoverAvahi(ctx context.Context) ([]Server, string) {
	if _, err := exec.LookPath("avahi-browse"); err != nil {
		return nil, "avahi-browse (package avahi-utils) is missing, so servers can't be found; type a server instead."
	}
	var mu sync.Mutex
	var out []Server
	var wg sync.WaitGroup
	for typ := range serviceKinds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text, _ := runFor(ctx, Discovery+time.Second, "avahi-browse", "-tpr", typ)
			mu.Lock()
			out = append(out, ParseAvahi(text)...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out, ""
}

// --- Windows: net view ---

// ParseNetView reads the servers `net view` lists ("\\NAS   remark").
func ParseNetView(out string) []Server {
	var list []Server
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, `\\`) {
			continue
		}
		name := strings.Fields(line[2:])
		if len(name) == 0 {
			continue
		}
		list = append(list, Server{Name: name[0], Host: name[0], Kind: "smb", Source: "netbios"})
	}
	return list
}

func discoverNetView(ctx context.Context) ([]Server, string) {
	text, err := runFor(ctx, Discovery+2*time.Second, "net", "view")
	list := ParseNetView(text)
	if err != nil && len(list) == 0 {
		return nil, "Windows lists no servers (network discovery may be off); type a server instead."
	}
	return list, ""
}
