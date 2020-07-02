package transloadit

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
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
	// Append the current date to the template name to make them unique
	template.Name = "gosdk-" + time.Now().Format("06-01-02-15-04-05")

	template.AddStep("optimize", map[string]interface{}{
		"robot": "/image/optimize",
		"use":   ":original",
	})
	template.AddStep("image/resize", map[string]interface{}{
		"background":        "#000000",
		"height":            75,
		"resize_strategy":   "pad",
		"robot":             "/image/resize",
		"width":             75,
		"use":               "optimize",
		"imagemagick_stack": "v2.0.7",
	})

	id, err := client.CreateTemplate(ctx, template)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Created template '%s' (%s) for testing.\n", template.Name, id)

	templateIDOptimizeResize = id

	templatesSetup = true
}
