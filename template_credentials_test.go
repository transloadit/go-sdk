package transloadit

import (
	"fmt"

	//	"fmt"
	"testing"
	"time"
)

func TestTemplateCredentials(t *testing.T) {
	t.Parallel()

	client := setup(t)
	templateCredentialName := generateTemplateName()

	templateCredential := NewTemplateCredential()
	templateCredential.Name = templateCredentialName
	templateCredential.Type = "s3"
	templateCredential.Content["key"] = "xyxy"
	templateCredential.Content["secret"] = "xyxyxyxy"
	templateCredential.Content["bucket"] = "mybucket.example.com"
	templateCredential.Content["bucket_region"] = "us-east-1"


	// Step 1: Create a brand new templateCredential
	id, err := client.CreateTemplateCredential(ctx, templateCredential)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Error("no templateCredential id returned")
	}

/*	// Step 2: Retrieve new templateCredential and assert it's properties
	if templateCredential, err = client.GetTemplateCredential(ctx, id); err != nil {
		t.Fatal(err)
	}

	if templateCredential.Name != templateCredentialName {
		t.Error("wrong templateCredential name")
	}
	if templateCredential.Type != "s3" {
		t.Error("wrong templateCredential type")
	}*/

	// Step 3: Delete templateCredential
	if err := client.DeleteTemplateCredential(ctx, id); err != nil {
		t.Fatal(err)
	}

	// Step 4: Assert templateCredential has been deleted
	//_, err = client.GetTemplateCredential(ctx, id)
	//if err.(RequestError).Code != "SERVER_404" {
	//	t.Error("templateCredential has not been deleted")
	//}
	time.Sleep(10 * time.Second)
	list, err := client.ListTemplateCredential(ctx, nil )
	if err!= nil {
		t.Error(err)
	}
    fmt.Printf("%d", len(list.TemplateCredential))
}