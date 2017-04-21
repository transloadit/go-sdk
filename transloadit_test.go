package transloadit

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

var ctx = context.Background()
var templatesSetup bool
var templateIDOptimizeResize string

func TestNewClient_MissingAuthKey(t *testing.T) {
	t.Parallel()

	defer func() {
		err := recover().(string)
		if !strings.Contains(err, "missing AuthKey") {
			t.Fatal("error should contain message")
		}
	}()

	_ = NewClient(DefaultConfig)
}

func TestNewClient_MissingAuthSecret(t *testing.T) {
	t.Parallel()

	defer func() {
		err := recover().(string)
		if !strings.Contains(err, "missing AuthSecret") {
			t.Fatal("error should contain message")
		}
	}()

	config := DefaultConfig
	config.AuthKey = "fooo"
	_ = NewClient(config)
}

func TestNewClient_Success(t *testing.T) {
	t.Parallel()

	config := DefaultConfig
	config.AuthKey = "fooo"
	config.AuthSecret = "bar"
	_ = NewClient(config)
}

func setup(t *testing.T) Client {
	config := DefaultConfig
	config.AuthKey = os.Getenv("TRANSLOADIT_KEY")
	config.AuthSecret = os.Getenv("TRANSLOADIT_SECRET")

	client := NewClient(config)

	return client
}

func setupTemplates(t *testing.T) {
	if templatesSetup {
		return
	}

	client := setup(t)

	template := NewTemplate()
	template.Name = "go-sdk-test"

	template.AddStep("optimize", map[string]interface{}{
		"robot": "/image/optimize",
		"use":   ":original",
	})
	template.AddStep("image/resize", map[string]interface{}{
		"background":      "#000000",
		"height":          75,
		"resize_strategy": "pad",
		"robot":           "/image/resize",
		"width":           75,
		"use":             "optimize",
	})

	id, err := client.CreateTemplate(ctx, template)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Created template 'go-sdk-test' (%s) for testing.\n", id)

	templateIDOptimizeResize = id

	templatesSetup = true
}
