package ai

// The one place the app itself goes on the network for AI (issue #146, step 2): an HTTPS request to the chat
// completions API the user configured (OpenRouter by default). It is only called by Client.Complete, when the user
// starts an AI run with that provider, and by Client.Models, when the user opens the model list of the AI settings;
// the zero-network test (privacy_test.go) allows this function and nothing else in the package.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"
)

// maxResponse caps how much of an answer is read: a chat answer or a model list is well below it, and a server that
// sends without end must not fill the memory.
const maxResponse = 32 << 20

// send makes one request and returns the status and body of the answer. The key goes in the Authorization header
// only (never in the address, which servers and proxies log). openRouter adds OpenRouter's optional app headers,
// which name mormiCommander and its project page, nothing about the user. Go drops the Authorization header itself
// when a redirect leads to another host; a redirect from https to plain http on the same host would keep it, so such
// a redirect is refused and the key never travels in clear text. localOnly (the rename plan, issue #33) connects to
// loopback addresses only, checked on the resolved address of every connection, takes no proxy, and refuses a
// redirect to a host that is not this computer.
func send(ctx context.Context, method, url, key string, openRouter, localOnly bool, body []byte) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if openRouter {
		req.Header.Set("HTTP-Referer", ProjectURL)
		req.Header.Set("X-Title", ProjectTitle)
	}
	// A model may think for minutes before it answers a long conversation; Stop cancels ctx sooner.
	client := &http.Client{Timeout: 10 * time.Minute, CheckRedirect: noDowngrade}
	if localOnly {
		dialer := &net.Dialer{Timeout: 10 * time.Second, Control: loopbackOnly}
		client.Transport = &http.Transport{Proxy: nil, DialContext: dialer.DialContext}
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if !IsLoopbackHost(req.URL.Hostname()) {
				return fmt.Errorf("refused a redirect to %s, which is not this computer", req.URL.Hostname())
			}
			return noDowngrade(req, via)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, b, nil
}

// noDowngrade refuses a redirect from https to http (the key would follow in clear text) and keeps Go's limit of ten.
func noDowngrade(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refused a redirect from https to %s, which would send the API key unencrypted", req.URL.Scheme)
	}
	return nil
}

// loopbackOnly refuses a connection to any address that is not a loopback one (it runs after name resolution, on
// the address actually dialled).
func loopbackOnly(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("refused to connect to %s, which is not this computer", host)
	}
	return nil
}
