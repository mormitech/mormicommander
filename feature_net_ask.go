package main

// Questions only the user can answer while a connection is made: the password, the passphrase of an SSH key,
// keyboard-interactive codes, the master password of the saved connections, and the SFTP host key ("The server's
// key is not known yet: accept it?"). The engines ask from the goroutine that dials, so the question goes to the page
// as a "net-ask" event and that goroutine waits for Net.Answer. The page shows it as a dialog (kind "net-<kind>"), so
// control mode can read and answer it like any other. When the connection is cancelled, open questions are withdrawn
// with "net-ask-gone".

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"mormicommander/internal/sftp"
)

// netAskTimeout is how long a question waits for an answer before the connection gives up.
const netAskTimeout = 15 * time.Minute

// ask sends a question to the page and waits for the answer; ok is false when the user cancelled (or nobody answered).
func (n *Net) ask(kind string, data map[string]any) (map[string]any, bool) {
	if data == nil {
		data = map[string]any{}
	}
	if n.asker != nil {
		return n.asker(kind, data)
	}
	if n.app == nil || n.app.ctx == nil {
		return nil, false
	}
	n.mu.Lock()
	n.askSeq++
	id := n.askSeq
	ch := make(chan map[string]any, 1)
	n.asks[id] = ch
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		delete(n.asks, id)
		n.mu.Unlock()
	}()
	data["id"], data["kind"] = id, kind
	n.app.emit("net-ask", data)
	t := time.NewTimer(netAskTimeout)
	defer t.Stop()
	select {
	case a := <-ch:
		if a == nil {
			return nil, false
		}
		ok, _ := a["ok"].(bool)
		return a, ok
	case <-t.C:
		n.app.emit("net-ask-gone", id)
		return nil, false
	}
}

// Answer answers a question of a connection ({ok: true, value: "…"} or {ok: false}).
func (n *Net) Answer(id int, answer map[string]any) error {
	n.mu.Lock()
	ch := n.asks[id]
	n.mu.Unlock()
	if ch == nil {
		return errors.New("This question is not open any more.")
	}
	select {
	case ch <- answer:
	default:
	}
	return nil
}

// dropAsks withdraws every open question (the connection was cancelled).
func (n *Net) dropAsks() {
	n.mu.Lock()
	ids := make([]int, 0, len(n.asks))
	for id, ch := range n.asks {
		ids = append(ids, id)
		select {
		case ch <- nil:
		default:
		}
	}
	n.mu.Unlock()
	for _, id := range ids {
		if n.app != nil {
			n.app.emit("net-ask-gone", id)
		}
	}
}

// askText asks for one text (a password when secret) and returns it.
func (n *Net) askText(kind, title, message string, secret bool) (string, bool) {
	a, ok := n.ask(kind, map[string]any{"title": title, "message": message, "secret": secret})
	if !ok {
		return "", false
	}
	v, _ := a["value"].(string)
	return v, true
}

// askPassword asks for a password.
func (n *Net) askPassword(title, message string) (string, bool) {
	return n.askText("password", title, message, true)
}

// askHostKey shows the SFTP server's key and lets the user accept it once, accept and save it, or refuse (F-632).
func (n *Net) askHostKey(info sftp.HostKeyInfo) sftp.HostKeyDecision {
	title, msg := "Unknown server key", fmt.Sprintf("mormiCommander has not connected to %s before. Make sure this is the server's key before you accept it.", info.Host)
	switch info.Status {
	case sftp.HostKeyChanged:
		title = "The server key has changed"
		msg = fmt.Sprintf("WARNING: the key of %s is not the one saved before. Someone may be listening in on the connection, or the server was set up again. Accept the new key only if you know why it changed.", info.Host)
	case sftp.HostKeyNewType:
		title = "New kind of server key"
		msg = fmt.Sprintf("%s shows a key of a kind not saved before. Accept it only if you trust the server.", info.Host)
	}
	known := make([]string, 0, len(info.KnownKeys))
	for _, k := range info.KnownKeys {
		known = append(known, k.Type+" "+k.Fingerprint)
	}
	a, ok := n.ask("hostkey", map[string]any{"title": title, "message": msg, "host": info.Host, "status": info.Status.String(),
		"keyType": info.KeyType, "fingerprint": info.Fingerprint, "md5": info.FingerprintMD5, "known": known, "saveFile": info.SaveFile})
	if !ok {
		return sftp.HostKeyReject
	}
	switch a["choice"] {
	case "save":
		return sftp.HostKeyAcceptAndSave
	case "once":
		return sftp.HostKeyAcceptOnce
	}
	return sftp.HostKeyReject
}

// askInteractive answers keyboard-interactive questions of an SSH server (passwords, one-time codes).
func (n *Net) askInteractive(user, host string) func(name, instruction string, questions []string, echos []bool) ([]string, error) {
	return func(name, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i, q := range questions {
			title := strings.TrimSpace(name)
			if title == "" {
				title = fmt.Sprintf("%s on %s", user, host)
			}
			v, ok := n.askText("interactive", title, strings.TrimSpace(strings.TrimSpace(instruction)+"\n"+q), !echos[i])
			if !ok {
				return nil, errNetCancelled
			}
			answers[i] = v
		}
		return answers, nil
	}
}
