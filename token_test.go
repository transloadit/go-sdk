package transloadit

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestIssueBearerToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/token" {
			t.Errorf("expected POST /token, got %s %s", request.Method, request.URL.Path)
		}
		expectedAuthorization := "Basic " + base64.StdEncoding.EncodeToString([]byte("key:secret"))
		if request.Header.Get("Authorization") != expectedAuthorization {
			t.Errorf("unexpected Authorization header %q", request.Header.Get("Authorization"))
		}
		if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("unexpected Content-Type %q", request.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read token request: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Errorf("parse token request: %v", err)
		}
		if form.Get("grant_type") != "client_credentials" || form.Get("scope") != "assemblies:read" {
			t.Errorf("unexpected token form %q", form.Encode())
		}
		if _, present := form["aud"]; present {
			t.Errorf("aud should be omitted, got %q", form.Get("aud"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"abc","expires_in":21600,"scope":"assemblies:read","token_type":"Bearer"}`)
	}))
	defer server.Close()

	client := NewClient(Config{AuthKey: "key", AuthSecret: "secret", Endpoint: server.URL})
	token, err := client.IssueBearerToken(
		context.Background(),
		BearerTokenOptions{Scope: "assemblies:read"},
	)
	if err != nil {
		t.Fatalf("IssueBearerToken failed: %v", err)
	}
	if token.AccessToken != "abc" || token.ExpiresIn != 21600 || token.TokenType != "Bearer" {
		t.Fatalf("unexpected token response: %#v", token)
	}
}

func TestIssueBearerTokenDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	redirected := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirected" {
			redirected = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Redirect(w, request, "/redirected", http.StatusFound)
	}))
	defer server.Close()

	client := NewClient(Config{AuthKey: "key", AuthSecret: "secret", Endpoint: server.URL})
	_, err := client.IssueBearerToken(context.Background(), BearerTokenOptions{})
	if err == nil {
		t.Fatal("IssueBearerToken should reject a redirect response")
	}
	if redirected {
		t.Fatal("IssueBearerToken followed a redirect with Basic credentials")
	}
}
