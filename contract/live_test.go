package contract

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"testing"
	"time"
)

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
	original, yes := ":original", true
	template := CreateTemplateParams_Template{Object: &CreateTemplateParams_Template_Object{
		Steps: &CreateTemplateParams_Template_Object_Steps{
			AdditionalProperties: map[string]CreateTemplateParams_Template_Object_Steps_AdditionalProperty{
				"passed": {FileFilter: &CreateTemplateParams_Template_Object_Steps_AdditionalProperty_FileFilter{
					Robot: "/file/filter",
					Use: &CreateTemplateParams_Template_Object_Steps_AdditionalProperty_FileFilter_Use{
						Variant: &CreateAssemblyParams_Object2_Steps_AdditionalProperty_AudioArtwork_Use_Variant{String: &original},
					},
					Result: &CreateTemplateParams_Template_Object_Steps_AdditionalProperty_FileFilter_Result{
						Variant: &CreateAssemblyParams_Object2_Steps_AdditionalProperty_TransloaditImport1_ForceAccept_Variant{Boolean: &yes},
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
	uploaded, err := client.CreateAssembly(ctx, CreateAssemblyInput{Params: params, Files: map[string]UploadFile{"file": {Reader: ioutil.NopCloser(bytes.NewReader(file)), Filename: "smilie.gif"}}})
	if err != nil {
		t.Fatal(err)
	}
	status := uploaded
	assemblyID := status.GetAssemblyId()
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
	for status.GetOk() != "ASSEMBLY_COMPLETED" {
		if status.WithError != nil || status.GetOk() == "ASSEMBLY_CANCELED" {
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
		status = result
	}
	completed = true
	digest := md5.Sum(file)
	uploads, results := status.GetUploads(), status.GetResults()
	if uploads == nil || len(*uploads) != 1 || (*uploads)[0].GetMd5hash().GetString() != hex.EncodeToString(digest[:]) || results == nil || len(results.AdditionalProperties["passed"]) != 1 || results.AdditionalProperties["passed"][0].Md5hash.GetString() != hex.EncodeToString(digest[:]) {
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
	_, err = client.GetTemplate(ctx, GetTemplateInput{TemplateIdOrName: id})
	missing, ok := err.(*ResponseError)
	if !ok || missing.Status != 400 || missing.Code() != "TEMPLATE_NOT_FOUND" {
		t.Fatalf("expected a classified missing Template error, got %v", err)
	}
	t.Log("SdkHttpCanaryVerified")
}
