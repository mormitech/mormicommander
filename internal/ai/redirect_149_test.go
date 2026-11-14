package ai

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A redirect from https to http on the same host would keep the Authorization header, so it is refused (#149).
func TestNoDowngradeRedirect(t *testing.T) {
	first := httptest.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", nil)
	down := httptest.NewRequest("POST", "http://openrouter.ai/api/v1/chat/completions", nil)
	if err := noDowngrade(down, []*http.Request{first}); err == nil {
		t.Fatal("an https to http redirect was followed")
	}
	same := httptest.NewRequest("POST", "https://openrouter.ai/other", nil)
	if err := noDowngrade(same, []*http.Request{first}); err != nil {
		t.Fatalf("an https to https redirect was refused: %v", err)
	}
	local := httptest.NewRequest("POST", "http://127.0.0.1:1234/b", nil)
	plain := httptest.NewRequest("POST", "http://127.0.0.1:1234/a", nil)
	if err := noDowngrade(local, []*http.Request{plain}); err != nil {
		t.Fatalf("an http to http redirect (a local server) was refused: %v", err)
	}
}
