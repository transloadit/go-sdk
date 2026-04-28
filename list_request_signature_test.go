package transloadit

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListRequest_UsesSha384PrefixedSignature(t *testing.T) {
	t.Parallel()

	var capturedParams string
	var capturedSignature string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		capturedParams = query.Get("params")
		capturedSignature = query.Get("signature")

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[],"count":0}`)
	}))
	defer server.Close()

	client := NewClient(Config{
		AuthKey:    "test-key",
		AuthSecret: "test-secret",
		Endpoint:   server.URL,
	})

	_, err := client.ListTemplates(context.Background(), &ListOptions{PageSize: 3})
	if err != nil {
		t.Fatalf("ListTemplates failed: %v", err)
	}

	if capturedParams == "" {
		t.Fatal("params should not be empty")
	}
	if capturedSignature == "" {
		t.Fatal("signature should not be empty")
	}
	if !strings.HasPrefix(capturedSignature, "sha384:") {
		t.Fatalf("expected sha384-prefixed signature, got %q", capturedSignature)
	}

	mac := hmac.New(sha512.New384, []byte("test-secret"))
	mac.Write([]byte(capturedParams))
	expected := "sha384:" + hex.EncodeToString(mac.Sum(nil))
	if capturedSignature != expected {
		t.Fatalf("signature mismatch\nexpected: %s\nactual:   %s", expected, capturedSignature)
	}
}
