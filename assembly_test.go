package transloadit

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

var assemblyURL string

func TestStartAssembly_Success(t *testing.T) {
	client := setup(t)
	assembly := NewAssembly()

	file, err := os.Open("./fixtures/lol_cat.jpg")
	if err != nil {
		t.Fatal(err)
	}

	assembly.AddReader("image", "lol_cat.jpg", file)
	assembly.AddFile("image2", "./fixtures/mona_lisa.jpg")

	assembly.AddStep("resize", map[string]interface{}{
		"robot":             "/image/resize",
		"width":             75,
		"height":            75,
		"resize_strategy":   "pad",
		"background":        "#000000",
		"imagemagick_stack": "v3.0.0",
	})

	assembly.NotifyURL = "https://example.com/"
	assembly.Fields["string_test"] = "foo"
	assembly.Fields["number_test"] = 100

	info, err := client.StartAssembly(ctx, assembly)
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyID == "" {
		t.Fatal("response doesn't contain assembly_id")
	}

	if info.NotifyURL != "https://example.com/" {
		t.Fatal("wrong notify url")
	}

	if info.Fields["string_test"] != "foo" {
		t.Fatal("wrong field string_test")
	}

	// Go's JSON package parses numbers as a float by default, so we
	// need 100 to be a float for comparison.
	if info.Fields["number_test"] != float64(100) {
		t.Fatal("wrong field number_test")
	}

	if len(info.Uploads) != 2 {
		t.Fatal("wrong number of uploads")
	}

	if info.Uploads[0].Name == "lol_cat.jpg" {
		if info.Uploads[0].Field != "image" {
			t.Fatal("wrong field name")
		}
	} else if info.Uploads[1].Name == "lol_cat.jpg" {
		if info.Uploads[1].Field != "image" {
			t.Fatal("wrong field name")
		}
	} else {
		t.Fatal("lol_cat.jpg not found in uploads")
	}

	assemblyURL = info.AssemblyURL
}

func TestGetAssembly(t *testing.T) {
	client := setup(t)
	assembly, err := client.GetAssembly(ctx, assemblyURL)
	if err != nil {
		t.Fatal(err)
	}

	if assembly.AssemblyID == "" {
		t.Fatal("assembly id not contained")
	}

	if assembly.AssemblyURL != assemblyURL {
		t.Fatal("assembly urls don't match")
	}
}

func TestGetAssemblyRejectsUntrustedURLWithoutSendingCredentials(t *testing.T) {
	requestReceived := make(chan struct{}, 1)
	untrustedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestReceived <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"assembly_id":"assembly-id"}`))
	}))
	defer untrustedServer.Close()

	configuredServer := httptest.NewServer(http.NotFoundHandler())
	defer configuredServer.Close()

	config := DefaultConfig
	config.AuthKey = "key"
	config.AuthSecret = "secret"
	config.Endpoint = configuredServer.URL
	client := NewClient(config)

	_, err := client.GetAssembly(ctx, untrustedServer.URL+"/assemblies/assembly-id")
	if err == nil {
		t.Fatal("expected an untrusted Assembly URL error")
	}
	select {
	case <-requestReceived:
		t.Fatal("sent a request to an untrusted Assembly URL")
	default:
	}
}

func TestGetAssemblyDoesNotAuthenticateNoAuthRequest(t *testing.T) {
	query := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		query <- request.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"assembly_id":"assembly-id"}`))
	}))
	defer server.Close()

	config := DefaultConfig
	config.AuthKey = "key"
	config.AuthSecret = "secret"
	config.Endpoint = server.URL
	client := NewClient(config)

	_, err := client.GetAssembly(ctx, server.URL+"/assemblies/assembly-id")
	if err != nil {
		t.Fatal(err)
	}
	if rawQuery := <-query; rawQuery != "" {
		t.Fatalf("expected an unauthenticated request, got query %q", rawQuery)
	}
}

func TestGetAssemblyAllowsHTTPSForConfiguredHostname(t *testing.T) {
	requested := false
	config := DefaultConfig
	config.AuthKey = "key"
	config.AuthSecret = "secret"
	config.Endpoint = "http://api2-devdock.transloadit.dev"
	client := NewClient(config)
	client.httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"assembly_id":"assembly-id"}`)),
		}, nil
	})

	_, err := client.GetAssembly(ctx, "https://api2-devdock.transloadit.dev/assemblies/assembly-id")
	if err != nil {
		t.Fatal(err)
	}
	if !requested {
		t.Fatal("expected an HTTPS request to the configured hostname")
	}
}

func TestStartAssembly_Failure(t *testing.T) {
	t.Parallel()

	config := DefaultConfig
	config.AuthKey = "does not exist"
	config.AuthSecret = "does not matter"

	client := NewClient(config)

	assembly := NewAssembly()

	file, err := os.Open("./fixtures/lol_cat.jpg")
	if err != nil {
		t.Fatal(err)
	}

	assembly.AddReader("image", "lol_cat.jpg", file)

	assembly.AddStep("resize", map[string]interface{}{
		"robot":             "/image/resize",
		"width":             75,
		"height":            75,
		"resize_strategy":   "pad",
		"background":        "#000000",
		"imagemagick_stack": "v3.0.0",
	})

	_, err = client.StartAssembly(ctx, assembly)
	reqErr := err.(RequestError)
	if reqErr.Code != "GET_ACCOUNT_UNKNOWN_AUTH_KEY" {
		t.Fatal("wrong error code in response")
	}
	if reqErr.Message == "" {
		t.Fatal("error message should not be empty")
	}
}

func TestStartAssembly_Template(t *testing.T) {
	setupTemplates(t)
	// Delete Template
	defer tearDownTemplate(t)
	client := setup(t)
	assembly := NewAssembly()

	assembly.TemplateID = templateIDOptimizeResize

	info, err := client.StartAssembly(ctx, assembly)
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyID == "" {
		t.Fatalf("response doesn't contain assembly_id. %s", info.Error)
	}

	if !strings.Contains(info.Params, templateIDOptimizeResize) {
		t.Fatal("template id not as parameter submitted")
	}
}

func TestStartAssemblyReplay(t *testing.T) {
	t.Parallel()

	client := setup(t)
	assembly := NewAssemblyReplay(assemblyURL)

	assembly.NotifyURL = "https://example.com/"
	assembly.ReparseTemplate = true

	assembly.AddStep("convert", map[string]interface{}{
		"robot": "/html/convert",
		"url":   "https://transloadit.com/",
	})

	info, err := client.StartAssemblyReplay(ctx, assembly)
	if err != nil {
		t.Fatal(err)
	}

	if info.Ok != "ASSEMBLY_REPLAYING" {
		t.Fatal("wrong status code returned")
	}

	if info.NotifyURL != "https://example.com/" {
		t.Fatal("wrong notify url")
	}
}

func TestCancelAssembly(t *testing.T) {
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

	info, err = client.CancelAssembly(ctx, info.AssemblyURL)
	if err != nil {
		t.Fatal(err)
	}

	if info.Ok != "ASSEMBLY_CANCELED" {
		t.Fatal("incorrect assembly status")
	}
}

func TestListAssemblies(t *testing.T) {
	t.Parallel()

	client := setup(t)

	assemblies, err := client.ListAssemblies(ctx, &ListOptions{
		PageSize: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(assemblies.Assemblies) < 3 {
		t.Fatal("wrong number of assemblies")
	}

	if assemblies.Count == 0 {
		t.Fatal("wrong count")
	}

	if assemblies.Assemblies[0].AssemblyID == "" {
		t.Fatal("wrong template name")
	}
}

func TestInteger_MarshalJSON(t *testing.T) {
	var info AssemblyInfo
	err := json.Unmarshal([]byte(`{"bytes_expected":55}`), &info)
	if err != nil {
		t.Fatal(err)
	}

	if info.BytesExpected != 55 {
		t.Fatal("wrong integer parsed")
	}

	err = json.Unmarshal([]byte(`{"bytes_expected":null}`), &info)
	if err != nil {
		t.Fatal(err)
	}

	if info.BytesExpected != 0 {
		t.Fatal("wrong default value for null")
	}

	err = json.Unmarshal([]byte(`{"bytes_expected":""}`), &info)
	if err != nil {
		t.Fatal(err)
	}

	if info.BytesExpected != 0 {
		t.Fatal("wrong default value for string")
	}
}

func TestAssemblyInfo_TusFields(t *testing.T) {
	t.Parallel()

	var info AssemblyInfo
	err := json.Unmarshal([]byte(`{
		"tus_url": "https://api2.example/resumable/files/",
		"uploads": [
			{
				"is_tus_file": true,
				"tus_upload_url": "https://api2.example/resumable/files/upload-id",
				"user_meta": {
					"hello": "world"
				}
			}
		],
		"results": {
			":original": [
				{
					"is_tus_file": false,
					"user_meta": {
						"hello": "world"
					}
				}
			]
		}
	}`), &info)
	if err != nil {
		t.Fatal(err)
	}

	if info.TUSURL != "https://api2.example/resumable/files/" {
		t.Fatal("wrong tus url")
	}
	if len(info.Uploads) != 1 || !info.Uploads[0].IsTUSFile {
		t.Fatal("wrong TUS upload marker")
	}
	if info.Uploads[0].TUSUploadURL != "https://api2.example/resumable/files/upload-id" {
		t.Fatal("wrong TUS upload url")
	}
	if info.Uploads[0].UserMeta["hello"] != "world" {
		t.Fatal("wrong upload user meta")
	}
	if info.Results[":original"][0].UserMeta["hello"] != "world" {
		t.Fatal("wrong result user meta")
	}
}
