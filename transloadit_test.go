package transloadit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var ctx = context.Background()
var templatesSetup bool
var templateIDOptimizeResize string

func TestContractImportIsOptIn(t *testing.T) {
	goExecutable := filepath.Join(runtime.GOROOT(), "bin", "go")
	if !filepath.IsAbs(runtime.GOROOT()) {
		// Trimmed builds embed "go" on Go 1.15 and nothing on newer versions. Neither is a
		// usable toolchain root; preserve the caller's PATH instead of guessing a relative binary.
		resolved, err := exec.LookPath("go")
		if err != nil {
			t.Fatal(err)
		}
		goExecutable = resolved
	} else if runtime.GOOS == "windows" {
		goExecutable += ".exe"
	}
	output, err := exec.Command(goExecutable, "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dependency := range strings.Fields(string(output)) {
		if dependency == "github.com/transloadit/go-sdk/contract" {
			t.Fatal("legacy consumers must not compile the opt-in generated contract package")
		}
	}
}

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

// Workflow fixtures are produced once by API2. These tests exercise existing public SDK methods,
// not an adapter-provided implementation of signing, waiting, retries or resumption.
type workflowVector struct {
	ID                     string
	Kind                   string
	Filename               string
	Hex                    string
	ChunkSize              int
	InterruptAfterBytes    int
	LoseResponseAfterBytes int
	ResponseDelayMs        int
	Responses              []struct{ Ok, Error string }
	Expected               struct{ Ok, Error string }
}

type workflowFixture struct {
	Format      string
	Version     int
	Credentials struct{ Key, Secret string }
	AssemblyID  string
	Admission   json.RawMessage
	Cases       []workflowVector
	TusMetadata []struct {
		ID, Filename, Append string
		Values               map[string]string
		Accepted             bool
	}
	TusReceipts []struct {
		ID       string
		Changes  map[string]interface{}
		Count    int
		State    map[string]string
		Accepted bool
	}
	SmartCdn []struct {
		ID, Workspace, Template, Input, ExpectedURL string
		ExpiresAt                                   int64
		Params                                      url.Values
	}
}

func sharedWorkflows(t *testing.T) workflowFixture {
	t.Helper()
	data, err := ioutil.ReadFile("contract/workflow-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures workflowFixture
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixtures); err != nil {
		t.Fatal(err)
	}
	if fixtures.Format != "transloadit-sdk-workflow-vectors" || fixtures.Version != 1 {
		t.Fatal("unsupported shared workflow fixture version")
	}
	if len(fixtures.Cases) == 0 || len(fixtures.SmartCdn) == 0 {
		t.Fatal("shared workflow fixtures must not silently lose their cases")
	}
	ids := make(map[string]bool)
	checkID := func(id string) {
		if id == "" || ids[id] {
			t.Fatalf("empty or duplicated workflow case ID %q", id)
		}
		ids[id] = true
	}
	for _, scenario := range fixtures.Cases {
		checkID(scenario.ID)
	}
	for _, scenario := range fixtures.SmartCdn {
		checkID(scenario.ID)
	}
	return fixtures
}

func TestSharedWorkflowSmartCDN(t *testing.T) {
	fixtures := sharedWorkflows(t)
	client := NewClient(Config{AuthKey: fixtures.Credentials.Key, AuthSecret: fixtures.Credentials.Secret})
	for _, scenario := range fixtures.SmartCdn {
		scenario := scenario
		t.Run(scenario.ID, func(t *testing.T) {
			before, err := json.Marshal(scenario.Params)
			if err != nil {
				t.Fatal(err)
			}
			actual := client.CreateSignedSmartCDNUrl(SignedSmartCDNUrlOptions{
				Workspace: scenario.Workspace, Template: scenario.Template, Input: scenario.Input,
				URLParams: scenario.Params, ExpiresAt: time.Unix(0, scenario.ExpiresAt*int64(time.Millisecond)),
			})
			if actual != scenario.ExpectedURL {
				t.Errorf("shared signing vector differs:\nwant %s\n got %s", scenario.ExpectedURL, actual)
			}
			after, err := json.Marshal(scenario.Params)
			if err != nil || string(before) != string(after) {
				t.Fatal("signing mutated caller-owned query parameters")
			}
		})
	}
}
