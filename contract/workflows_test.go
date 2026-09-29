package contract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"
)

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
