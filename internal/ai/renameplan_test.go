package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func planAnswer(content string) func(w http.ResponseWriter) {
	return jsonAnswer(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}}})
}

func TestCheckLocalURL(t *testing.T) {
	for _, ok := range []string{"http://localhost:11434/v1", "http://127.0.0.1:1234/v1", "http://[::1]:8080/v1", "https://localhost/v1", "http://127.0.0.2:80", "http://ollama.localhost:11434/v1"} {
		if err := CheckLocalURL(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "https://openrouter.ai/api/v1", "http://192.168.1.5:11434/v1", "http://localhost.example.com/v1", "ftp://localhost/", "http://u:p@localhost/v1", "localhost:11434", "http://10.0.0.1/"} {
		if err := CheckLocalURL(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseRenamePlan(t *testing.T) {
	p, err := ParseRenamePlan("Sure! Here it is:\n```json\n{\"nameMask\": \"[Y]-[M]-[D] [N]\", \"search\": \" \", \"replace\": \"_\", \"regex\": \"false\", \"case\": \"lower\", \"explanation\": \"Date first,\\n spaces to underscores.\", \"rm\": \"-rf\"}\n```\nEnjoy.")
	if err != nil {
		t.Fatal(err)
	}
	if p.NameMask != "[Y]-[M]-[D] [N]" || p.Search != " " || p.Replace != "_" || p.RegEx || p.Case != "lower" || p.Explanation != "Date first, spaces to underscores." {
		t.Errorf("plan %+v", p)
	}
	if strings.Join(p.Set, ",") != "nameMask,search,replace,regex,case" {
		t.Errorf("set %v", p.Set)
	}
	for _, bad := range []string{
		"no json here",
		`{"explanation": "nothing"}`,
		`{"case": "shouting"}`,
		`{"search": "(", "regex": true}`,
		`{"nameMask": 5}`,
		`{"nameMask": "a\nb"}`,
		`{"regex": "maybe"}`,
		`{"nameMask": "` + strings.Repeat("x", 600) + `"}`,
	} {
		if _, err := ParseRenamePlan(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if p, err := ParseRenamePlan(`{"search": "(\\d+)", "replace": "#$1", "regex": true, "case": "unchanged"}`); err != nil || !p.RegEx || p.Case != "" {
		t.Errorf("regex plan %+v %v", p, err)
	}
}

func TestRenamePlanFromFakeLocalServer(t *testing.T) {
	f := newFakeAPI(t, planAnswer(`{"nameMask":"[N]","search":"IMG_","replace":"Holiday ","explanation":"Replaces IMG_ with Holiday."}`))
	c := &Client{BaseURL: f.srv.URL + "/v1", LocalOnly: true}
	names := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		names = append(names, "IMG_"+string(rune('a'+i%26))+".jpg")
	}
	p, err := c.RenamePlan(context.Background(), "llama3.2", "  replace IMG_ with Holiday  ", names)
	if err != nil {
		t.Fatal(err)
	}
	if p.Search != "IMG_" || p.Replace != "Holiday " {
		t.Errorf("plan %+v", p)
	}
	if len(f.requests) != 1 {
		t.Fatalf("%d requests", len(f.requests))
	}
	req := f.requests[0]
	if req["model"] != "llama3.2" || req["tools"] != nil {
		t.Errorf("request %v", req)
	}
	user := req["messages"].([]any)[1].(map[string]any)["content"].(string)
	if !strings.Contains(user, "Instruction: replace IMG_ with Holiday\n") || !strings.Contains(user, "(and 10 more)") || strings.Count(user, ".jpg") != MaxPlanNames {
		t.Errorf("prompt %q", user)
	}
	if h := f.headers[0].Get("Authorization"); h != "" {
		t.Errorf("Authorization %q sent", h)
	}

	// no model, no instruction, not local, not LocalOnly: refused before any request
	for _, tc := range []struct {
		c           *Client
		model, inst string
	}{
		{c, "", "x"},
		{c, "m", "  "},
		{&Client{BaseURL: "http://192.0.2.1/v1", LocalOnly: true}, "m", "x"},
		{&Client{BaseURL: f.srv.URL + "/v1"}, "m", "x"},
		{c, "m", strings.Repeat("x", MaxPlanInstruction+1)},
	} {
		if _, err := tc.c.RenamePlan(context.Background(), tc.model, tc.inst, nil); err == nil {
			t.Errorf("%+v accepted", tc)
		}
	}
	if len(f.requests) != 1 {
		t.Errorf("refused plans reached the server: %d requests", len(f.requests))
	}
}

// A local server that redirects to another machine is not followed, and a LocalOnly client does not dial a
// non-loopback address even when asked through the low-level send.
func TestRenamePlanStaysLocal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://192.0.2.1/v1/chat/completions", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/v1", LocalOnly: true}
	_, err := c.RenamePlan(context.Background(), "m", "do it", []string{"a.txt"})
	if err == nil || !strings.Contains(err.Error(), "not this computer") {
		t.Errorf("redirect: %v", err)
	}
	if err := loopbackOnly("tcp", "192.0.2.1:80", nil); err == nil {
		t.Error("dial to 192.0.2.1 allowed")
	}
	if err := loopbackOnly("tcp", "[::1]:80", nil); err != nil {
		t.Errorf("dial to ::1: %v", err)
	}
}
