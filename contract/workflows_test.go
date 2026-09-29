package contract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func tusFixtureClient(t *testing.T, location string, overrides map[string]string, creationStatus, patchStatus int) (*Client, *[]string) {
	t.Helper()
	origin := "http://127.0.0.1:4000"
	id := strings.Repeat("1", 32)
	metadata, position := "", 0
	methods := []string{}
	client, err := NewClient(Config{Origin: origin, BearerToken: "never-forward", HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		methods = append(methods, request.Method)
		if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" {
			t.Error("credentials forwarded")
		}
		header := make(http.Header)
		header.Set("Tus-Resumable", "1.0.0")
		status := 200
		switch request.Method {
		case "GET":
			return workflowJSON(t, map[string]string{"assembly_id": id, "ok": "ASSEMBLY_UPLOADING", "assembly_ssl_url": origin + "/assemblies/" + id, "tus_url": origin + "/resumable/files/"}), nil
		case "POST":
			metadata = request.Header.Get("Upload-Metadata")
			header.Set("Location", location)
			status = creationStatus
		case "HEAD":
			header.Set("Upload-Length", "4")
			header.Set("Upload-Offset", strconv.Itoa(position))
			header.Set("Upload-Metadata", metadata)
			for key, value := range overrides {
				header.Set(key, value)
			}
		case "PATCH":
			body, err := ioutil.ReadAll(request.Body)
			if err != nil {
				t.Error(err)
			}
			position += len(body)
			header.Set("Upload-Offset", strconv.Itoa(position))
			status = patchStatus
		default:
			t.Errorf("unexpected method %s", request.Method)
		}
		return &http.Response{StatusCode: status, Header: header, Body: ioutil.NopCloser(strings.NewReader(""))}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	return client, &methods
}

func tusFixtureInput() AssemblyUploadOptions {
	return AssemblyUploadOptions{AssemblyID: strings.Repeat("1", 32), Reader: bytes.NewReader([]byte("test")), Size: 4, Filename: "input.txt", RetryDelay: time.Millisecond}
}

func TestTusWorkflowRejectsDestinations(t *testing.T) {
	for _, location := range []string{
		"https://evil.example/resumable/files/one", "//evil.example/resumable/files/one",
		"http://127.0.0.1:4000/resumable/files/../one", "http://127.0.0.1:4000/resumable/files/%2e%2e",
		"http://127.0.0.1:4000/resumable/files/one?override=DELETE", "http://127.0.0.1:4000/resumable/files/one#fragment",
		"http://127.0.0.1:4000/resumable/files/one/two", "https://user:secret@api2-uploader.transloadit.com/resumable/files/one",
	} {
		t.Run(location, func(t *testing.T) {
			client, methods := tusFixtureClient(t, location, nil, 201, 204)
			_, err := client.UploadAssemblyFile(context.Background(), tusFixtureInput())
			var failure *AssemblyUploadError
			if !errors.As(err, &failure) || len(*methods) != 2 {
				t.Fatalf("untrusted destination was followed: %v, %v", err, *methods)
			}
		})
	}
}

func TestTusWorkflowRejectsHeaders(t *testing.T) {
	for _, headers := range []map[string]string{
		{"Upload-Length": "5"}, {"Upload-Length": "3"}, {"Upload-Offset": "-1"}, {"Upload-Offset": "1.5"},
		{"Upload-Offset": "5"}, {"Upload-Offset": "00"}, {"Upload-Offset": "0, 0"},
		{"Tus-Resumable": "other"}, {"Upload-Metadata": ""},
		{"Upload-Metadata": "assembly_url Zm9yZWlnbg==,filename aW5wdXQudHh0,fieldname ZmlsZQ=="},
	} {
		client, methods := tusFixtureClient(t, "/resumable/files/one", headers, 201, 204)
		_, err := client.UploadAssemblyFile(context.Background(), tusFixtureInput())
		if err == nil || len(*methods) != 3 {
			t.Fatalf("invalid headers reached PATCH: %v, %v", headers, err)
		}
	}
}

func TestTusWorkflowPersistenceAndChangedFile(t *testing.T) {
	client, methods := tusFixtureClient(t, "/resumable/files/one", nil, 201, 204)
	input := tusFixtureInput()
	var saved AssemblyUploadSession
	persistence := errors.New("persistence unavailable")
	input.OnSession = func(session AssemblyUploadSession) error { saved = session; return persistence }
	_, err := client.UploadAssemblyFile(context.Background(), input)
	digest := sha256.Sum256([]byte("test"))
	if !errors.Is(err, persistence) || saved.SHA256 != hex.EncodeToString(digest[:]) || len(*methods) != 2 {
		t.Fatalf("checkpoint was not persisted before bytes: %v", err)
	}
	fresh, calls := tusFixtureClient(t, "/resumable/files/one", nil, 201, 204)
	input.Reader = bytes.NewReader([]byte("xxxx"))
	if _, err := fresh.ResumeAssemblyFile(context.Background(), input, saved); err == nil || len(*calls) != 0 {
		t.Fatal("changed file was allowed")
	}
}

func TestTusWorkflowRecoveryIsOffsetBasedAndBounded(t *testing.T) {
	client, methods := tusFixtureClient(t, "/resumable/files/one", nil, 503, 204)
	if _, err := client.UploadAssemblyFile(context.Background(), tusFixtureInput()); err == nil || len(*methods) != 2 {
		t.Fatal("uncertain creation was retried")
	}
	client, methods = tusFixtureClient(t, "/resumable/files/one", nil, 201, 503)
	if _, err := client.UploadAssemblyFile(context.Background(), tusFixtureInput()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*methods, ",") != "GET,POST,HEAD,PATCH,HEAD" {
		t.Fatalf("accepted bytes were replayed: %v", *methods)
	}
	client, methods = tusFixtureClient(t, "/resumable/files/one", nil, 201, 204)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.UploadAssemblyFile(ctx, tusFixtureInput()); !errors.Is(err, context.Canceled) || len(*methods) != 0 {
		t.Fatal("aborted workflow performed requests")
	}
}

func workflowJSON(t *testing.T, value interface{}) *http.Response {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: ioutil.NopCloser(strings.NewReader(string(body)))}
}

func TestWorkflowAdmissionVectors(t *testing.T) {
	data, err := ioutil.ReadFile("workflow-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Admission struct {
			AssemblyID                                              string
			AcceptedOrigins, RejectedOrigins, RejectedAssemblyPaths []string
		}
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	vectors := fixtures.Admission
	if len(vectors.AcceptedOrigins) == 0 || len(vectors.RejectedOrigins) == 0 || len(vectors.RejectedAssemblyPaths) == 0 {
		t.Fatal("missing admission vectors")
	}
	owner := "https://api2-owner.transloadit.com"
	type testCase struct {
		url      string
		accepted bool
	}
	var cases []testCase
	for _, origin := range vectors.AcceptedOrigins {
		cases = append(cases, testCase{origin + "/assemblies/" + vectors.AssemblyID, true})
	}
	for _, origin := range vectors.RejectedOrigins {
		cases = append(cases, testCase{origin + "/assemblies/" + vectors.AssemblyID, false})
	}
	for _, path := range vectors.RejectedAssemblyPaths {
		cases = append(cases, testCase{owner + path, false})
	}
	for index, scenario := range cases {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			calls := 0
			client, err := NewClient(Config{BearerToken: "must-not-forward", HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.Header.Get("Authorization") != "" {
					t.Error("credential forwarded")
				}
				state := "ASSEMBLY_EXECUTING"
				if calls > 1 {
					state = "ASSEMBLY_CANCELED"
				}
				return workflowJSON(t, map[string]string{"assembly_id": vectors.AssemblyID, "assembly_ssl_url": scenario.url, "ok": state}), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.CancelAndWaitForAssembly(context.Background(), AssemblyWorkflowOptions{AssemblyID: vectors.AssemblyID, Interval: time.Millisecond})
			if scenario.accepted {
				if err != nil || result.GetOk() != "ASSEMBLY_CANCELED" || calls != 2 {
					t.Fatalf("admission failed: %v calls=%d", err, calls)
				}
			} else if err == nil || calls != 1 {
				t.Fatalf("followed rejected destination: err=%v calls=%d", err, calls)
			}
		})
	}
}

func TestWorkflowRejectsIncompleteOrChangedStatus(t *testing.T) {
	id := "11111111111111111111111111111111"
	owner := "https://api2-owner.transloadit.com/assemblies/" + id
	for index, change := range []map[string]interface{}{
		{"assembly_id": strings.Repeat("b", 32), "ok": "ASSEMBLY_COMPLETED"},
		{"assembly_id": nil}, {"assembly_ssl_url": nil}, {"ok": "UNKNOWN_STATE"},
		{"error": "FILE_FILTER_DECLINED_FILE"},
	} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			calls := 0
			client, err := NewClient(Config{BearerToken: "synthetic", HTTPClient: &http.Client{Transport: roundTripFunction(func(*http.Request) (*http.Response, error) {
				calls++
				status := map[string]interface{}{"assembly_id": id, "assembly_ssl_url": owner, "ok": "ASSEMBLY_EXECUTING"}
				for key, value := range change {
					status[key] = value
				}
				return workflowJSON(t, status), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.CancelAndWaitForAssembly(context.Background(), AssemblyWorkflowOptions{AssemblyID: id})
			if err == nil || calls != 1 {
				t.Fatalf("invalid status followed: err=%v calls=%d", err, calls)
			}
		})
	}
}

func TestWorkflowDeadlinesAndNoRetry(t *testing.T) {
	id := "11111111111111111111111111111111"
	owner := "https://api2-owner.transloadit.com/assemblies/" + id
	for _, mode := range []string{"before", "late", "active-cancel", "dropped-write", "changed-owner"} {
		t.Run(mode, func(t *testing.T) {
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			if mode == "before" {
				stop()
			}
			calls, deletes := 0, 0
			dropped := errors.New("synthetic dropped write")
			client, err := NewClient(Config{BearerToken: "synthetic", HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.Method == "DELETE" {
					deletes++
					if mode == "dropped-write" {
						return nil, dropped
					}
				}
				status := map[string]string{"assembly_id": id, "assembly_ssl_url": owner, "ok": "ASSEMBLY_EXECUTING"}
				if mode == "late" {
					time.Sleep(30 * time.Millisecond)
					status["ok"] = "ASSEMBLY_COMPLETED"
				}
				if mode == "changed-owner" && calls > 1 {
					status["assembly_ssl_url"] = "https://api2-other.transloadit.com/assemblies/" + id
				}
				return workflowJSON(t, status), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			timeout := 200 * time.Millisecond
			if mode == "late" {
				timeout = time.Millisecond
			}
			_, err = client.CancelAndWaitForAssembly(ctx, AssemblyWorkflowOptions{AssemblyID: id, Timeout: timeout, Interval: time.Millisecond})
			switch mode {
			case "before":
				if !errors.Is(err, context.Canceled) || calls != 0 {
					t.Fatalf("aborted: %v calls=%d", err, calls)
				}
			case "late", "active-cancel":
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("deadline: %v", err)
				}
				if mode == "active-cancel" && deletes != 1 {
					t.Fatal("cancellation was retried")
				}
			case "dropped-write":
				if !errors.Is(err, dropped) || deletes != 1 {
					t.Fatalf("retried write: %v deletes=%d", err, deletes)
				}
			case "changed-owner":
				if err == nil || calls != 2 {
					t.Fatalf("changed owner followed: %v calls=%d", err, calls)
				}
			}
		})
	}
}

func TestWorkflowDoesNotSendCookieJar(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	id := "11111111111111111111111111111111"
	endpoint, err := url.Parse("https://api2-owner.transloadit.com")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(endpoint, []*http.Cookie{{Name: "private", Value: "must-not-forward"}})
	client, err := NewClient(Config{Origin: endpoint.String(), BearerToken: "synthetic", HTTPClient: &http.Client{Jar: jar, Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Cookie") != "" || request.Header.Get("Authorization") != "" {
			t.Error("ambient credential sent")
		}
		return workflowJSON(t, map[string]string{"assembly_id": id, "ok": "ASSEMBLY_COMPLETED"}), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.WaitForAssembly(context.Background(), AssemblyWorkflowOptions{AssemblyID: id}); err != nil {
		t.Fatal(err)
	}
	if len(jar.Cookies(endpoint)) != 1 {
		t.Fatal("caller cookie jar changed")
	}
}

func TestWorkflowRejectsInvalidConfiguredOrigins(t *testing.T) {
	for _, origin := range []string{
		"https://user:secret@[::1]", "https://[::1]?query=1", "https://[::1]#fragment",
		"https://[::1]/proxy", "http://[2001:db8::1]",
	} {
		t.Run(origin, func(t *testing.T) {
			if _, err := NewClient(Config{BearerToken: "synthetic", AssemblyOrigins: []string{origin}}); err == nil {
				t.Fatal("invalid uploader configuration accepted")
			}
		})
	}
}

func TestWorkflowNullableTerminalError(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprint(cancel), func(t *testing.T) {
			id := strings.Repeat("a", 32)
			client, err := NewClient(Config{BearerToken: "synthetic", HTTPClient: &http.Client{Transport: roundTripFunction(func(*http.Request) (*http.Response, error) {
				return workflowJSON(t, map[string]interface{}{"assembly_id": id, "error": "FILE_FILTER_DECLINED_FILE", "ok": nil}), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			workflow := client.WaitForAssembly
			if cancel {
				workflow = client.CancelAndWaitForAssembly
			}
			result, err := workflow(context.Background(), AssemblyWorkflowOptions{AssemblyID: id})
			if err != nil {
				t.Fatal(err)
			}
			if result.WithError == nil || result.WithError.Error != "FILE_FILTER_DECLINED_FILE" {
				t.Fatal("terminal processing error lost")
			}
		})
	}
}

func TestWorkflowExplicitIPv6Owner(t *testing.T) {
	for _, origin := range []string{"http://[::1]:8081", "https://[2001:db8::1]"} {
		for _, trusted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/trusted=%t", origin, trusted), func(t *testing.T) {
				id := strings.Repeat("a", 32)
				owner := origin + "/assemblies/" + id
				var requests []string
				config := Config{Origin: "https://entry.example.com", BearerToken: "synthetic", HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
					requests = append(requests, request.Method+" "+request.URL.String())
					if request.Header.Get("Authorization") != "" {
						t.Error("credential forwarded")
					}
					state := "ASSEMBLY_EXECUTING"
					if len(requests) == 3 {
						state = "ASSEMBLY_CANCELED"
					}
					return workflowJSON(t, map[string]string{"assembly_id": id, "assembly_ssl_url": owner, "ok": state}), nil
				})}}
				if trusted {
					config.AssemblyOrigins = []string{origin}
				}
				client, err := NewClient(config)
				if err != nil {
					t.Fatal(err)
				}
				result, err := client.CancelAndWaitForAssembly(context.Background(), AssemblyWorkflowOptions{AssemblyID: id, Interval: time.Millisecond})
				if !trusted {
					if err == nil || len(requests) != 1 {
						t.Fatalf("followed untrusted uploader: err=%v requests=%v", err, requests)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if result.GetOk() != "ASSEMBLY_CANCELED" {
					t.Fatal("missing terminal status")
				}
				expected := []string{"GET https://entry.example.com/assemblies/" + id, "DELETE " + owner, "GET " + owner}
				if strings.Join(requests, "\n") != strings.Join(expected, "\n") {
					t.Fatalf("requests: %v; expected %v", requests, expected)
				}
			})
		}
	}
}

func TestWorkflowPreservesConfiguredEndpoint(t *testing.T) {
	id := "11111111111111111111111111111111"
	for _, endpoint := range []string{
		"https://example.com/proxy", "https://example.com/a%20b",
		"http://[::1]:8080", "http://127.0.0.2", "http://localhost.",
	} {
		for _, publicOwner := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/public=%t", endpoint, publicOwner), func(t *testing.T) {
				owner := endpoint + "/assemblies/" + id
				if publicOwner {
					owner = "https://api2-owner.transloadit.com/assemblies/" + id
				}
				var requests []string
				client, err := NewClient(Config{Origin: endpoint, BearerToken: "synthetic", HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
					requests = append(requests, request.Method+" "+request.URL.String())
					state := "ASSEMBLY_EXECUTING"
					if len(requests) == 3 {
						state = "ASSEMBLY_CANCELED"
					}
					return workflowJSON(t, map[string]string{"assembly_id": id, "assembly_ssl_url": owner, "ok": state}), nil
				})}})
				if err != nil {
					t.Fatal(err)
				}
				result, err := client.CancelAndWaitForAssembly(context.Background(), AssemblyWorkflowOptions{AssemblyID: id, Interval: time.Millisecond})
				if err != nil {
					t.Fatal(err)
				}
				if result.GetOk() != "ASSEMBLY_CANCELED" {
					t.Fatal("missing terminal status")
				}
				expected := []string{"GET " + endpoint + "/assemblies/" + id, "DELETE " + owner, "GET " + owner}
				if strings.Join(requests, "\n") != strings.Join(expected, "\n") {
					t.Fatalf("requests: %v; expected %v", requests, expected)
				}
			})
		}
	}
}

func TestWorkflowNormalizesConfiguredAuthority(t *testing.T) {
	for _, scenario := range []struct{ entry, trusted, owner, expectedEntry string }{
		{"https://UPLOADER.EXAMPLE:443", "", "https://uploader.example", "https://uploader.example"},
		{"https://UPLOADER.EXAMPLE:443/proxy", "", "https://uploader.example/proxy", "https://uploader.example/proxy"},
		{"https://entry.example", "https://UPLOADER.EXAMPLE:443", "https://uploader.example", "https://entry.example"},
		{"http://LOCALHOST:80/proxy", "", "http://localhost/proxy", "http://localhost/proxy"},
		{"https://[2001:DB8::1]:443/proxy", "", "https://[2001:db8::1]/proxy", "https://[2001:db8::1]/proxy"},
		{"https://entry.example", "https://[2001:DB8::1]:443", "https://[2001:db8::1]", "https://entry.example"},
	} {
		for _, cancel := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/cancel=%t", scenario.entry, scenario.trusted, cancel), func(t *testing.T) {
				id := strings.Repeat("a", 32)
				path := "/assemblies/" + id
				var requests []string
				terminalCall := 2
				if cancel {
					terminalCall = 3
				}
				config := Config{Origin: scenario.entry, BearerToken: "synthetic", HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
					requests = append(requests, request.Method+" "+request.URL.String())
					state := "ASSEMBLY_EXECUTING"
					if len(requests) == terminalCall {
						state = "ASSEMBLY_CANCELED"
					}
					return workflowJSON(t, map[string]string{"assembly_id": id, "assembly_ssl_url": scenario.owner + path, "ok": state}), nil
				})}}
				if scenario.trusted != "" {
					config.AssemblyOrigins = []string{scenario.trusted}
				}
				client, err := NewClient(config)
				if err != nil {
					t.Fatal(err)
				}
				workflow := client.WaitForAssembly
				expected := []string{"GET " + scenario.expectedEntry + path}
				if cancel {
					workflow = client.CancelAndWaitForAssembly
					expected = append(expected, "DELETE "+scenario.owner+path)
				}
				expected = append(expected, "GET "+scenario.owner+path)
				result, err := workflow(context.Background(), AssemblyWorkflowOptions{AssemblyID: id, Interval: time.Millisecond})
				if err != nil {
					t.Fatal(err)
				}
				if result.GetOk() != "ASSEMBLY_CANCELED" {
					t.Fatal("missing terminal status")
				}
				if strings.Join(requests, "\n") != strings.Join(expected, "\n") {
					t.Fatalf("requests: %v; expected %v", requests, expected)
				}
			})
		}
	}
}

func TestWorkflowDoesNotInferProxyPrefixes(t *testing.T) {
	id := "11111111111111111111111111111111"
	for _, owner := range []string{
		"https://example.com/other/assemblies/", "https://api2-owner.transloadit.com/proxy/assemblies/",
		"https://example.com/proxy/../proxy/assemblies/", "https://example.com/%70roxy/assemblies/",
	} {
		t.Run(owner, func(t *testing.T) {
			calls := 0
			client, err := NewClient(Config{Origin: "https://example.com/proxy", BearerToken: "synthetic", HTTPClient: &http.Client{Transport: roundTripFunction(func(*http.Request) (*http.Response, error) {
				calls++
				return workflowJSON(t, map[string]string{"assembly_id": id, "assembly_ssl_url": owner + id, "ok": "ASSEMBLY_EXECUTING"}), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.CancelAndWaitForAssembly(context.Background(), AssemblyWorkflowOptions{AssemblyID: id}); err == nil || calls != 1 {
				t.Fatalf("followed undeclared prefix: err=%v calls=%d", err, calls)
			}
		})
	}
}

func TestWorkflowConfirmsCancellationHTTPError(t *testing.T) {
	id := strings.Repeat("a", 32)
	for _, terminal := range []bool{false, true} {
		t.Run(fmt.Sprint(terminal), func(t *testing.T) {
			var requests []string
			client, err := NewClient(Config{BearerToken: "synthetic", HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
				requests = append(requests, request.Method)
				if request.Method == "DELETE" || (terminal && len(requests) > 1) {
					response := workflowJSON(t, map[string]string{"assembly_id": id, "error": "ASSEMBLY_EXPIRED"})
					if request.Method == "DELETE" {
						response.StatusCode = 410
					}
					return response, nil
				}
				return workflowJSON(t, map[string]string{"assembly_id": id, "ok": "ASSEMBLY_EXECUTING", "assembly_ssl_url": "https://api2-owner.transloadit.com/assemblies/" + id}), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.CancelAndWaitForAssembly(context.Background(), AssemblyWorkflowOptions{AssemblyID: id})
			if terminal {
				if err != nil {
					t.Fatal(err)
				}
				if result.WithError == nil || result.WithError.Error != "ASSEMBLY_EXPIRED" {
					t.Fatal("missing terminal error")
				}
			} else {
				var responseError *ResponseError
				if !errors.As(err, &responseError) || responseError.Status != 410 {
					t.Fatalf("active status disguised cleanup: %v", err)
				}
			}
			if strings.Join(requests, ",") != "GET,DELETE,GET" {
				t.Fatalf("requests: %v", requests)
			}
		})
	}
}

func TestWorkflowRetriesRateLimitedReads(t *testing.T) {
	id := strings.Repeat("a", 32)
	for _, phase := range []string{"discovery", "poll"} {
		t.Run(phase, func(t *testing.T) {
			var calls []time.Time
			client, err := NewClient(Config{BearerToken: "synthetic", HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
				calls = append(calls, time.Now())
				if (phase == "discovery" && len(calls) == 1) || (phase == "poll" && len(calls) == 2) {
					response := workflowJSON(t, map[string]string{"error": "ASSEMBLY_STATUS_FETCHING_RATE_LIMIT_REACHED"})
					response.StatusCode = 429
					response.Header.Set("Retry-After", "1")
					return response, nil
				}
				state := "ASSEMBLY_COMPLETED"
				if phase == "poll" && len(calls) == 1 {
					state = "ASSEMBLY_EXECUTING"
				}
				return workflowJSON(t, map[string]string{"assembly_id": id, "ok": state, "assembly_ssl_url": "https://api2-owner.transloadit.com/assemblies/" + id}), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.WaitForAssembly(context.Background(), AssemblyWorkflowOptions{AssemblyID: id, Interval: time.Millisecond, Timeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			if result.GetOk() != "ASSEMBLY_COMPLETED" || len(calls) < 2 {
				t.Fatal("missing completion or retry")
			}
			if calls[len(calls)-1].Sub(calls[len(calls)-2]) < 990*time.Millisecond {
				t.Fatal("Retry-After ignored")
			}
		})
	}
}

func TestWorkflowBoundsReadRetries(t *testing.T) {
	id := strings.Repeat("a", 32)
	for _, mode := range []string{"deadline", "caller", "server-error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			calls := 0
			client, err := NewClient(Config{BearerToken: "synthetic", HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
				calls++
				response := workflowJSON(t, map[string]string{"error": "RATE_LIMIT_REACHED"})
				response.StatusCode = 429
				response.Header.Set("Retry-After", "999999999999999999999")
				if mode == "caller" {
					time.AfterFunc(10*time.Millisecond, stop)
				}
				if mode == "server-error" {
					response.Header.Del("Retry-After")
					response.StatusCode = 503
					if calls > 1 {
						response.StatusCode = 403
					}
				}
				return response, nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.WaitForAssembly(ctx, AssemblyWorkflowOptions{AssemblyID: id, Interval: time.Millisecond, Timeout: 200 * time.Millisecond})
			if mode == "server-error" {
				var responseError *ResponseError
				if calls != 2 || !errors.As(err, &responseError) || responseError.Status != 403 {
					t.Fatalf("server retry: %v calls=%d", err, calls)
				}
				return
			}
			want := context.DeadlineExceeded
			if mode == "caller" {
				want = context.Canceled
			}
			if !errors.Is(err, want) || calls != 1 {
				t.Fatalf("bounded retry: %v calls=%d", err, calls)
			}
		})
	}
}

func TestWorkflowRetryAfterMetadata(t *testing.T) {
	now := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, scenario := range []struct {
		header string
		want   time.Duration
	}{
		{"2", 2 * time.Second}, {now.Add(2 * time.Second).Format(http.TimeFormat), 2 * time.Second},
		{now.Add(-time.Second).Format(http.TimeFormat), 0}, {"0.5", 0}, {"-1", 0}, {"invalid", 0},
		{"999999999999999999999", time.Duration(1<<63 - 1)},
	} {
		if got := retryAfterDuration(scenario.header, now); got != scenario.want {
			t.Errorf("%s: %v != %v", scenario.header, got, scenario.want)
		}
	}
}
