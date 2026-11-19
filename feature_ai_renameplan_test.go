package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIRenamePlan(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"case\":\"lower\",\"explanation\":\"Lowercase.\"}"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	// off by default: no address, no button, refused without a request
	x, _ := testAI(t, aiOptions{})
	if x.RenamePlanReady() {
		t.Error("ready with no address")
	}
	if _, err := x.RenamePlan("lowercase", []string{"A.TXT"}); err == nil {
		t.Error("plan with no address")
	}
	// a cloud address is refused
	x.options = func() aiOptions { return aiOptions{LocalURL: "https://openrouter.ai/api/v1", LocalModel: "m"} }
	if _, err := x.RenamePlan("lowercase", []string{"A.TXT"}); err == nil {
		t.Error("plan with a cloud address")
	}
	if hits != 0 {
		t.Fatalf("%d requests before the address was local", hits)
	}
	x.options = func() aiOptions { return aiOptions{LocalURL: srv.URL + "/v1", LocalModel: "llama3.2"} }
	if !x.RenamePlanReady() {
		t.Error("not ready with an address")
	}
	p, err := x.RenamePlan("lowercase everything", []string{"A.TXT", "B.TXT"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Case != "lower" || len(p.Set) != 1 || p.Explanation != "Lowercase." || hits != 1 {
		t.Errorf("plan %+v, %d hits", p, hits)
	}
}
