package transloadit

import (
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

	file2, err := os.Open("./fixtures/mona_lisa.jpg")
	if err != nil {
		t.Fatal(err)
	}

	assembly.AddReader("image", "lol_cat.jpg", file)
	assembly.AddReader("image2", "mona_lisa.jpg", file2)

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

	if info.ClientAgent != "Transloadit Go SDK v1" {
		t.Fatal("wrong user agent")
	}

	assemblyId = info.AssemblyId
	assemblyUrl = info.AssemblyUrl
}

func TestAssemblyFail(t *testing.T) {

	config := DefaultConfig
	config.AuthKey = "does not exist"
	config.AuthSecret = "does not matter"

	client, err := NewClient(&config)
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

	info, err := assembly.Upload()
	if err != nil {
		t.Fatal(err)
	}

	if info.Error != "GET_ACCOUNT_UNKNOWN_AUTH_KEY" {
		t.Fatal("reponse doesn't contain error message")
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

	client := setup(t)

	assembly := client.ReplayAssembly(assemblyId)

	assembly.NotifyUrl = "http://requestb.in/1kwp6lx1"
	assembly.ReparseTemplate()

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

	client := setup(t)

	assembly := client.CreateAssembly()

	assembly.TemplateId = "64c11b20308811e4b5548d4f316c150f"

	info, err := assembly.Upload()
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyId == "" {
		t.Fatal("response doesn't contain assembly_id")
	}

	if !strings.Contains(info.Params, "64c11b20308811e4b5548d4f316c150f") {
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

	_, err = client.CancelAssembly(info.AssemblyUrl)
	if err != nil {
		t.Fatal(err)
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
