package transloadit

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSign_UsesSha384WithAlgorithmPrefix(t *testing.T) {
	client := NewClient(Config{
		AuthKey:    "foo_key",
		AuthSecret: "foo_secret",
		Endpoint:   "https://api2.transloadit.com",
	})

	params, signature, err := client.sign(map[string]interface{}{
		"foo": "bar",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(signature, "sha384:") {
		t.Fatalf("signature prefix should be sha384:, got %q", signature)
	}

	hash := hmac.New(sha512.New384, []byte(client.config.AuthSecret))
	hash.Write([]byte(params))
	expected := "sha384:" + hex.EncodeToString(hash.Sum(nil))

	if signature != expected {
		t.Fatalf("wrong signature, expected %q got %q", expected, signature)
	}
}

func TestListRequest_UsesSha384WithAlgorithmPrefix(t *testing.T) {
	client := NewClient(Config{
		AuthKey:    "foo_key",
		AuthSecret: "foo_secret",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query().Get("params")
		signature := r.URL.Query().Get("signature")

		if params == "" {
			t.Fatal("params query should be set")
		}

		if !strings.HasPrefix(signature, "sha384:") {
			t.Fatalf("listRequest signature prefix should be sha384:, got %q", signature)
		}

		hash := hmac.New(sha512.New384, []byte(client.config.AuthSecret))
		hash.Write([]byte(params))
		expected := "sha384:" + hex.EncodeToString(hash.Sum(nil))

		if signature != expected {
			t.Fatalf("wrong listRequest signature, expected %q got %q", expected, signature)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"count":0}`))
	}))
	defer server.Close()

	client.config.Endpoint = server.URL

	list, err := client.ListTemplates(context.Background(), &ListOptions{PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}

	if list.Count != 0 {
		t.Fatalf("expected empty list count 0, got %d", list.Count)
	}
}
