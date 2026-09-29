package transloadit

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transloadit/go-sdk/contract"
)

func TestWaitForAssembly(t *testing.T) {
	t.Parallel()

	client := setup(t)

	assembly := NewAssembly()

	assembly.AddStep("convert", map[string]interface{}{
		"robot": "/html/convert",
		"url":   "https://transloadit.com/",
	})

	info, err := client.StartAssembly(ctx, assembly)
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyURL == "" {
		t.Fatal("response doesn't contain assembly_url")
	}

	finishedInfo, err := client.WaitForAssembly(ctx, info)
	if err != nil {
		t.Fatal(err)
	}

	// Assembly completed
	if finishedInfo.AssemblyID != info.AssemblyID {
		t.Fatal("unmatching assembly ids")
	}
}

func TestWaitForAssembly_Cancel(t *testing.T) {
	t.Parallel()
	client := setup(t)

	ctx, cancel := context.WithTimeout(ctx, 100*time.Nanosecond)
	defer cancel()

	_, err := client.WaitForAssembly(ctx, &AssemblyInfo{
		AssemblySSLURL: "https://api2.transloadit.com/assemblies/foo",
	})

	// Go 1.8 and Go 1.7 have different error messages if a request get canceled.
	// Therefore we test for both cases.
	// Sometimes, a "dial tcp: i/o timeout" error is thrown if the context times
	// out shortly before the dialing is started, see:
	// https://sourcegraph.com/github.com/golang/go@d6a27e8edcd992b36446c5021a3c7560d983e9a6/-/blob/src/net/dial.go#L123-125
	// Therefore we also accept i/o timeouts as errors here.
	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "request canceled") && !strings.Contains(err.Error(), "i/o timeout") {
		t.Fatalf("operation's deadline should be exceeded: %s", err)
	}
}

func TestSharedWorkflows(t *testing.T) {
	fixtures := sharedWorkflows(t)
	for _, scenario := range fixtures.Cases {
		scenario := scenario
		t.Run(scenario.ID, func(t *testing.T) {
			t.Parallel()
			if scenario.Kind == "resume" {
				if scenario.ID != "resume-interrupted-upload" {
					t.Fatal("unclassified resume scenario")
				}
				t.Skip("UNSUPPORTED: the public Go SDK has multipart upload, but no tus/resume API; not generated workflow proof")
			}
			var mu sync.Mutex
			requests, polls, deletes := 0, 0, 0
			var uploaded []byte
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if scenario.ResponseDelayMs > 0 {
					select {
					case <-time.After(time.Duration(scenario.ResponseDelayMs) * time.Millisecond):
					case <-r.Context().Done():
						return
					}
				}
				mu.Lock()
				defer mu.Unlock()
				state := AssemblyInfo{AssemblyID: fixtures.AssemblyID,
					AssemblyURL:    server.URL + "/assemblies/" + fixtures.AssemblyID,
					AssemblySSLURL: server.URL + "/assemblies/" + fixtures.AssemblyID,
					Ok:             "ASSEMBLY_UPLOADING"}
				if r.Method == "POST" && scenario.Kind == "upload" {
					if r.URL.Path != "/assemblies" {
						t.Error("unexpected Assembly upload path")
					}
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					defer r.MultipartForm.RemoveAll()
					file, header, err := r.FormFile("file")
					if err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					defer file.Close()
					if header.Filename != scenario.Filename {
						t.Error("upload filename changed")
					}
					uploaded, err = ioutil.ReadAll(file)
					if err != nil {
						t.Error(err)
					}
				} else if r.URL.Path != "/assemblies/"+fixtures.AssemblyID {
					t.Error("unexpected Assembly path")
				} else if r.Method == "DELETE" {
					deletes++
					state.Ok = "ASSEMBLY_CANCELED"
				} else if r.Method == "GET" {
					if scenario.Kind == "wait" {
						if polls >= len(scenario.Responses) {
							t.Error("SDK polled past the terminal response")
							w.WriteHeader(500)
							return
						}
						state.Ok, state.Error = scenario.Responses[polls].Ok, scenario.Responses[polls].Error
					} else if deletes > 0 {
						state.Ok = "ASSEMBLY_CANCELED"
					} else if scenario.Kind == "upload" {
						state.Ok = "ASSEMBLY_COMPLETED"
					} else if scenario.ResponseDelayMs > 0 {
						state.Ok = "ASSEMBLY_COMPLETED"
					}
					polls++
				} else {
					t.Error("unexpected workflow method")
				}
				w.Header().Set("Content-Type", "application/json")
				// Emit the shared scenario's actual wire fields, not zero-valued legacy SDK fields
				// such as null arrays that the public response contract never promises.
				body := map[string]string{"assembly_id": state.AssemblyID, "assembly_ssl_url": state.AssemblySSLURL, "assembly_url": state.AssemblyURL}
				if state.Error != "" {
					body["error"] = state.Error
				} else {
					body["ok"] = state.Ok
				}
				if err := json.NewEncoder(w).Encode(body); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client := NewClient(Config{AuthKey: fixtures.Credentials.Key, AuthSecret: fixtures.Credentials.Secret, Endpoint: server.URL})
			generated, err := contract.NewClient(contract.Config{AuthKey: fixtures.Credentials.Key, AuthSecret: fixtures.Credentials.Secret, Origin: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			input := contract.AssemblyWorkflowOptions{AssemblyID: fixtures.AssemblyID, Interval: time.Millisecond}
			deadline, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			switch scenario.Kind {
			case "wait", "cancel":
				var result *contract.AssemblyWorkflowResult
				if scenario.Kind == "cancel" {
					result, err = generated.CancelAndWaitForAssembly(deadline, input)
				} else {
					result, err = generated.WaitForAssembly(deadline, input)
				}
				if err != nil {
					t.Fatal(err)
				}
				failure := ""
				if result.WithError != nil {
					failure = string(result.WithError.Error)
				}
				if result.GetOk() != scenario.Expected.Ok || failure != scenario.Expected.Error {
					t.Fatalf("wrong terminal result: %#v", result)
				}
				mu.Lock()
				defer mu.Unlock()
				if scenario.Kind == "wait" && polls != len(scenario.Responses) {
					t.Fatalf("workflow returned early: %d/%d responses", polls, len(scenario.Responses))
				}
				if scenario.Kind == "cancel" && deletes != 1 {
					t.Fatalf("expected one cancellation, received %d", deletes)
				}
			case "abort", "deadline":
				// Leave time to reach the fixture even on loaded CI runners; delayed responses
				// still arrive well after this deadline.
				control, cancel := context.WithTimeout(deadline, 500*time.Millisecond)
				defer cancel()
				expected := context.DeadlineExceeded
				if scenario.Kind == "abort" {
					cancel()
					expected = context.Canceled
				}
				_, err := generated.WaitForAssembly(control, input)
				if !errors.Is(err, expected) {
					t.Fatalf("expected %v, received %v", expected, err)
				}
				mu.Lock()
				defer mu.Unlock()
				if scenario.Kind == "abort" && requests != 0 {
					t.Fatal("aborted SDK performed an HTTP request")
				}
				if scenario.Kind == "deadline" && requests == 0 {
					t.Fatal("deadline case never reached the HTTP server")
				}
			case "upload":
				data, err := hex.DecodeString(scenario.Hex)
				if err != nil {
					t.Fatal(err)
				}
				assembly := NewAssembly()
				assembly.AddReader("file", scenario.Filename, ioutil.NopCloser(bytes.NewReader(data)))
				result, err := client.StartAssembly(deadline, assembly)
				if err != nil {
					t.Fatal(err)
				}
				result, err = client.WaitForAssembly(deadline, result)
				if err != nil || result.Ok != "ASSEMBLY_COMPLETED" {
					t.Fatalf("upload did not complete: %v", err)
				}
				mu.Lock()
				defer mu.Unlock()
				if !bytes.Equal(data, uploaded) {
					t.Fatal("uploaded bytes changed")
				}
			default:
				t.Fatalf("unclassified shared workflow kind %q", scenario.Kind)
			}
		})
	}
}
