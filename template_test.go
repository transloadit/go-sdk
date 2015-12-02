package transloadit

import (
	"testing"
)

var templateId string

func TestCreateTemplate(t *testing.T) {
	client := setup(t)

	template := NewTemplate("go-sdk-test-create-template")

	template.AddStep("resize", map[string]interface{}{
		"robot":           "/image/resize",
		"width":           75,
		"height":          75,
		"resize_strategy": "pad",
		"background":      "#000000",
	})

	template.AddStep("optimize", map[string]interface{}{
		"robot":    "/image/optimize",
	})

	id, err := client.CreateTemplate(template)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("no template id returned")
	}

	templateId = id
}

func TestGetTemplate(t *testing.T) {
	client := setup(t)

	template, err := client.GetTemplate(templateId)
	if err != nil {
		t.Fatal(err)
	}

	if template.Name != "go-sdk-test-create-template" {
		t.Fatal("wrong template name")
	}
	if _, found := template.Steps["resize"]; !found {
		t.Fatal("resize step missing")
	}
	if _, found := template.Steps["optimize"]; !found {
		t.Fatal("optimize step missing")
	}
}

func TestEditTemplate(t *testing.T) {
	client := setup(t)

	template := NewTemplate("go-sdk-test-new")

	template.AddStep("bar", map[string]interface{}{})
	template.AddStep("baz", map[string]interface{}{})

	err := client.EditTemplate(templateId, template)
	if err != nil {
		t.Fatal(err)
	}

	if template.Name != "go-sdk-test-new" {
		t.Fatal("wrong template name")
	}
	if _, found := template.Steps["resize"]; found {
		t.Fatal("resize step not removed")
	}
	if _, found := template.Steps["bar"]; !found {
		t.Fatal("bar step missing")
	}
	if _, found := template.Steps["baz"]; !found {
		t.Fatal("baz step missing")
	}
}

func TestDeleteTemplate(t *testing.T) {
	client := setup(t)

	if err := client.DeleteTemplate(templateId); err != nil {
		t.Fatal(err)
	}
}

func TestListTemplates(t *testing.T) {
	setupTemplates(t)
	client := setup(t)

	templates, err := client.ListTemplates(&ListOptions{
		PageSize: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(templates.Templates) == 0 {
		t.Fatal("wrong number of templates")
	}

	if templates.Count == 0 {
		t.Fatal("wrong count")
	}

	if templates.Templates[0].Name == "" {
		t.Fatal("wrong template name")
	}
}
