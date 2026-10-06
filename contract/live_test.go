package contract

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestContractDevdockCleansFailedAssembly(t *testing.T) {
	id := strings.Repeat("1", 32)
	var mu sync.Mutex
	position, cancellations := 0, 0
	metadata, origin := "", ""
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		response.Header().Set("Content-Type", "application/json")
		body := map[string]interface{}{}
		switch request.URL.Path {
		case "/templates", "/templates/template", "/templates/builtin/test":
			body = map[string]interface{}{"id": "template", "name": "synthetic", "ok": "TEMPLATE_CREATED", "assembly_status_expiry": nil, "content": nil, "message": "Synthetic Template", "require_signature_auth": 1, "transcoding_result_expiry": nil}
			if request.Method == "GET" {
				body["ok"] = "TEMPLATE_FOUND"
			}
			if request.Method == "DELETE" {
				if request.Header.Get("Authorization") == "Bearer synthetic-token" {
					response.WriteHeader(403)
					body = map[string]interface{}{"error": "INSUFFICIENT_SCOPE"}
				} else {
					body = map[string]interface{}{"ok": "TEMPLATE_DELETED", "message": "Synthetic deletion"}
				}
			} else if request.Method == "GET" && request.URL.Path == "/templates" {
				itemID := "template"
				if strings.Contains(request.URL.Query().Get("params"), "exclusively-latest") {
					itemID = "builtin/test"
				}
				body = map[string]interface{}{"count": 1, "items": []map[string]interface{}{{"id": itemID, "content": nil}}}
			} else if request.URL.Path == "/templates/builtin/test" {
				body["id"] = "builtin/test"
			}
		case "/token":
			body = map[string]interface{}{"access_token": "synthetic-token", "expires_in": 3600, "scope": "templates:read", "token_type": "Bearer"}
		case "/assemblies", "/assemblies/" + id:
			code := "ASSEMBLY_UPLOADING"
			if position == 128 {
				code = "REQUEST_ABORTED"
			}
			if request.Method == "DELETE" {
				cancellations++
				code = "ASSEMBLY_CANCELED"
			}
			body = map[string]interface{}{"assembly_id": id, "ok": code, "assembly_ssl_url": origin + "/assemblies/" + id, "tus_url": origin + "/resumable/files/"}
		case "/resumable/files", "/resumable/files/", "/resumable/files/one":
			response.Header().Set("Tus-Resumable", "1.0.0")
			switch request.Method {
			case "POST":
				metadata = request.Header.Get("Upload-Metadata")
				response.Header().Set("Location", "/resumable/files/one")
				response.WriteHeader(201)
			case "HEAD":
				response.Header().Set("Upload-Length", "128")
				response.Header().Set("Upload-Offset", strconv.Itoa(position))
				response.Header().Set("Upload-Metadata", metadata)
			case "PATCH":
				data, err := ioutil.ReadAll(request.Body)
				if err != nil {
					t.Error(err)
					return
				}
				position += len(data)
				response.Header().Set("Upload-Offset", strconv.Itoa(position))
				response.WriteHeader(204)
			default:
				t.Errorf("unexpected tus method %s", request.Method)
			}
			return
		default:
			t.Errorf("unexpected canary route %s", request.URL.Path)
			response.WriteHeader(404)
		}
		if err := json.NewEncoder(response).Encode(body); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	origin = strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	file := filepath.Join(t.TempDir(), "input.gif")
	if err := ioutil.WriteFile(file, make([]byte, 128), 0600); err != nil {
		t.Fatal(err)
	}
	// Execute the actual canary in a child because its expected t.Fatal must not fail this test.
	command := exec.Command(os.Args[0], "-test.run=^TestContractDevdock$", "-test.count=1", "-test.timeout=10s")
	command.Env = append(os.Environ(), "API2_CONTRACT_TEST_ORIGIN="+origin, "API2_CONTRACT_TEST_CAPABILITY_ORIGIN="+origin,
		"API2_CONTRACT_TEST_KEY=synthetic-key", "API2_CONTRACT_TEST_SECRET=synthetic-secret", "API2_CONTRACT_TEST_FILE="+file)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Assembly processing failed") {
		t.Fatalf("canary did not reach the intended terminal failure: %v, %s", err, output)
	}
	mu.Lock()
	defer mu.Unlock()
	if cancellations != 1 {
		t.Fatalf("failed canary sent %d cancellations, want one", cancellations)
	}
}

func TestContractDevdock(t *testing.T) {
	origin := os.Getenv("API2_CONTRACT_TEST_ORIGIN")
	if origin == "" {
		t.Skip("requires API2's owned devdock fixture")
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Hostname() != "localhost" {
		t.Fatal("canary requires localhost")
	}
	capabilityOrigin := os.Getenv("API2_CONTRACT_TEST_CAPABILITY_ORIGIN")
	var interruptUpload context.CancelFunc
	configuration := Config{Origin: origin, AssemblyOrigins: []string{capabilityOrigin}, AuthKey: os.Getenv("API2_CONTRACT_TEST_KEY"), AuthSecret: os.Getenv("API2_CONTRACT_TEST_SECRET"), HTTPClient: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		target := request.URL.Scheme + "://" + request.URL.Host
		if target != origin && target != capabilityOrigin {
			return nil, fmt.Errorf("non-local canary destination")
		}
		response, err := http.DefaultTransport.RoundTrip(request)
		if err == nil && request.Method == "PATCH" && response.StatusCode == 204 && interruptUpload != nil {
			interruptUpload()
			interruptUpload = nil
		}
		return response, err
	})}}
	client, err := NewClient(configuration)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	original, yes := ":original", true
	template := CreateTemplateParams_Template{Object: &CreateTemplateParams_Template_Object{
		Steps: &AssemblySteps{
			AdditionalProperties: map[string]AssemblySteps_AdditionalProperty{
				"passed": {FileFilter: &AssemblySteps_AdditionalProperty_FileFilter{
					Robot: "/file/filter",
					Use: &AssemblySteps_AdditionalProperty_FileFilter_Use{
						Variant: &AssemblySteps_AdditionalProperty_FileFilter_Use_Variant{String: &original},
					},
					Result: &AssemblySteps_AdditionalProperty_FileFilter_Result{
						Variant: &AssemblySteps_AdditionalProperty_FileFilter_Result_Variant{Boolean: &yes},
					},
				}},
			},
		},
	}}
	created, err := client.CreateTemplate(ctx, CreateTemplateInput{Params: CreateTemplateParams{Name: fmt.Sprintf("contract-go-%d", time.Now().UnixNano()), Template: template}})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Id.GetString()
	if id == "" {
		t.Fatal("missing Template identity")
	}
	deleted := false
	defer func() {
		if !deleted {
			cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
			defer stop()
			if _, err := client.DeleteTemplate(cleanup, DeleteTemplateInput{TemplateIdOrName: id}); err != nil {
				t.Error(err)
			}
		}
	}()
	got, err := client.GetTemplate(ctx, GetTemplateInput{TemplateIdOrName: id})
	if err != nil {
		t.Fatal(err)
	}
	if got.Id.GetString() != id {
		t.Fatal("wrong retrieved Template")
	}
	none := ListTemplatesParams_IncludeBuiltin("none")
	listParams := ListTemplatesParams{Keywords: &ListTemplatesParams_Keywords{String: &created.Name}, IncludeBuiltin: &none}
	listed, err := client.ListTemplates(ctx, ListTemplatesInput{Params: listParams})
	if err != nil {
		t.Fatal(err)
	}
	if listed.Count != 1 {
		t.Fatal("query filter was not honored")
	}
	exclusively := ListTemplatesParams_IncludeBuiltin("exclusively-latest")
	builtinParams := ListTemplatesParams{IncludeBuiltin: &exclusively}
	builtins, err := client.ListTemplates(ctx, ListTemplatesInput{Params: builtinParams})
	if err != nil {
		t.Fatal(err)
	}
	if len(builtins.Items) == 0 {
		t.Fatal("missing built-in Templates")
	}
	builtinID := builtins.Items[0].Id.GetString()
	builtin, err := client.GetTemplate(ctx, GetTemplateInput{TemplateIdOrName: builtinID})
	if err != nil {
		t.Fatal(err)
	}
	if builtin.Id.GetString() != builtinID {
		t.Fatal("wrong built-in Template")
	}
	scope := "templates:read"
	token, err := client.IssueBearerToken(ctx, IssueBearerTokenInput{Body: IssueBearerTokenBody{GrantType: "client_credentials", Scope: &scope}})
	if err != nil {
		t.Fatal(err)
	}
	bearer, err := NewClient(Config{Origin: origin, BearerToken: token.AccessToken})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bearer.GetTemplate(ctx, GetTemplateInput{TemplateIdOrName: id}); err != nil {
		t.Fatal(err)
	}
	_, err = bearer.DeleteTemplate(ctx, DeleteTemplateInput{TemplateIdOrName: id})
	scopeErr, ok := err.(*ResponseError)
	if !ok || scopeErr.Status != 403 {
		t.Fatalf("expected insufficient scope, got %v", err)
	}
	file, err := ioutil.ReadFile(os.Getenv("API2_CONTRACT_TEST_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	maxSize, maxFiles := float64(10000000), Integer(1)
	params := CreateAssemblyParams{Object2: &CreateAssemblyParams_Object2{
		TemplateId: &id,
		Auth:       &CreateAssemblyParams_Object2_Auth{MaxSize: &maxSize, MaxNumberOfFiles: &maxFiles},
	}}
	uploaded, err := client.CreateAssembly(ctx, CreateAssemblyInput{Params: params, Fields: map[string]string{"num_expected_upload_files": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	status := uploaded
	assemblyID := status.GetAssemblyId()
	if assemblyID == "" {
		t.Fatal("missing Assembly identity")
	}
	finished := false
	defer func() {
		if !finished {
			cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
			defer stop()
			if _, err := client.CancelAndWaitForAssembly(cleanup, AssemblyWorkflowOptions{AssemblyID: assemblyID}); err != nil {
				t.Error(err)
			}
		}
	}()
	interrupted, stopUpload := context.WithCancel(ctx)
	defer stopUpload()
	interruptUpload = stopUpload
	var checkpoint AssemblyUploadSession
	uploadInput := AssemblyUploadOptions{
		AssemblyID: assemblyID, Reader: bytes.NewReader(file), Size: int64(len(file)), Filename: "smilie.gif", ChunkSize: 64,
		OnSession: func(_ context.Context, session AssemblyUploadSession) error { checkpoint = session; return nil },
	}
	if _, err := client.UploadAssemblyFile(interrupted, uploadInput); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected owned interruption: %v", err)
	}
	if checkpoint.UploadURL == "" {
		t.Fatal("missing resume checkpoint")
	}
	fresh, err := NewClient(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.ResumeAssemblyFile(ctx, uploadInput, checkpoint); err != nil {
		t.Fatal(err)
	}
	status, err = client.WaitForAssembly(ctx, AssemblyWorkflowOptions{AssemblyID: assemblyID, Interval: 250 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if status.GetOk() != "ASSEMBLY_COMPLETED" {
		t.Fatal("Assembly processing failed")
	}
	finished = true
	digest := md5.Sum(file)
	uploads, results := status.GetUploads(), status.GetResults()
	if uploads == nil || len(*uploads) != 1 || (*uploads)[0].GetMd5hash().GetString() != hex.EncodeToString(digest[:]) || results == nil || len(results.AdditionalProperties["passed"]) != 1 || results.AdditionalProperties["passed"][0].Md5hash.GetString() != hex.EncodeToString(digest[:]) {
		t.Fatal("processed file digest mismatch")
	}
	reconciliation := []string{}
	afterCleanupConfig := configuration
	afterCleanupConfig.HTTPClient = &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		reconciliation = append(reconciliation, request.Method)
		if request.Method == "HEAD" {
			if request.URL.String() != checkpoint.UploadURL {
				t.Fatal("wrong upload resource")
			}
			// Simulate only the missing temporary resource; the receipt comes from real API2.
			return &http.Response{StatusCode: 404, Header: http.Header{}, Body: ioutil.NopCloser(bytes.NewReader(nil))}, nil
		}
		if request.Method != "GET" {
			t.Fatal("reconciliation must never write")
		}
		return configuration.HTTPClient.Transport.RoundTrip(request)
	})}
	afterCleanup, err := NewClient(afterCleanupConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := afterCleanup.ResumeAssemblyFile(ctx, uploadInput, checkpoint); err != nil {
		t.Fatal(err)
	}
	if len(reconciliation) != 3 || reconciliation[0] != "GET" || reconciliation[1] != "HEAD" || reconciliation[2] != "GET" {
		t.Fatalf("wrong reconciliation requests: %v", reconciliation)
	}
	pending, err := client.CreateAssembly(ctx, CreateAssemblyInput{Params: params, Fields: map[string]string{"num_expected_upload_files": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	pendingID := pending.GetAssemblyId()
	if pendingID == "" {
		t.Fatal("missing pending Assembly ID")
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, err := client.CancelAndWaitForAssembly(cleanup, AssemblyWorkflowOptions{AssemblyID: pendingID}); err != nil {
			t.Error(err)
		}
	}()
	if pending.GetOk() != "ASSEMBLY_UPLOADING" {
		t.Fatal("expected an active Assembly")
	}
	canceled, err := client.CancelAndWaitForAssembly(ctx, AssemblyWorkflowOptions{AssemblyID: pendingID, Interval: 250 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if canceled.GetOk() != "ASSEMBLY_CANCELED" {
		t.Fatal("cancellation was not confirmed")
	}
	removed, err := client.DeleteTemplate(ctx, DeleteTemplateInput{TemplateIdOrName: id})
	if err != nil {
		t.Fatal(err)
	}
	if removed.Ok != "TEMPLATE_DELETED" {
		t.Fatal("Template was not deleted")
	}
	deleted = true
	_, err = client.GetTemplate(ctx, GetTemplateInput{TemplateIdOrName: id})
	missing, ok := err.(*ResponseError)
	if !ok || missing.Status != 400 || missing.Code() != "TEMPLATE_NOT_FOUND" {
		t.Fatalf("expected a classified missing Template error, got %v", err)
	}
	t.Log("SdkHttpCanaryVerified")
}
