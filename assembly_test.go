package transloadit

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

var assemblyURL string

func TestStartAssembly_Success(t *testing.T) {
	client := setup(t)
	assembly := NewAssembly()

	file, err := os.Open("./fixtures/lol_cat.jpg")
	if err != nil {
		t.Fatal(err)
	}

	assembly.AddReader("image", "lol_cat.jpg", file)
	assembly.AddFile("image2", "./fixtures/mona_lisa.jpg")

	assembly.AddStep("resize", map[string]interface{}{
		"robot":             "/image/resize",
		"width":             75,
		"height":            75,
		"resize_strategy":   "pad",
		"background":        "#000000",
		"imagemagick_stack": "v2.0.7",
	})

	assembly.NotifyURL = "https://example.com/"

	info, err := client.StartAssembly(ctx, assembly)
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyID == "" {
		t.Fatal("response doesn't contain assembly_id")
	}

	if info.NotifyURL != "https://example.com/" {
		t.Fatal("wrong notify url")
	}

	if len(info.Uploads) != 2 {
		t.Fatal("wrong number of uploads")
	}

	if info.Uploads[0].Name == "lol_cat.jpg" {
		if info.Uploads[0].Field != "image" {
			t.Fatal("wrong field name")
		}
	} else if info.Uploads[1].Name == "lol_cat.jpg" {
		if info.Uploads[1].Field != "image" {
			t.Fatal("wrong field name")
		}
	} else {
		t.Fatal("lol_cat.jpg not found in uploads")
	}

	assemblyURL = info.AssemblyURL
}

func TestStartAssembly_Failure(t *testing.T) {
	t.Parallel()

	config := DefaultConfig
	config.AuthKey = "does not exist"
	config.AuthSecret = "does not matter"

	client := NewClient(config)

	assembly := NewAssembly()

	file, err := os.Open("./fixtures/lol_cat.jpg")
	if err != nil {
		t.Fatal(err)
	}

	assembly.AddReader("image", "lol_cat.jpg", file)

	assembly.AddStep("resize", map[string]interface{}{
		"robot":             "/image/resize",
		"width":             75,
		"height":            75,
		"resize_strategy":   "pad",
		"background":        "#000000",
		"imagemagick_stack": "v2.0.7",
	})

	_, err = client.StartAssembly(ctx, assembly)
	reqErr := err.(RequestError)
	if reqErr.Code != "GET_ACCOUNT_UNKNOWN_AUTH_KEY" {
		t.Fatal("wrong error code in response")
	}
	if reqErr.Message == "" {
		t.Fatal("error message should not be empty")
	}
}

func TestStartAssembly_Template(t *testing.T) {
	setupTemplates(t)
	client := setup(t)
	assembly := NewAssembly()

	assembly.TemplateID = templateIDOptimizeResize

	info, err := client.StartAssembly(ctx, assembly)
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyID == "" {
		t.Fatal(fmt.Sprintf("response doesn't contain assembly_id. %s", info.Error))
	}

	if !strings.Contains(info.Params, templateIDOptimizeResize) {
		t.Fatal("template id not as parameter submitted")
	}
}

func TestGetAssembly(t *testing.T) {
	client := setup(t)
	assembly, err := client.GetAssembly(ctx, assemblyURL)
	if err != nil {
		t.Fatal(err)
	}

	if assembly.AssemblyID == "" {
		t.Fatal("assembly id not contained")
	}

	if assembly.AssemblyURL != assemblyURL {
		t.Fatal("assembly urls don't match")
	}
}

func TestStartAssemblyReplay(t *testing.T) {
	t.Skip()
	t.Parallel()

	client := setup(t)
	assembly := NewAssemblyReplay(assemblyURL)

	assembly.NotifyURL = "https://example.com/"
	assembly.ReparseTemplate = true

	assembly.AddStep("convert", map[string]interface{}{
		"robot": "/html/convert",
		"url":   "https://transloadit.com/",
	})

	info, err := client.StartAssemblyReplay(ctx, assembly)
	if err != nil {
		t.Fatal(err)
	}

	if info.Ok != "ASSEMBLY_REPLAYING" {
		t.Fatal("wrong status code returned")
	}

	if info.NotifyURL != "https://example.com/" {
		t.Fatal("wrong notify url")
	}
}

func TestCancelAssembly(t *testing.T) {
	t.Parallel()

	client := setup(t)
	assembly := NewAssembly()

	assembly.AddStep("convert", map[string]interface{}{
		"robot": "/html/convert",
		"url":   "https://transloadit.com/",
	})

	info, err := client.StartAssembly(ctx, assembly)
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyURL == "" {
		t.Fatal("response doesn't contain assembly_url")
	}

	info, err = client.CancelAssembly(ctx, info.AssemblyURL)
	if err != nil {
		t.Fatal(err)
	}

	if info.Ok != "ASSEMBLY_CANCELED" {
		t.Fatal("incorrect assembly status")
	}
}

func TestListAssemblies(t *testing.T) {
	t.Parallel()

	client := setup(t)

	assemblies, err := client.ListAssemblies(ctx, &ListOptions{
		PageSize: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(assemblies.Assemblies) < 3 {
		t.Fatal("wrong number of assemblies")
	}

	if assemblies.Count == 0 {
		t.Fatal("wrong count")
	}

	if assemblies.Assemblies[0].AssemblyID == "" {
		t.Fatal("wrong template name")
	}
}
