package transloadit

import "testing"

func TestTemplate(t *testing.T) {
	t.Parallel()

	client := setup(t)

	template := NewTemplate()
	template.Name = "go-sdk-test-template"
	template.AddStep("resize", map[string]interface{}{
		"robot":             "/image/resize",
		"width":             75,
		"height":            75,
		"resize_strategy":   "pad",
		"background":        "#000000",
		"imagemagick_stack": "v2.0.7",
	})
	template.AddStep("optimize", map[string]interface{}{
		"robot": "/image/optimize",
	})

	// Step 1: Create a brand new template
	id, err := client.CreateTemplate(ctx, template)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Error("no template id returned")
	}

	// Step 2: Retrieve new template and assert it's properties
	if template, err = client.GetTemplate(ctx, id); err != nil {
		t.Fatal(err)
	}

	if template.Name != "go-sdk-test-template" {
		t.Error("wrong template name")
	}
	if _, found := template.Content.Steps["resize"]; !found {
		t.Error("resize step missing")
	}
	if _, found := template.Content.Steps["optimize"]; !found {
		t.Error("optimize step missing")
	}

	template = NewTemplate()
	template.Name = "go-sdk-test-new"
	template.AddStep("bar", map[string]interface{}{})
	template.AddStep("baz", map[string]interface{}{})

	// Step 3: Update previously created template
	if err := client.UpdateTemplate(ctx, id, template); err != nil {
		t.Fatal(err)
	}

	// Step 4: Retrieve template again and assert edited properties
	if template, err = client.GetTemplate(ctx, id); err != nil {
		t.Fatal(err)
	}

	if template.Name != "go-sdk-test-new" {
		t.Error("wrong template name")
	}
	if _, found := template.Content.Steps["resize"]; found {
		t.Error("resize step not removed")
	}
	if _, found := template.Content.Steps["bar"]; !found {
		t.Error("bar step missing")
	}
	if _, found := template.Content.Steps["baz"]; !found {
		t.Error("baz step missing")
	}

	// Step 5: Delete template
	if err := client.DeleteTemplate(ctx, id); err != nil {
		t.Fatal(err)
	}

	// Step 6: Assert template has been deleted
	_, err = client.GetTemplate(ctx, id)
	if err.(RequestError).Code != "TEMPLATE_NOT_FOUND" {
		t.Error("template has not been deleted")
	}
}

func TestListTemplates(t *testing.T) {
	t.Parallel()

	client := setup(t)

	templates, err := client.ListTemplates(ctx, &ListOptions{
		PageSize: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(templates.Templates) != 3 {
		t.Fatal("wrong number of templates")
	}

	if templates.Count == 0 {
		t.Fatal("wrong count")
	}

	if templates.Templates[0].Name == "" {
		t.Fatal("wrong template name")
	}

	if templates.Templates[0].Content.Steps == nil {
		t.Fatal("empty template content")
	}
}
