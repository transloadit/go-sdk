package transloadit

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

var assemblyId string
var assemblyUrl string

func TestAssembly(t *testing.T) {
	client := setup(t)
	assembly := client.CreateAssembly()

	file, err := os.Open("./fixtures/lol_cat.jpg")
	if err != nil {
		t.Fatal(err)
	}

	assembly.AddReader("image", "lol_cat.jpg", file)
	assembly.AddFile("image2", "./fixtures/mona_lisa.jpg")

	assembly.AddStep("resize", map[string]interface{}{
		"robot":           "/image/resize",
		"width":           75,
		"height":          75,
		"resize_strategy": "pad",
		"background":      "#000000",
	})

	assembly.NotifyUrl = "http://requestb.in/1kwp6lx1"

	info, err := assembly.Upload()
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyId == "" {
		t.Fatal("response doesn't contain assembly_id")
	}

	if info.NotifyUrl != "http://requestb.in/1kwp6lx1" {
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

	expectedAgent := "Transloadit Go SDK v"
	if info.ClientAgent[:len(expectedAgent)] != expectedAgent {
		t.Fatal("wrong user agent")
	}
}

func TestAssemblyFail(t *testing.T) {
	config := DefaultConfig
	config.AuthKey = "does not exist"
	config.AuthSecret = "does not matter"

	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}

	assembly := client.CreateAssembly()

	file, err := os.Open("./fixtures/lol_cat.jpg")
	if err != nil {
		t.Fatal(err)
	}

	assembly.AddReader("image", "lol_cat.jpg", file)

	assembly.AddStep("resize", map[string]interface{}{
		"robot":           "/image/resize",
		"width":           75,
		"height":          75,
		"resize_strategy": "pad",
		"background":      "#000000",
	})

	_, err = assembly.Upload()
	reqErr := err.(RequestError)
	if reqErr.Code != "GET_ACCOUNT_UNKNOWN_AUTH_KEY" {
		t.Fatal("wrong error code in response")
	}
	if reqErr.Message == "" {
		t.Fatal("error message should not be empty")
	}
}

func TestAssemblyBlocking(t *testing.T) {
	client := setup(t)
	assembly := client.CreateAssembly()

	file, err := os.Open("./fixtures/lol_cat.jpg")
	if err != nil {
		t.Fatal(err)
	}

	assembly.AddReader("image", "lol_cat.jpg", file)

	assembly.AddStep("resize", map[string]interface{}{
		"robot":           "/image/resize",
		"width":           75,
		"height":          75,
		"resize_strategy": "pad",
		"background":      "#000000",
	})

	assembly.Blocking = true

	info, err := assembly.Upload()
	if err != nil {
		t.Fatal(err)
	}

	if info.Ok != "ASSEMBLY_COMPLETED" {
		t.Fatal("wrong assembly status")
	}

	if info.AssemblyId == "" {
		t.Fatal("response doesn't contain assembly_id")
	}

	if len(info.Uploads) != 1 {
		t.Fatal("wrong number of uploads")
	}

	assemblyId = info.AssemblyId
	assemblyUrl = info.AssemblyUrl
}

func TestGetAssembly(t *testing.T) {
	client := setup(t)
	assembly, err := client.GetAssembly(assemblyUrl)
	if err != nil {
		t.Fatal(err)
	}

	if assembly.AssemblyId == "" {
		t.Fatal("assembly id not contained")
	}

	if assembly.AssemblyUrl != assemblyUrl {
		t.Fatal("assembly urls don't match")
	}
}

func TestReplayAssembly(t *testing.T) {
	fmt.Println("Replaying assembly:", assemblyId, assemblyUrl)
	client := setup(t)
	assembly := client.ReplayAssembly(assemblyId)

	assembly.NotifyUrl = "http://requestb.in/1kwp6lx1"
	assembly.ReparseTemplate = true

	info, err := assembly.Start()
	if err != nil {
		t.Fatal(err)
	}

	if info.Ok != "ASSEMBLY_REPLAYING" {
		t.Fatal("wrong status code returned")
	}

	if info.NotifyUrl != "http://requestb.in/1kwp6lx1" {
		t.Fatal("wrong notify url")
	}
}

func TestReplayAssemblyBlocking(t *testing.T) {
	fmt.Println("Replaying assembly:", assemblyId, assemblyUrl)
	client := setup(t)
	assembly := client.ReplayAssembly(assemblyId)

	assembly.Blocking = true

	info, err := assembly.Start()
	if err != nil {
		t.Fatal(err)
	}

	if info.Ok != "ASSEMBLY_COMPLETED" {
		t.Fatal("wrong status code returned")
	}
}

func TestAssemblyUsingTemplate(t *testing.T) {
	setupTemplates(t)
	client := setup(t)
	assembly := client.CreateAssembly()

	assembly.TemplateId = templateIdOptimizeResize

	info, err := assembly.Upload()
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyId == "" {
		t.Fatal(fmt.Sprintf("response doesn't contain assembly_id. %s", info.Error))
	}

	if !strings.Contains(info.Params, templateIdOptimizeResize) {
		t.Fatal("template id not as parameter submitted")
	}
}

func TestCancelAssembly(t *testing.T) {
	client := setup(t)
	assembly := client.CreateAssembly()

	assembly.AddStep("import", map[string]interface{}{
		"robot": "/http/import",
		"url":   "http://mirror.nl.leaseweb.net/speedtest/10000mb.bin",
	})

	info, err := assembly.Upload()
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyUrl == "" {
		t.Fatal("response doesn't contain assembly_url")
	}

	info, err = client.CancelAssembly(info.AssemblyUrl)
	if err != nil {
		t.Fatal(err)
	}

	if info.Ok != "ASSEMBLY_CANCELED" {
		t.Fatal("incorrect assembly status")
	}
}

func TestListAssemblies(t *testing.T) {
	client := setup(t)

	assemblies, err := client.ListAssemblies(&ListOptions{
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

	if assemblies.Assemblies[0].AssemblyId == "" {
		t.Fatal("wrong template name")
	}
}
