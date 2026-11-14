package ai

// The one place the app itself goes on the network for AI (issue #146, step 2): an HTTPS request to the chat
// completions API the user configured (OpenRouter by default). It is only called by Client.Complete, when the user
// starts an AI run with that provider, and by Client.Models, when the user opens the model list of the AI settings;
// the zero-network test (privacy_test.go) allows this function and nothing else in the package.

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

// maxResponse caps how much of an answer is read: a chat answer or a model list is well below it, and a server that
// sends without end must not fill the memory.
const maxResponse = 32 << 20

// send makes one request and returns the status and body of the answer. The key goes in the Authorization header
// only (never in the address, which servers and proxies log). openRouter adds OpenRouter's optional app headers,
// which name mormiCommander and its project page, nothing about the user. Go drops the Authorization header itself
// when a redirect leads to another host, so a redirect can't carry the key away.
func send(ctx context.Context, method, url, key string, openRouter bool, body []byte) (int, []byte, error) {
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
	client := &http.Client{Timeout: 10 * time.Minute}
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
