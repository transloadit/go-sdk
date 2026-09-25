package contract

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
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

type roundTripFunction func(*http.Request) (*http.Response, error)

func (run roundTripFunction) RoundTrip(request *http.Request) (*http.Response, error) {
	return run(request)
}

type failedBody struct{ err error }

func (body failedBody) Read([]byte) (int, error) { return 0, body.err }
func (failedBody) Close() error                  { return nil }

type staticBody struct{ io.Reader }

func (staticBody) Close() error { return nil }

func TestDefaultDeadlineBelongsToCaller(t *testing.T) {
	client, err := NewClient(Config{AuthKey: "synthetic-key", AuthSecret: "synthetic-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if client.httpClient.Timeout != 0 {
		t.Fatal("default client imposes a total upload deadline")
	}
}

func TestAdditiveSuccessResponseFields(t *testing.T) {
	var token IssueBearerTokenResult
	if err := json.Unmarshal([]byte(`{"access_token":"synthetic","expires_in":60,"scope":"templates:read","token_type":"Bearer","future_field":true}`), &token); err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "synthetic" {
		t.Fatal("lost known response fields")
	}
}

func TestIntegralJSONRepresentations(t *testing.T) {
	for _, source := range []string{"1.0", "1e3", "-2.00", "9223372036854775807.0"} {
		var value ValueIntegerOrString
		if err := json.Unmarshal([]byte(source), &value); err != nil {
			t.Fatalf("valid integral representation %s: %v", source, err)
		}
		if value.Choice1 == nil {
			t.Fatalf("numeric token became another alternative: %s", source)
		}
	}
	for _, source := range []string{"1.5", "1.00000000000000000001", "9223372036854775808.0", "1e1000000000"} {
		var value ValueIntegerOrString
		if err := json.Unmarshal([]byte(source), &value); err == nil {
			t.Fatalf("non-integral or overflowing value accepted: %s", source)
		}
	}
}

type blockingUpload struct {
	started   chan struct{}
	closed    chan struct{}
	readDone  chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func (reader *blockingUpload) Read([]byte) (int, error) {
	reader.startOnce.Do(func() { close(reader.started) })
	<-reader.closed
	close(reader.readDone)
	return 0, io.EOF
}

func (reader *blockingUpload) Close() error {
	reader.closeOnce.Do(func() { close(reader.closed) })
	return nil
}

func TestMultipartReaderFinishesBeforeReturn(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%t", cancelRequest), func(t *testing.T) {
			reader := &blockingUpload{started: make(chan struct{}), closed: make(chan struct{}), readDone: make(chan struct{})}
			defer reader.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			consumed := make(chan struct{})
			client, err := NewClient(Config{AuthKey: "synthetic-key", AuthSecret: "synthetic-secret", HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
				go func() { io.Copy(ioutil.Discard, request.Body); close(consumed) }()
				defer request.Body.Close()
				select {
				case <-reader.started:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				if cancelRequest {
					cancel()
					return nil, ctx.Err()
				}
				return &http.Response{StatusCode: 400, Body: staticBody{strings.NewReader(`{"error":"TEST_ERROR"}`)}, Header: http.Header{}}, nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			var params CreateAssemblyParams
			if err := json.Unmarshal([]byte(`{"template_id":"synthetic-template"}`), &params); err != nil {
				t.Fatal(err)
			}
			_, err = client.CreateAssembly(ctx, CreateAssemblyInput{Params: params, Files: map[string]UploadFile{"file": {Reader: reader, Filename: "file.bin"}}})
			<-consumed
			if err == nil {
				t.Fatal("expected early-response or cancellation error")
			}
			select {
			case <-reader.readDone:
			default:
				t.Fatal("SDK returned while still reading the caller's upload")
			}
		})
	}
}

func TestBodyReadTransportError(t *testing.T) {
	cause := errors.New("synthetic read failure")
	client, err := NewClient(Config{AuthKey: "synthetic-key", AuthSecret: "synthetic-secret", HTTPClient: &http.Client{Transport: roundTripFunction(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: failedBody{cause}, Header: http.Header{}}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetTemplate(context.Background(), GetTemplateInput{TemplateIdOrName: "test"})
	var transportError *TransportError
	if !errors.As(err, &transportError) || !errors.Is(err, cause) {
		t.Fatalf("body error is not a transport error: %v", err)
	}
}

func TestRejectInsecureRemoteOrigin(t *testing.T) {
	_, err := NewClient(Config{Origin: "http://proxy.example.com", AuthKey: "synthetic-key", AuthSecret: "synthetic-secret"})
	if err == nil {
		t.Fatal("insecure credential transport was accepted")
	}
}

func TestBuiltinTemplateAndProxyPrefix(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/proxy/templates/builtin/encode-hls-video@0.0.1" {
			t.Error("changed proxy prefix or built-in Template ID")
		}
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"TEST_ERROR"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{Origin: server.URL + "/proxy", AuthKey: "synthetic-key", AuthSecret: "synthetic-secret"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetTemplate(context.Background(), GetTemplateInput{TemplateIdOrName: "builtin/encode-hls-video@0.0.1"})
	if _, ok := err.(*ResponseError); !ok {
		t.Fatalf("valid built-in ID was not sent: %v", err)
	}
	_, err = client.GetTemplate(context.Background(), GetTemplateInput{TemplateIdOrName: "builtin/../auth_keys"})
	if err == nil || calls != 1 {
		t.Fatal("unsafe path was not rejected before transport")
	}
}

func TestAssemblyUploadConstraints(t *testing.T) {
	var params CreateAssemblyParams
	if err := json.Unmarshal([]byte(`{"template_id":"synthetic","auth":{"max_size":7,"max_number_of_files":1,"referer":"example.invalid"}}`), &params); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(Config{AuthKey: "synthetic-key", AuthSecret: "synthetic-secret", HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		defer request.MultipartForm.RemoveAll()
		raw := request.FormValue("params")
		var data struct {
			Auth struct {
				Key      string `json:"key"`
				MaxSize  int    `json:"max_size"`
				MaxFiles int    `json:"max_number_of_files"`
				Referer  string `json:"referer"`
			} `json:"auth"`
		}
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			t.Error(err)
		}
		if data.Auth.Key != "synthetic-key" || data.Auth.MaxSize != 7 || data.Auth.MaxFiles != 1 || data.Auth.Referer != "example.invalid" {
			t.Error("upload constraints were lost")
		}
		return &http.Response{StatusCode: 400, Body: staticBody{strings.NewReader(`{"error":"TEST_ERROR"}`)}, Header: http.Header{}}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateAssembly(context.Background(), CreateAssemblyInput{Params: params})
	if _, ok := err.(*ResponseError); !ok {
		t.Fatalf("unexpected request error: %v", err)
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
	_, err = client.CreateAssembly(context.Background(), CreateAssemblyInput{Params: params, Files: map[string]UploadFile{"file": {Reader: ioutil.NopCloser(bytes.NewReader([]byte{0, 255, 1})), Filename: "test.bin"}}})
	if err == nil || calls != 1 {
		t.Fatalf("redirect policy failed: %v, calls=%d", err, calls)
	}
	_, err = client.GetTemplate(context.Background(), GetTemplateInput{TemplateIdOrName: ".."})
	if err == nil || calls != 1 {
		t.Fatal("dot path reached transport")
	}
}
