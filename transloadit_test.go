package transloadit

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

var templatesSetup bool
var templateIdOptimizeResize string

func TestCreateClient(t *testing.T) {

	client, err := NewClient(&DefaultConfig)
	if client != nil {
		t.Fatal("client should be nil")
	}

	if !strings.Contains(err.Error(), "missing AuthKey") {
		t.Fatal("error should contain message")
	}

	config := DefaultConfig
	config.AuthKey = "fooo"
	client, err = NewClient(&config)
	if client != nil {
		t.Fatal("client should be nil")
	}

	if !strings.Contains(err.Error(), "missing AuthSecret") {
		t.Fatal("error should contain message")
	}

	config = DefaultConfig
	config.AuthKey = "fooo"
	config.AuthSecret = "bar"
	client, err = NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}

	if client == nil {
		t.Fatal("client should not be nil")
	}

}

func setup(t *testing.T) *Client {

	config := DefaultConfig
	config.AuthKey = os.Getenv("TRANSLOADIT_KEY")
	config.AuthSecret = os.Getenv("TRANSLOADIT_SECRET")

	client, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}

	return client

}

func setupTemplates(t *testing.T) {

	if templatesSetup {
		return
	}

	client := setup(t)

	template := NewTemplate("go-sdk-test")

	template.AddStep("optimize", map[string]interface{}{
		"png_tool": "optipng",
		"robot":    "/image/optimize",
	})
	template.AddStep("resize", map[string]interface{}{
		"background":      "#000000",
		"height":          75,
		"resize_strategy": "pad",
		"robot":           "/image/resize",
		"width":           75,
	})

	id, err := client.CreateTemplate(template)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Created template 'go-sdk-test' (%s) for testing.\n", id)

	templateIdOptimizeResize = id

	templatesSetup = true

}
