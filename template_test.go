package transloadit

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTemplate(t *testing.T) {
	t.Parallel()

	client := setup(t)
	templateName := generateTemplateName()

	template := NewTemplate()
	template.Name = templateName
	template.RequireSignatureAuth = true
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
	template.Content.AdditionalProperties["notify_url"] = "https://example.com"

	// Step 1: Create a brand new template
	id, err := client.CreateTemplate(ctx, template)
	defer client.DeleteTemplate(ctx, id)
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

	if template.Name != templateName {
		t.Error("wrong template name")
	}
	if !template.RequireSignatureAuth {
		t.Error("require_signature_auth is not enabled")
	}
	if _, found := template.Content.Steps["resize"]; !found {
		t.Error("resize step missing")
	}
	if _, found := template.Content.Steps["optimize"]; !found {
		t.Error("optimize step missing")
	}
	if template.Content.AdditionalProperties["notify_url"] != "https://example.com" {
		t.Error("missing notify_url")
	}

	newTemplateName := generateTemplateName()
	template = NewTemplate()
	template.Name = newTemplateName
	template.AddStep("bar", map[string]interface{}{})
	template.AddStep("baz", map[string]interface{}{})
	template.Content.AdditionalProperties["allow_steps_override"] = true
	template.RequireSignatureAuth = false

	// Step 3: Update previously created template
	if err := client.UpdateTemplate(ctx, id, template); err != nil {
		t.Fatal(err)
	}

	// Step 4: Retrieve template again and assert edited properties
	if template, err = client.GetTemplate(ctx, id); err != nil {
		t.Fatal(err)
	}

	if template.Name != newTemplateName {
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
	if template.RequireSignatureAuth {
		t.Error("require_signature_auth was not disabled after an update")
	}
	if template.Content.AdditionalProperties["allow_steps_override"] != true {
		t.Error("missing allow_steps_override")
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

func TestTemplateContent_MarshalJSON(t *testing.T) {
	content := TemplateContent{
		Steps: map[string]interface{}{
			":original": map[string]interface{}{
				"robot": "/upload/handle",
			},
			"resize": map[string]interface{}{
				"robot": "/image/resize",
			},
		},
		AdditionalProperties: map[string]interface{}{
			"notify_url":           "https://example.com",
			"allow_steps_override": false,
		},
	}

	result, err := json.MarshalIndent(content, "", "	")
	if err != nil {
		t.Fatal(err)
	}

	// Go orders the keys of the JSON object lexicographically
	if string(result) != `{
	"allow_steps_override": false,
	"notify_url": "https://example.com",
	"steps": {
		":original": {
			"robot": "/upload/handle"
		},
		"resize": {
			"robot": "/image/resize"
		}
	}
}` {
		t.Fatal("wrong JSON for template content")
	}
}

func TestTemplateContent_UnmarshalJSON(t *testing.T) {
	var content TemplateContent

	err := json.Unmarshal([]byte(`{
	"steps": {
		":original": {
			"robot": "/upload/handle"
		},
		"resize": {
			"robot": "/image/resize"
		}
	},
	"allow_steps_override": false,
	"notify_url": "https://example.com"
}`), &content)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(
		content,
		TemplateContent{
			Steps: map[string]interface{}{
				":original": map[string]interface{}{
					"robot": "/upload/handle",
				},
				"resize": map[string]interface{}{
					"robot": "/image/resize",
				},
			},
			AdditionalProperties: map[string]interface{}{
				"allow_steps_override": false,
				"notify_url":           "https://example.com",
			},
		}) {
		t.Fatal("wrong template content for JSON")
	}
}
