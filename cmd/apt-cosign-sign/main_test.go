package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveIDToken_ExplicitWins(t *testing.T) {
	tok, err := resolveIDToken("explicit-token", false)
	if err != nil {
		t.Fatalf("resolveIDToken: %v", err)
	}
	if tok != "explicit-token" {
		t.Fatalf("tok = %q", tok)
	}
}

func TestGitHubActionsIDToken_NotInActions(t *testing.T) {
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")

	_, ok, err := githubActionsIDToken()
	if err != nil {
		t.Fatalf("githubActionsIDToken: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false outside GitHub Actions")
	}
}

func TestGitHubActionsIDToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("audience"); got != "sigstore" {
			t.Errorf("audience query param = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer runner-token" {
			t.Errorf("Authorization header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":"fake-jwt"}`))
	}))
	defer srv.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runner-token")

	tok, ok, err := githubActionsIDToken()
	if err != nil {
		t.Fatalf("githubActionsIDToken: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if tok != "fake-jwt" {
		t.Fatalf("tok = %q", tok)
	}
}

func TestGitHubActionsIDToken_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runner-token")

	_, ok, err := githubActionsIDToken()
	if !ok {
		t.Fatal("expected ok=true (we are 'in Actions'), even though the request failed")
	}
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}
