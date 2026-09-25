package contract

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"testing"
	"time"
)

func nativeJSONString(t *testing.T, value interface{}) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		t.Fatal(err)
	}
	return text
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
	client, err := NewClient(Config{Origin: origin, AuthKey: os.Getenv("API2_CONTRACT_TEST_KEY"), AuthSecret: os.Getenv("API2_CONTRACT_TEST_SECRET")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	template := `{"steps":{"passed":{"robot":"/file/filter","use":":original","result":true}}}`
	created, err := client.CreateTemplate(ctx, CreateTemplateInput{Params: CreateTemplateParams{Name: fmt.Sprintf("contract-go-%d", time.Now().UnixNano()), Template: CreateTemplateParams_Template{Choice2: &template}}})
	if err != nil {
		t.Fatal(err)
	}
	id := nativeJSONString(t, created.Id)
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
	if nativeJSONString(t, got.Id) != id {
		t.Fatal("wrong retrieved Template")
	}
	var listParams ListTemplatesParams
	encoded, err := json.Marshal(map[string]interface{}{"keywords": []string{created.Name}, "include_builtin": "none"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &listParams); err != nil {
		t.Fatal(err)
	}
	listed, err := client.ListTemplates(ctx, ListTemplatesInput{Params: listParams})
	if err != nil {
		t.Fatal(err)
	}
	if listed.Count != 1 {
		t.Fatal("query filter was not honored")
	}
	var builtinParams ListTemplatesParams
	if err := json.Unmarshal([]byte(`{"include_builtin":"exclusively-latest"}`), &builtinParams); err != nil {
		t.Fatal(err)
	}
	builtins, err := client.ListTemplates(ctx, ListTemplatesInput{Params: builtinParams})
	if err != nil {
		t.Fatal(err)
	}
	if len(builtins.Items) == 0 {
		t.Fatal("missing built-in Templates")
	}
	builtinID := nativeJSONString(t, builtins.Items[0].Id)
	builtin, err := client.GetTemplate(ctx, GetTemplateInput{TemplateIdOrName: builtinID})
	if err != nil {
		t.Fatal(err)
	}
	if nativeJSONString(t, builtin.Id) != builtinID {
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
	var params CreateAssemblyParams
	encoded, err = json.Marshal(map[string]interface{}{"template_id": id, "auth": map[string]int{"max_size": 10000000, "max_number_of_files": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &params); err != nil {
		t.Fatal(err)
	}
	uploaded, err := client.CreateAssembly(ctx, CreateAssemblyInput{Params: params, Files: map[string]UploadFile{"file": {Reader: bytes.NewReader(file), Filename: "smilie.gif"}}})
	if err != nil {
		t.Fatal(err)
	}
	// The typed union is deliberately not reduced to one happy-path response model.
	var status struct {
		AssemblyID string `json:"assembly_id"`
		OK         string `json:"ok"`
		Error      string `json:"error"`
		Uploads    []struct {
			MD5 string `json:"md5hash"`
		} `json:"uploads"`
		Results map[string][]struct {
			MD5 string `json:"md5hash"`
		} `json:"results"`
	}
	readStatus := func(value interface{}) {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &status); err != nil {
			t.Fatal(err)
		}
	}
	readStatus(uploaded)
	assemblyID := status.AssemblyID
	if assemblyID == "" {
		t.Fatal("missing Assembly identity")
	}
	completed := false
	defer func() {
		if !completed {
			cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
			defer stop()
			if _, err := client.CancelAssembly(cleanup, CancelAssemblyInput{AssemblyId: assemblyID}); err != nil {
				t.Error(err)
			}
		}
	}()
	for status.OK != "ASSEMBLY_COMPLETED" {
		if status.Error != "" || status.OK == "ASSEMBLY_CANCELED" {
			t.Fatal("Assembly processing failed")
		}
		select {
		case <-ctx.Done():
			t.Fatal("Assembly deadline exceeded")
		case <-time.After(250 * time.Millisecond):
		}
		result, err := client.GetAssembly(ctx, GetAssemblyInput{AssemblyId: assemblyID})
		if err != nil {
			t.Fatal(err)
		}
		readStatus(result)
	}
	completed = true
	digest := md5.Sum(file)
	if len(status.Uploads) != 1 || status.Uploads[0].MD5 != hex.EncodeToString(digest[:]) || len(status.Results["passed"]) != 1 || status.Results["passed"][0].MD5 != hex.EncodeToString(digest[:]) {
		t.Fatal("processed file digest mismatch")
	}
	removed, err := client.DeleteTemplate(ctx, DeleteTemplateInput{TemplateIdOrName: id})
	if err != nil {
		t.Fatal(err)
	}
	if removed.Ok != "TEMPLATE_DELETED" {
		t.Fatal("Template was not deleted")
	}
	deleted = true
	t.Log("SdkHttpCanaryVerified")
}
