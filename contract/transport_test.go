package contract

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOptionalNullablePresence(t *testing.T) {
	data, err := ioutil.ReadFile("wire-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Params []json.RawMessage `json:"optionalAuthKeyParams"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Params) == 0 {
		t.Fatal("missing shared wire vectors")
	}
	for _, raw := range vectors.Params {
		var expected map[string]interface{}
		if err := json.Unmarshal(raw, &expected); err != nil {
			t.Fatal(err)
		}
		canonical, err := json.Marshal(expected)
		if err != nil {
			t.Fatal(err)
		}
		source := string(canonical)
		var params UpdateAuthKeyParams
		if err := json.Unmarshal([]byte(source), &params); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != source {
			t.Fatalf("presence changed: %s -> %s", source, encoded)
		}
	}
}

func TestNullableUnionDoesNotDecodeNullAsZero(t *testing.T) {
	var params UpdateAuthKeyParams
	if err := json.Unmarshal([]byte(`{"is_allowed_for_smartcdn":null}`), &params); err == nil {
		t.Fatal("non-nullable boolean accepted null")
	}
}

func TestTransportErrorDoesNotPrintSignedURL(t *testing.T) {
	client, err := NewClient(Config{Origin: "http://127.0.0.1:1", AuthKey: "synthetic-key", AuthSecret: "synthetic-secret"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.GetTemplate(ctx, GetTemplateInput{TemplateIdOrName: "test"})
	if err == nil || strings.Contains(err.Error(), "params") || strings.Contains(err.Error(), "synthetic-key") || !errors.Is(err, context.Canceled) {
		t.Fatalf("transport error was not safely wrapped: %v", err)
	}
}

func TestSignedQueryAndPathEncoding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.EscapedPath() != "/templates/name%20with%20%25%20and%20+" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
		params := r.URL.Query().Get("params")
		mac := hmac.New(sha512.New384, []byte("synthetic-secret"))
		mac.Write([]byte(params))
		if r.URL.Query().Get("signature") != "sha384:"+hex.EncodeToString(mac.Sum(nil)) {
			t.Error("signature did not cover transmitted params")
		}
		if !strings.Contains(params, `"nonce":0`) {
			t.Error("zero nonce was lost")
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"NOT_FOUND"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{Origin: server.URL, AuthKey: "synthetic-key", AuthSecret: "synthetic-secret"})
	if err != nil {
		t.Fatal(err)
	}
	var params GetTemplateParams
	if err := json.Unmarshal([]byte(`{"nonce":0}`), &params); err != nil {
		t.Fatal(err)
	}
	_, err = client.GetTemplate(context.Background(), GetTemplateInput{TemplateIdOrName: "name with % and +", Params: params})
	response, ok := err.(*ResponseError)
	if !ok || response.Status != 404 || string(response.Data) != `{"error":"NOT_FOUND"}` {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestBearerUpdateAndDeleteBody(t *testing.T) {
	var methods []string
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Header.Get("Authorization") != "Bearer synthetic-token" {
			t.Error("missing bearer token")
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		fields, err := url.ParseQuery(string(body))
		if err != nil {
			t.Error(err)
		}
		bodies = append(bodies, fields.Get("params"))
		if fields.Get("signature") != "" {
			t.Error("bearer request was signed")
		}
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"TEST_ERROR"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{Origin: server.URL, BearerToken: "synthetic-token"})
	if err != nil {
		t.Fatal(err)
	}
	var params UpdateAuthKeyParams
	if err := json.Unmarshal([]byte(`{"is_allowed_for_smartcdn":false,"nonce":0,"signature_algo":null}`), &params); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateAuthKey(context.Background(), UpdateAuthKeyInput{AuthKeyId: "123", Params: params}); err == nil {
		t.Fatal("expected test HTTP error")
	}
	if _, err := client.DeleteTemplate(context.Background(), DeleteTemplateInput{TemplateIdOrName: "123"}); err == nil {
		t.Fatal("expected test HTTP error")
	}
	if strings.Join(methods, ",") != "PUT,DELETE" {
		t.Fatalf("wrong methods: %v", methods)
	}
	if len(bodies) != 2 || bodies[0] != `{"is_allowed_for_smartcdn":false,"nonce":0,"signature_algo":null}` || bodies[1] != "{}" {
		t.Fatalf("wrong params: %v", bodies)
	}
}

func TestBasicTokenAndTextJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, secret, ok := r.BasicAuth()
		if !ok || key != "synthetic-key" || secret != "synthetic-secret" {
			t.Error("missing Basic auth")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Error("wrong token form")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(`{"access_token":"synthetic-token","expires_in":60,"scope":"templates:read","token_type":"Bearer"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{Origin: server.URL, AuthKey: "synthetic-key", AuthSecret: "synthetic-secret"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.IssueBearerToken(context.Background(), IssueBearerTokenInput{Body: IssueBearerTokenBody{GrantType: "client_credentials"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken != "synthetic-token" {
		t.Fatal("wrong decoded response")
	}
}

func TestMultipartAndRedirectRejection(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Error(err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		data, err := ioutil.ReadAll(file)
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(data, []byte{0, 255, 1}) {
			t.Error("changed upload bytes")
		}
		params := r.FormValue("params")
		mac := hmac.New(sha512.New384, []byte("synthetic-secret"))
		mac.Write([]byte(params))
		if r.FormValue("signature") != "sha384:"+hex.EncodeToString(mac.Sum(nil)) {
			t.Error("wrong multipart signature")
		}
		w.Header().Set("Location", "/must-not-follow")
		w.WriteHeader(307)
	}))
	defer server.Close()
	client, err := NewClient(Config{Origin: server.URL, AuthKey: "synthetic-key", AuthSecret: "synthetic-secret"})
	if err != nil {
		t.Fatal(err)
	}
	var params CreateAssemblyParams
	if err := json.Unmarshal([]byte(`{"template_id":"synthetic-template"}`), &params); err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateAssembly(context.Background(), CreateAssemblyInput{Params: params, Files: map[string]UploadFile{"file": {Reader: bytes.NewReader([]byte{0, 255, 1}), Filename: "test.bin"}}})
	if err == nil || calls != 1 {
		t.Fatalf("redirect policy failed: %v, calls=%d", err, calls)
	}
	_, err = client.GetTemplate(context.Background(), GetTemplateInput{TemplateIdOrName: ".."})
	if err == nil || calls != 1 {
		t.Fatal("dot path reached transport")
	}
}
