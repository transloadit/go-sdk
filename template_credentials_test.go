package transloadit

import (
	"reflect"
	"testing"
)

func TestTemplateCredentials(t *testing.T) {
	t.Parallel()

	client := setup(t)
	templateCredentialName := generateTemplateName()

	templateCredentialPost := NewTemplateCredential()
	templateCredentialPost.Name = templateCredentialName
	templateCredentialPost.Type = "s3"
	templateCredentialContent := map[string]interface{}{
		"key":           "xyxy",
		"secret":        "xyxyxyxy",
		"bucket":        "mybucket.example.com",
		"bucket_region": "us-east-1",
	}
	templateCredentialPost.Content = templateCredentialContent

	// Step 1: Create a brand new templateCredential
	id, err := client.CreateTemplateCredential(ctx, templateCredentialPost)
	if err != nil {
		t.Error(err)
	}
	if id == "" {
		t.Error("no templateCredentialPost id returned")
	}

	// Step 2: Retrieve new templateCredential created and assert its properties
	var templateCredential TemplateCredential
	if templateCredential, err = client.GetTemplateCredential(ctx, id); err != nil {
		t.Error(err)
	}
	checkTemplateCredential(t, templateCredential, templateCredentialName, templateCredentialContent)

	// Step 3: List all Templated credentials and assume that the created templateCredential is present
	list, err := client.ListTemplateCredential(ctx, nil)
	if err != nil {
		t.Error(err)
	}
	found := false
	for _, cred := range list.TemplateCredential {
		if cred.ID == id {
			checkTemplateCredential(t, cred, templateCredentialName, templateCredentialContent)
			found = true
		}
	}
	if !found {
		t.Errorf("Created TemplateCredential not found id=%s", id)
	}

	// Step 4: Delete test templateCredential
	if err := client.DeleteTemplateCredential(ctx, id); err != nil {
		t.Error(err)
	}

	// Step 5: Assert templateCredential has been deleted
	_, err = client.GetTemplateCredential(ctx, id)
	if err.(RequestError).Code != "TEMPLATE_CREDENTIALS_NOT_READ" {
		t.Error("templateCredentialPost has not been deleted")
	}
}

func checkTemplateCredential(t *testing.T, cred TemplateCredential, templateCredentialName string, expected map[string]interface{}) {
	if cred.Name != templateCredentialName {
		t.Error("wrong templateCredentialPost name")
	}
	if cred.Type != "s3" {
		t.Error("wrong templateCredentialPost type")
	}
	if !reflect.DeepEqual(cred.Content, expected) {
		t.Errorf("Different in content expected=%+v . In response : %+v", expected, cred.Content)
	}
}
