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
	"strconv"
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
			if scenario.Kind == "upload" || scenario.Kind == "resume" {
				testSharedContractUpload(t, fixtures, scenario)
				return
			}
			var mu sync.Mutex
			requests, polls, deletes := 0, 0, 0
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
				if r.URL.Path != "/assemblies/"+fixtures.AssemblyID {
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
			default:
				t.Fatalf("unclassified shared workflow kind %q", scenario.Kind)
			}
		})
	}
}

func testSharedContractUpload(t *testing.T, fixtures workflowFixture, scenario workflowVector) {
	t.Helper()
	data, err := hex.DecodeString(scenario.Hex)
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	interrupted, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	var received []byte
	var offsets []int
	creates, heads := 0, 0
	metadata := ""
	responseLost := false
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials sent to capability")
		}
		if r.Method == "GET" && r.URL.Path == "/assemblies/"+fixtures.AssemblyID {
			state := "ASSEMBLY_UPLOADING"
			if len(received) == len(data) {
				state = "ASSEMBLY_COMPLETED"
			}
			if err := json.NewEncoder(w).Encode(map[string]string{
				"assembly_id": fixtures.AssemblyID, "ok": state,
				"assembly_ssl_url": server.URL + "/assemblies/" + fixtures.AssemblyID,
				"tus_url":          server.URL + "/resumable/files/",
			}); err != nil {
				t.Error(err)
			}
			return
		}
		w.Header().Set("Tus-Resumable", "1.0.0")
		switch {
		case r.Method == "POST" && r.URL.Path == "/resumable/files":
			creates++
			if creates != 1 || r.Header.Get("Upload-Length") != strconv.Itoa(len(data)) {
				t.Error("incorrect or duplicate creation")
			}
			metadata = r.Header.Get("Upload-Metadata")
			w.Header().Set("Location", server.URL+"/resumable/files/one")
			w.WriteHeader(201)
		case r.Method == "HEAD" && r.URL.Path == "/resumable/files/one":
			heads++
			w.Header().Set("Upload-Offset", strconv.Itoa(len(received)))
			w.Header().Set("Upload-Length", strconv.Itoa(len(data)))
			w.Header().Set("Upload-Metadata", metadata)
			w.WriteHeader(200)
		case r.Method == "PATCH" && r.URL.Path == "/resumable/files/one":
			chunk, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			if r.Header.Get("Upload-Offset") != strconv.Itoa(len(received)) || r.Header.Get("Content-Type") != "application/offset+octet-stream" || len(chunk) > scenario.ChunkSize || len(chunk) == 0 {
				t.Error("incorrect PATCH")
			}
			offsets = append(offsets, len(received))
			received = append(received, chunk...)
			if scenario.Kind == "resume" && len(received) == scenario.InterruptAfterBytes {
				cancel()
				return
			}
			if !responseLost && scenario.LoseResponseAfterBytes == len(received) {
				responseLost = true
				hijacker, ok := w.(http.Hijacker)
				if !ok {
					t.Error("fixture cannot interrupt connection")
					return
				}
				connection, _, err := hijacker.Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				connection.Close()
				return
			}
			w.Header().Set("Upload-Offset", strconv.Itoa(len(received)))
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	config := contract.Config{Origin: server.URL, AuthKey: fixtures.Credentials.Key, AuthSecret: fixtures.Credentials.Secret}
	client, err := contract.NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	var saved contract.AssemblyUploadSession
	input := contract.AssemblyUploadOptions{
		AssemblyID: fixtures.AssemblyID, Reader: bytes.NewReader(data), Size: int64(len(data)),
		Filename: scenario.Filename, ChunkSize: int64(scenario.ChunkSize), RetryDelay: time.Millisecond,
		OnSession: func(session contract.AssemblyUploadSession) error { saved = session; return nil },
	}
	_, err = client.UploadAssemblyFile(interrupted, input)
	if scenario.Kind == "resume" {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected interruption, received %v", err)
		}
		mu.Lock()
		if len(received) != scenario.InterruptAfterBytes {
			t.Error("wrong interruption offset")
		}
		mu.Unlock()
		serialized, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		var restored contract.AssemblyUploadSession
		if err := json.Unmarshal(serialized, &restored); err != nil {
			t.Fatal(err)
		}
		fresh, err := contract.NewClient(config)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fresh.ResumeAssemblyFile(ctx, input, restored); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		count := len(offsets)
		mu.Unlock()
		if _, err := fresh.ResumeAssemblyFile(ctx, input, restored); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		if len(offsets) != count {
			t.Error("already completed upload sent more bytes")
		}
		mu.Unlock()
	} else if err != nil {
		t.Fatal(err)
	}
	finished, err := client.WaitForAssembly(ctx, contract.AssemblyWorkflowOptions{AssemblyID: fixtures.AssemblyID, Interval: time.Millisecond})
	if err != nil || finished.GetOk() != "ASSEMBLY_COMPLETED" {
		t.Fatalf("assembly did not complete: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if creates != 1 || heads == 0 || !bytes.Equal(data, received) {
		t.Fatal("upload identity, offset read or final bytes differ")
	}
	if scenario.Kind == "resume" && (len(offsets) < 2 || offsets[1] != scenario.InterruptAfterBytes) {
		t.Fatal("new client did not resume from server offset")
	}
	if scenario.LoseResponseAfterBytes > 0 && (len(offsets) < 2 || offsets[1] != scenario.LoseResponseAfterBytes || heads < 2) {
		t.Fatal("lost response did not recover with a fresh HEAD")
	}
}
