package transloadit

import (
	"os"
	"testing"
)

var templateId string

func TestCreateTemplate(t *testing.T) {

	config := DefaultConfig
	config.AuthKey = os.Getenv("AUTH_KEY")
	config.AuthSecret = os.Getenv("AUTH_SECRET")

	client, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}

	template := NewTemplate("go-sdk-test")

	template.AddStep("resize", map[string]interface{}{
		"robot":           "/image/resize",
		"width":           75,
		"height":          75,
		"resize_strategy": "pad",
		"background":      "#000000",
	})

	template.AddStep("optimize", map[string]interface{}{
		"robot":    "/image/optimize",
		"png_tool": "optipng",
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

	config := DefaultConfig
	config.AuthKey = os.Getenv("AUTH_KEY")
	config.AuthSecret = os.Getenv("AUTH_SECRET")

	client, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}

	template, err := client.GetTemplate(templateId)
	if err != nil {
		t.Fatal(err)
	}
	if template.Name != "go-sdk-test" {
		t.Fatal("wrong template name")
	}
	if _, found := template.Steps["resize"]; !found {
		t.Fatal("resize step missing")
	}
	if _, found := template.Steps["optimize"]; !found {
		t.Fatal("optimize step missing")
	}
}

func TestDeleteTemplate(t *testing.T) {

	config := DefaultConfig
	config.AuthKey = os.Getenv("AUTH_KEY")
	config.AuthSecret = os.Getenv("AUTH_SECRET")

	client, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}

	if err = client.DeleteTemplate(templateId); err != nil {
		t.Fatal(err)
	}
}
