package transloadit

import (
	"fmt"
	"testing"
	"time"
)

func TestTemplateCredentials(t *testing.T) {
	t.Parallel()

	client := setup(t)
	templateCredentialName := generateTemplateName()

	templateCredentialPost := NewTemplateCredential()
	templateCredentialPost.Name = templateCredentialName
	templateCredentialPost.Type = "s3"
	templateCredentialPost.Content["key"] = "xyxy"
	templateCredentialPost.Content["secret"] = "xyxyxyxy"
	templateCredentialPost.Content["bucket"] = "mybucket.example.com"
	templateCredentialPost.Content["bucket_region"] = "us-east-1"

	// Step 1: Create a brand new templateCredentialPost
	id, err := client.CreateTemplateCredential(ctx, templateCredentialPost)
	if err != nil {
		t.Fatal(err)
	}
	defer client.DeleteTemplateCredential(ctx, id)
	if id == "" {
		t.Error("no templateCredentialPost id returned")
	}

	// Step 2: Retrieve new templateCredentialPost and assert it's properties
	var templateCredential TemplateCredential
	if templateCredential, err = client.GetTemplateCredential(ctx, id); err != nil {
		t.Fatal(err)
	}

	if templateCredential.Name != templateCredentialName {
		t.Error("wrong templateCredentialPost name")
	}
	if templateCredential.Type != "s3" {
		t.Error("wrong templateCredentialPost type")
	}

	// Step 3: Delete templateCredentialPost
	if err := client.DeleteTemplateCredential(ctx, id); err != nil {
		t.Fatal(err)
	}

	// Step 4: Assert templateCredentialPost has been deleted
	_, err = client.GetTemplateCredential(ctx, id)
	if err.(RequestError).Code != "TEMPLATE_CREDENTIALS_NOT_READ" {
		t.Error("templateCredentialPost has not been deleted")
	}
	time.Sleep(10 * time.Second)
	list, err := client.ListTemplateCredential(ctx, nil)
	if err != nil {
		t.Error(err)
	}
	fmt.Printf("%d", len(list.TemplateCredential))
}

func TestListTemplateCredentials(t *testing.T) {
	t.Parallel()
	client := setup(t)
	_, err := client.ListTemplateCredential(ctx, nil)
	if err != nil {
		t.Error(err)
	}
}
