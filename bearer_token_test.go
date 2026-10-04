package transloadit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIssueBearerToken_SendsBasicAuthAndFormBody(t *testing.T) {
	client := NewClient(Config{
		AuthKey:    "foo_key",
		AuthSecret: "foo_secret",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Errorf("expected path /token, got %q", r.URL.Path)
		}

		user, pass, ok := r.BasicAuth()
		if !ok || user != "foo_key" || pass != "foo_secret" {
			t.Errorf("expected basic auth foo_key/foo_secret, got %q/%q (ok=%v)", user, pass, ok)
		}

		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("grant_type"); got != "client_credentials" {
			t.Errorf("expected grant_type=client_credentials, got %q", got)
		}
		if got := r.Form.Get("scope"); got != "assemblies:read" {
			t.Errorf("expected scope=assemblies:read, got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"opaque-token","expires_in":21600,"scope":"assemblies:read","token_type":"Bearer"}`))
	}))
	defer server.Close()

	client.config.Endpoint = server.URL

	token, err := client.IssueBearerToken(context.Background(), BearerTokenRequest{Scope: "assemblies:read"})
	if err != nil {
		t.Fatal(err)
	}

	if token.AccessToken != "opaque-token" {
		t.Errorf("expected access token %q, got %q", "opaque-token", token.AccessToken)
	}
	if token.ExpiresIn != 21600 {
		t.Errorf("expected expires_in 21600, got %d", token.ExpiresIn)
	}
	if token.TokenType != "Bearer" {
		t.Errorf("expected token_type Bearer, got %q", token.TokenType)
	}
}

func TestIssueBearerToken_ReturnsRequestError(t *testing.T) {
	client := NewClient(Config{
		AuthKey:    "foo_key",
		AuthSecret: "foo_secret",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"GET_ACCOUNT_UNKNOWN_AUTH_KEY","message":"unknown auth key"}`))
	}))
	defer server.Close()

	client.config.Endpoint = server.URL

	_, err := client.IssueBearerToken(context.Background(), BearerTokenRequest{})
	if err == nil {
		t.Fatal("expected an error")
	}

	reqErr, ok := err.(RequestError)
	if !ok {
		t.Fatalf("expected RequestError, got %T: %s", err, err)
	}
	if reqErr.Code != "GET_ACCOUNT_UNKNOWN_AUTH_KEY" {
		t.Errorf("expected error code GET_ACCOUNT_UNKNOWN_AUTH_KEY, got %q", reqErr.Code)
	}
}
