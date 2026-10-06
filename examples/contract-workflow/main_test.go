package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/transloadit/go-sdk/contract"
)

func TestAssemblyPollingFailures(t *testing.T) {
	for _, test := range []struct {
		ok     string
		failed bool
	}{
		{"ASSEMBLY_UPLOADING", false},
		{"ASSEMBLY_EXECUTING", false},
		{"ASSEMBLY_REPLAYING", false},
		{"ASSEMBLY_COMPLETED", false},
		{"ASSEMBLY_CANCELED", true},
		{"REQUEST_ABORTED", true},
	} {
		t.Run(test.ok, func(t *testing.T) {
			var status contract.CreateAssemblyResult
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{"ok":%q}`, test.ok)), &status); err != nil {
				t.Fatal(err)
			}
			if got := assemblyFailed(&status); got != test.failed {
				t.Fatalf("failure classification: got %v, want %v", got, test.failed)
			}
		})
	}
	if !assemblyFailed(&contract.CreateAssemblyResult{WithError: &contract.CreateAssemblyResult_WithError{}}) {
		t.Fatal("an explicit error must stop polling")
	}
}

type exampleTransport func(*http.Request) (*http.Response, error)

func (send exampleTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return send(request)
}

func TestExampleCancelsAbortedAssembly(t *testing.T) {
	for key, value := range map[string]string{"TRANSLOADIT_KEY": "synthetic-example-key", "TRANSLOADIT_SECRET": "synthetic-example-secret", "TRANSLOADIT_SIGNATURE_ALGORITHM": "sha256"} {
		key, previous := key, os.Getenv(key)
		_, existed := os.LookupEnv(key)
		if err := os.Setenv(key, value); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(key, previous)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
	previousArgs, previousTransport := os.Args, http.DefaultTransport
	t.Cleanup(func() { os.Args, http.DefaultTransport = previousArgs, previousTransport })
	os.Args = []string{"contract-workflow", "main.go"}
	id, templateID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	owner := "https://api2-uploader.transloadit.com/assemblies/" + id
	requests := []string{}
	http.DefaultTransport = exampleTransport(func(request *http.Request) (*http.Response, error) {
		if request.Body != nil {
			_, _ = ioutil.ReadAll(request.Body)
			_ = request.Body.Close()
		}
		path := request.URL.Path
		requests = append(requests, request.Method+" "+path)
		body := map[string]interface{}{}
		switch {
		case strings.HasPrefix(path, "/templates"):
			ok := "TEMPLATE_CREATED"
			if request.Method == "GET" {
				ok = "TEMPLATE_FOUND"
			}
			if request.Method == "DELETE" {
				ok = "TEMPLATE_DELETED"
			}
			body = map[string]interface{}{"id": templateID, "ok": ok, "message": "Synthetic Template", "name": "synthetic", "content": map[string]interface{}{}, "assembly_status_expiry": nil, "transcoding_result_expiry": nil, "require_signature_auth": 0}
		case path == "/assemblies":
			body = map[string]interface{}{"assembly_id": id, "ok": "ASSEMBLY_UPLOADING"}
		case path == "/assemblies/"+id:
			ok := "REQUEST_ABORTED"
			if request.Method == "DELETE" {
				if request.URL.String() != owner {
					t.Errorf("cancellation missed the uploader: %s", request.URL)
				}
				ok = "ASSEMBLY_CANCELED"
			}
			body = map[string]interface{}{"assembly_id": id, "assembly_ssl_url": owner, "ok": ok}
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
		}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: ioutil.NopCloser(strings.NewReader(string(data)))}, nil
	})
	if err := run(); err == nil || err.Error() != "Assembly processing did not complete successfully" {
		t.Fatalf("unexpected example outcome: %v", err)
	}
	want := "POST /templates,GET /templates/" + templateID + ",POST /assemblies,GET /assemblies/" + id + ",GET /assemblies/" + id + ",DELETE /assemblies/" + id + ",DELETE /templates/" + templateID
	if strings.Join(requests, ",") != want {
		t.Fatalf("example skipped cleanup: %v", requests)
	}
}
