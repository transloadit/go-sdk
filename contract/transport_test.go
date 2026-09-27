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
	"runtime"
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

func TestResponseErrorCode(t *testing.T) {
	for _, test := range []struct{ body, code string }{
		{`{"error":"TEMPLATE_NOT_FOUND","message":"synthetic-private"}`, "TEMPLATE_NOT_FOUND"},
		{`{"error":"synthetic-private"}`, ""},
		{`{"error":["TEMPLATE_NOT_FOUND"]}`, ""},
		{`{"message":"synthetic-private"}`, ""},
		{`null`, ""},
		{`invalid JSON`, ""},
	} {
		err := &ResponseError{Status: 400, Data: []byte(test.body)}
		if err.Code() != test.code {
			t.Fatalf("unexpected admitted error code: %q", err.Code())
		}
		if err.Error() != "API request failed with HTTP 400" {
			t.Fatal("response content entered error message")
		}
	}
}

func TestIntegralJSONRepresentations(t *testing.T) {
	for _, source := range []string{"1.0", "1e3", "-2.00", "9223372036854775807.0", "0e-9223372036854775808"} {
		var value ValueIntegerOrString
		if err := json.Unmarshal([]byte(source), &value); err != nil {
			t.Fatalf("valid integral representation %s: %v", source, err)
		}
		if value.Integer == nil {
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

func TestIntegerNormalizationHasBoundedAllocations(t *testing.T) {
	// A long mantissa and a cancelling exponent still represent the small integer 1.
	data := []byte("1" + strings.Repeat("0", 10000) + "e-10000")
	allocations := testing.AllocsPerRun(1, func() {
		var value int64
		if err := unmarshalInteger(data, &value); err != nil || value != 1 {
			t.Fatalf("valid cancelling exponent: %d, %v", value, err)
		}
	})
	if allocations > 32 {
		t.Fatalf("integer normalization allocated %.0f times for a 10 KB token", allocations)
	}
}

func TestExactIntegerNormalization(t *testing.T) {
	for _, test := range []struct {
		source string
		value  int64
	}{
		{"10.0", 10}, {"1.2300e2", 123}, {"1200e-2", 12}, {"0.001E+3", 1},
		{"-0.001e3", -1}, {"9223372036854775807.0", 9223372036854775807},
		{"-9223372036854775808.0", -9223372036854775808},
		{"-0e99999999999999999999999999999999", 0},
		{"1" + strings.Repeat("0", 1000000) + "e-1000000", 1},
	} {
		var value int64
		if err := unmarshalInteger([]byte(test.source), &value); err != nil || value != test.value {
			t.Fatalf("integer normalization: got %d, want %d, error %v", value, test.value, err)
		}
	}
	for _, source := range []string{"1.2300e1", "9223372036854775808e0", "-9223372036854775809e0", "1e9223372036854775807", "1e-9223372036854775808", "1.0e999999999999999999999", "01", `"1"`, "true"} {
		var value int64
		if err := unmarshalInteger([]byte(source), &value); err == nil {
			t.Fatalf("invalid integer accepted: %s", source)
		}
	}
}

func TestRecursiveUnionDecodingDoesNotMultiplyWork(t *testing.T) {
	allocations := func(depth int) float64 {
		data := []byte(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
		return testing.AllocsPerRun(1, func() {
			var value CreateAssemblyParams_Object2_Steps_AdditionalProperty_AiChat_Messages_Variant2_Array_Item_Variant_Variant1_System1_ProviderOptions_AdditionalProperty_AdditionalProperty
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
		})
	}
	shallow, deep := allocations(5), allocations(10)
	// Compare allocations rather than wall time so busy CI hosts cannot make this flaky.
	if deep > shallow*8 {
		t.Fatalf("doubling nesting multiplied decoder work: %.0f -> %.0f allocations", shallow, deep)
	}
}

func TestUnionRejectsExcessiveNesting(t *testing.T) {
	data := []byte(strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65))
	var value CreateAssemblyParams_Object2_Steps_AdditionalProperty_AiChat_Messages_Variant2_Array_Item_Variant_Variant1_System1_ProviderOptions_AdditionalProperty_AdditionalProperty
	if err := json.Unmarshal(data, &value); err == nil {
		t.Fatal("union decoding accepted more than 64 nested containers")
	}
	if err := json.Unmarshal([]byte(strings.Repeat("[", 64)+"0"+strings.Repeat("]", 64)), &value); err != nil {
		t.Fatalf("documented maximum depth must remain usable: %v", err)
	}
	quoted, err := json.Marshal(strings.Repeat(`[{\"\\`, 100))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(quoted, &value); err != nil {
		t.Fatalf("quoted delimiters must not count as nesting: %v", err)
	}
}

func TestUnionPathsDoNotCopyLongParents(t *testing.T) {
	children := make(map[string]interface{})
	for index := 0; index < 128; index++ {
		children[fmt.Sprint(index)] = true
	}
	value := map[string]interface{}{strings.Repeat("p", 256*1024): children}
	paths := make(map[unionPath]int)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	unionFieldPaths(value, 0, paths)
	runtime.ReadMemStats(&after)
	// The old full-prefix representation allocates over 32 MiB for this 256 KiB object.
	// Leave ample room for map growth and runtime bookkeeping without a timing assertion.
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 4*1024*1024 {
		t.Fatalf("field paths copied their long parent names: %d allocated bytes", allocated)
	}
	if len(paths) != 129 {
		t.Fatal("lost a nested field path")
	}
}

type trackedUpload struct {
	io.Reader
	closed bool
}

func (reader *trackedUpload) Close() error { reader.closed = true; return nil }

func TestMultipartRejectsHeaderLineBreaks(t *testing.T) {
	for _, test := range []struct{ name, fileKey, filename, extraKey string }{
		{"filename CR", "file", "image\r.jpg", "field"},
		{"filename LF", "file", "image\n.jpg", "field"},
		{"file key CR", "file\rname", "image.jpg", "field"},
		{"file key LF", "file\nname", "image.jpg", "field"},
		{"extra key CR", "file", "image.jpg", "field\rname"},
		{"extra key LF", "file", "image.jpg", "field\nname"},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			client, err := NewClient(Config{BearerToken: "synthetic-token", HTTPClient: &http.Client{
				Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
					requests++
					return nil, errors.New("unexpected request")
				}),
			}})
			if err != nil {
				t.Fatal(err)
			}
			reader := &trackedUpload{Reader: strings.NewReader("synthetic")}
			err = client.request(context.Background(), operation{Method: "POST", Path: "/upload", Encoding: "multipart/form-data", ParamsField: "params"}, nil, struct{}{},
				map[string]UploadFile{test.fileKey: {Reader: reader, Filename: test.filename}},
				map[string]string{test.extraKey: "value"}, nil)
			if err == nil || requests != 0 || !reader.closed {
				t.Fatalf("invalid multipart input must fail before HTTP and close its stream: err=%v requests=%d closed=%v", err, requests, reader.closed)
			}
		})
	}
}

func TestGeneratedReadableAccessors(t *testing.T) {
	var absent *ValueNullOrString
	if absent.GetString() != "" {
		t.Fatal("nil scalar accessor")
	}
	for _, source := range []string{
		`{"ok":"ASSEMBLY_UPLOADING","assembly_id":"canonical","assemblyId":"legacy"}`,
		`{"ok":"ASSEMBLY_COMPLETED","assembly_id":"canonical","assemblyId":"legacy"}`,
		`{"error":"ASSEMBLY_CRASHED","assembly_id":"canonical","assemblyId":"legacy"}`,
	} {
		var status GetAssemblyResult
		if err := json.Unmarshal([]byte(source), &status); err != nil {
			t.Fatal(err)
		}
		if status.GetAssemblyId() != "canonical" || status.GetAssemblyIdCamelCase() != "legacy" {
			t.Fatal("Assembly ID spellings were confused")
		}
		encoded, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		var roundtrip GetAssemblyResult
		if err := json.Unmarshal(encoded, &roundtrip); err != nil {
			t.Fatal(err)
		}
		if roundtrip.GetAssemblyId() != "canonical" || roundtrip.GetAssemblyIdCamelCase() != "legacy" {
			t.Fatal("Assembly ID was discarded on marshal")
		}
	}
	for _, source := range []string{`"workspace-template"`, `"builtin/example@latest"`} {
		var id CreateTemplateResult_Id
		if err := json.Unmarshal([]byte(source), &id); err != nil {
			t.Fatal(err)
		}
		if id.GetString() == "" {
			t.Fatal("Template ID requires positional variant knowledge")
		}
	}
}

func TestExtremeIntegerExponent(t *testing.T) {
	var value Integer
	if err := json.Unmarshal([]byte("1e-9223372036854775808"), &value); err == nil {
		t.Fatal("extreme exponent was accepted")
	}
}

func TestCanonicalLoopbackOrigins(t *testing.T) {
	for _, origin := range []string{"http://LOCALHOST:8080", "http://localhost.:8080"} {
		if _, err := NewClient(Config{Origin: origin, AuthKey: "synthetic-key", AuthSecret: "synthetic-secret"}); err != nil {
			t.Fatalf("canonical loopback origin rejected: %s: %v", origin, err)
		}
	}
	for _, origin := range []string{"http://localhost.example.com", "http://127.0.0.1.", "http://127.0.0.1.example.com"} {
		if _, err := NewClient(Config{Origin: origin, AuthKey: "synthetic-key", AuthSecret: "synthetic-secret"}); err == nil {
			t.Fatalf("non-loopback DNS name accepted: %s", origin)
		}
	}
}

func TestOriginRejectsEmptyQueryDelimiter(t *testing.T) {
	for _, origin := range []string{"https://api.example/?", "https://api.example/proxy?", "http://localhost:8080/?"} {
		if _, err := NewClient(Config{Origin: origin, AuthKey: "synthetic-key", AuthSecret: "synthetic-secret"}); err == nil {
			t.Fatalf("empty query delimiter would swallow appended operation paths: %s", origin)
		}
	}
}

func TestUnionFieldSelection(t *testing.T) {
	type first struct {
		A *string `json:"a,omitempty"`
	}
	type second struct {
		B *string `json:"b,omitempty"`
	}
	candidates := []func() interface{}{
		func() interface{} { return new(first) },
		func() interface{} { return new(second) },
	}
	choice, decoded, err := unmarshalUnion([]byte(`{"b":"x","future":true}`), candidates, []bool{false, false})
	if err != nil || choice != 1 {
		t.Fatalf("did not retain the modeled field: %d, %v", choice, err)
	}
	winner, ok := decoded.(*second)
	if !ok || winner.B == nil || *winner.B != "x" {
		t.Fatal("decoded winner was not preserved")
	}
	if _, _, err := unmarshalUnion([]byte(`{"a":"x","b":"y"}`), candidates, []bool{false, false}); err == nil {
		t.Fatal("accepted alternatives that each discard a modeled field")
	}
	if _, _, err := unmarshalUnion([]byte(`null`), candidates, []bool{false, false}); err == nil {
		t.Fatal("accepted non-nullable union null")
	}
	// A raw alternative can preserve all fields without coercing large JSON numbers to float64.
	candidates = append(candidates, func() interface{} { return new(json.RawMessage) })
	choice, _, err = unmarshalUnion([]byte(`{"a":"x","b":"y","big":1e400}`), candidates, []bool{false, false, true})
	if err != nil || choice != 2 {
		t.Fatalf("did not select the lossless alternative: %d, %v", choice, err)
	}
}

func TestUnionFieldPathEscaping(t *testing.T) {
	paths := make(map[unionPath]int)
	unionFieldPaths(map[string]interface{}{
		"a/b":  true,
		"a":    map[string]interface{}{"b": true},
		"a~1b": []interface{}{map[string]interface{}{"x": true}},
	}, 0, paths)
	array := paths[unionPath{key: "a~1b"}]
	item := paths[unionPath{parent: array, key: "0", array: true}]
	for _, path := range []unionPath{
		{key: "a/b"}, {key: "a"}, {parent: paths[unionPath{key: "a"}], key: "b"},
		{key: "a~1b"}, {parent: array, key: "0", array: true}, {parent: item, key: "x"},
	} {
		if paths[path] == 0 {
			t.Fatalf("missing distinct field path: %v", path)
		}
	}
	if len(paths) != 6 {
		t.Fatalf("unexpected field paths: %v", paths)
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
