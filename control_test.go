package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func startTestControl(t *testing.T) (*App, *control) {
	t.Helper()
	a := NewApp(startOpts{ControlFile: filepath.Join(t.TempDir(), "ctl.json")})
	if _, err := a.startControl("0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.stopControl)
	return a, a.ctl
}

func post(t *testing.T, c *control, path, body string, hdr map[string]string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("POST", "http://"+c.addr+path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	json.NewDecoder(res.Body).Decode(&m)
	return res.StatusCode, m
}

func TestControlFileAndAuth(t *testing.T) {
	a, c := startTestControl(t)
	b, err := os.ReadFile(c.file)
	if err != nil {
		t.Fatal(err)
	}
	var info map[string]any
	json.Unmarshal(b, &info)
	if info["token"] != c.token || !strings.HasPrefix(info["url"].(string), "http://127.0.0.1:") {
		t.Errorf("control file %v", info)
	}
	if st, _ := os.Stat(c.file); st.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Errorf("control file readable by others: %v", st.Mode())
	}
	if code, _ := post(t, c, "/cmd", `{"cmd":"info"}`, nil); code != 401 {
		t.Errorf("no token: %d", code)
	}
	if code, _ := post(t, c, "/cmd", `{"cmd":"info"}`, map[string]string{"Authorization": "Bearer " + c.token, "Origin": "http://evil"}); code != 403 {
		t.Errorf("origin: %d", code)
	}
	code, m := post(t, c, "/cmd", `{"cmd":"info"}`, map[string]string{"Authorization": "Bearer " + c.token})
	if code != 200 || m["version"] != version() {
		t.Errorf("info: %d %v", code, m)
	}
	_ = a
}

func TestControlQueueAndGoCommands(t *testing.T) {
	a, c := startTestControl(t)
	registerControl("test-echo", func(a *App, m map[string]any) (map[string]any, error) {
		return map[string]any{"ok": true, "echo": m["v"]}, nil
	})
	auth := map[string]string{"Authorization": "Bearer " + c.token}
	if _, m := post(t, c, "/cmd", `{"cmd":"test-echo","v":"hi"}`, auth); m["echo"] != "hi" {
		t.Errorf("registered command: %v", m)
	}
	post(t, c, "/cmd", `{"cmd":"answer","kind":"folder","values":["/tmp/x"]}`, auth)
	if v, ok := a.mockAnswer("folder", "start"); !ok || v != "/tmp/x" {
		t.Errorf("queued answer %q %v", v, ok)
	}
	if v, ok := a.mockAnswer("folder", "again"); !ok || v != "" {
		t.Errorf("empty queue gave %q", v)
	}
	if _, m := post(t, c, "/cmd", `{"cmd":"asked"}`, auth); len(m["asked"].([]any)) != 2 {
		t.Errorf("asked %v", m)
	}
	if _, m := post(t, c, "/cmd", `{}`, auth); m["error"] != "no cmd" {
		t.Errorf("empty: %v", m)
	}
}

func TestControlPageReply(t *testing.T) {
	a, c := startTestControl(t)
	// no window: the page part is played by a goroutine that answers the first command
	go func() {
		for {
			c.mu.Lock()
			n := len(c.waiting)
			c.mu.Unlock()
			if n > 0 {
				a.ControlReply(1, `{"ok":true,"from":"page"}`)
				return
			}
		}
	}()
	auth := map[string]string{"Authorization": "Bearer " + c.token}
	if _, m := post(t, c, "/cmd", `{"cmd":"state"}`, auth); m["from"] != "page" {
		t.Errorf("page reply: %v", m)
	}
	file := c.file
	a.stopControl()
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("control file left behind")
	}
}

// TestControlGuardRefusals pins the rest of the guard (audit #88): a wrong token, a request for another Host (DNS
// rebinding) and the GET /info endpoint without a token are refused, the server listens on loopback only, and control
// mode is off unless asked for.
func TestControlGuardRefusals(t *testing.T) {
	_, c := startTestControl(t)
	if !strings.HasPrefix(c.addr, "127.0.0.1:") {
		t.Errorf("control listens on %q, want loopback only", c.addr)
	}
	if code, _ := post(t, c, "/cmd", `{"cmd":"info"}`, map[string]string{"Authorization": "Bearer wrong"}); code != 401 {
		t.Errorf("wrong token: %d", code)
	}
	req, _ := http.NewRequest("POST", "http://"+c.addr+"/cmd", strings.NewReader(`{"cmd":"info"}`))
	req.Host = "evil.example:80"
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Errorf("foreign host: %d", res.StatusCode)
	}
	res, err = http.Get("http://" + c.addr + "/info")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Errorf("info without token: %d", res.StatusCode)
	}
	if o := parseArgs(nil); o.Control != "" || o.Demo {
		t.Errorf("control or demo on by default: %+v", o)
	}
}
