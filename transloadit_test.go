package transloadit

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
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
	template.Name = generateTemplateName()

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
		"imagemagick_stack": "v3.0.0",
	})

	id, err := client.CreateTemplate(ctx, template)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Created template '%s' (%s) for testing.\n", template.Name, id)

	templateIDOptimizeResize = id
	templatesSetup = true
}

func tearDownTemplate(t *testing.T) {
	if !templatesSetup {
		return
	}

	client := setup(t)
	if err := client.DeleteTemplate(ctx, templateIDOptimizeResize); err != nil {
		t.Fatalf("Error to delete template %s: %s", templateIDOptimizeResize, err)
	}

	templateIDOptimizeResize = ""
	templatesSetup = false
}

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
var letters = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

func generateTemplateName() string {
	b := make([]rune, 16)
	for i := range b {
		b[i] = letters[seededRand.Intn(len(letters))]
	}
	return "gosdk-" + string(b)
}

func TestCreateSignedSmartCDNUrl(t *testing.T) {
	client := NewClient(Config{
		AuthKey:    "foo_key",
		AuthSecret: "foo_secret",
	})

	params := url.Values{}
	params.Add("foo", "bar")
	params.Add("aaa", "42") // This must be sorted before `foo`
	params.Add("aaa", "21")

	url := client.CreateSignedSmartCDNUrl(SignedSmartCDNUrlOptions{
		Workspace: "foo_workspace",
		Template:  "foo_template",
		Input:     "foo/input",
		URLParams: params,
		ExpiresAt: time.Date(2024, 5, 1, 1, 0, 0, 0, time.UTC),
	})

	expected := "https://foo_workspace.tlcdn.com/foo_template/foo%2Finput?aaa=42&aaa=21&auth_key=foo_key&exp=1714525200000&foo=bar&sig=sha256%3A9a8df3bb28eea621b46ec808a250b7903b2546be7e66c048956d4f30b8da7519"

	if url != expected {
		t.Errorf("Expected URL:\n%s\nGot:\n%s", expected, url)
	}
}
