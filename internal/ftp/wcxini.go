// This file imports saved connections from Total Commander's wcx_ftp.ini, so
// users switching from Total Commander keep their server list. The file is
// a Windows INI: a [connections] section lists the connection names, and
// each name has its own section with host (optionally with a scheme and a
// port), username, remote and local directory, passive mode and transfer
// mode. Total Commander stores passwords with its own reversible scrambling,
// which we deliberately do not import: the user enters each password once
// and it is then sealed with the master password like any other.
// Backslashes in connection names are Total Commander's way of grouping
// connections and become folders here.
package ftp

import (
	"bufio"
	"io"
	"sort"
	"strconv"
	"strings"
)

// ParseINI reads a Windows-style INI file into sections of lower-case keys
// (section names keep their case; lookups are case-insensitive by
// lower-casing the section name in the map key as well).
func parseINI(r io.Reader) (map[string]map[string]string, []string, error) {
	sections := map[string]map[string]string{}
	var order []string
	cur := ""
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			line = strings.TrimPrefix(line, "\uFEFF")
			first = false
		}
		line = strings.TrimSpace(line)
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if line[0] == '[' && strings.HasSuffix(line, "]") {
			cur = line[1 : len(line)-1]
			key := strings.ToLower(cur)
			if _, ok := sections[key]; !ok {
				sections[key] = map[string]string{}
				order = append(order, cur)
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		sec := sections[strings.ToLower(cur)]
		if sec == nil {
			sec = map[string]string{}
			sections[strings.ToLower(cur)] = sec
		}
		sec[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return sections, order, sc.Err()
}

// ImportWcxFtpINI converts Total Commander's wcx_ftp.ini into sites (without
// IDs or passwords), in the order of its [connections] list. Add them with
// Store.Add.
func ImportWcxFtpINI(r io.Reader) ([]Site, error) {
	secs, _, err := parseINI(r)
	if err != nil {
		return nil, err
	}
	conns := secs["connections"]
	type numbered struct {
		n    int
		name string
	}
	var list []numbered
	for k, v := range conns {
		if n, err := strconv.Atoi(k); err == nil && v != "" {
			list = append(list, numbered{n, v})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n < list[j].n })
	var sites []Site
	for _, item := range list {
		sec := secs[strings.ToLower(item.name)]
		if sec == nil || sec["host"] == "" {
			continue
		}
		site, ok := wcxSite(item.name, sec)
		if ok {
			sites = append(sites, site)
		}
	}
	return sites, nil
}

func wcxSite(fullName string, sec map[string]string) (Site, bool) {
	name := fullName
	folder := ""
	if i := strings.LastIndexAny(fullName, `\/`); i >= 0 {
		folder = cleanFolder(fullName[:i])
		name = fullName[i+1:]
	}
	site := Site{Name: name, Folder: folder, Protocol: ProtoFTP}
	host := sec["host"]
	switch {
	case strings.HasPrefix(strings.ToLower(host), "sftp://"):
		site.Protocol = ProtoSFTP
		host = host[7:]
	case strings.HasPrefix(strings.ToLower(host), "ftps://"):
		site.Protocol = ProtoFTPS
		host = host[7:]
	case strings.HasPrefix(strings.ToLower(host), "ftp://"):
		host = host[6:]
	}
	if sec["ssl"] == "1" && site.Protocol == ProtoFTP {
		site.Protocol = ProtoFTPES
	}
	host = strings.TrimSuffix(host, "/")
	if h, p, ok := cutPort(host); ok {
		host, site.Port = h, p
	}
	if host == "" {
		return Site{}, false
	}
	site.Host = host
	if u := sec["username"]; !strings.EqualFold(u, "anonymous") {
		site.User = u
	}
	site.RemoteDir = sec["directory"]
	site.LocalDir = sec["localdir"]
	if v, ok := sec["pasvmode"]; ok && v == "0" {
		site.Active = true
	}
	switch strings.ToUpper(sec["transfermode"]) {
	case "I":
		site.Transfer = "binary"
	case "A":
		site.Transfer = "ascii"
	case "":
	default:
		site.Transfer = "auto"
	}
	if sec["utf8"] == "1" {
		site.Encoding = "utf-8"
	}
	if v, err := strconv.Atoi(sec["keepalive"]); err == nil && v > 0 {
		site.Keepalive = v
	}
	return site, true
}

// cutPort splits "host:port" (and "[v6]:port"); bare IPv6 addresses are
// left alone.
func cutPort(s string) (string, int, bool) {
	if strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]")
		if end < 0 {
			return s, 0, false
		}
		host := s[1:end]
		if rest := s[end+1:]; strings.HasPrefix(rest, ":") {
			if p, err := strconv.Atoi(rest[1:]); err == nil {
				return host, p, true
			}
		}
		return host, 0, true
	}
	if strings.Count(s, ":") != 1 {
		return s, 0, false
	}
	h, ps, _ := strings.Cut(s, ":")
	p, err := strconv.Atoi(ps)
	if err != nil || p <= 0 || p > 65535 {
		return s, 0, false
	}
	return h, p, true
}
