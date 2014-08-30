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

	assembly.AddReader("image", file)
	assembly.AddReader("image2", file2)

	assembly.AddStep("resize", map[string]interface{}{
		"robot":           "/image/resize",
		"width":           75,
		"height":          75,
		"resize_strategy": "pad",
		"background":      "#000000",
	})

	assembly.NotifyUrl = "http://requestb.in/1kwp6lx1"

	res, err := assembly.Upload()
	if err != nil {
		t.Fatal(err)
	}

	if res["assembly_id"] == nil {
		t.Fatal("response doesn't contain assembly_id")
	}

	if res["notify_url"] != "http://requestb.in/1kwp6lx1" {
		t.Fatal("wrong notify url")
	}

	if len(res["uploads"].([]interface{})) != 2 {
		t.Fatal("wrong number of uploads")
	}

	assemblyId = res["assembly_id"].(string)
	assemblyUrl = res["assembly_url"].(string)
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

	assembly.AddReader("image", file)

	assembly.AddStep("resize", map[string]interface{}{
		"robot":           "/image/resize",
		"width":           75,
		"height":          75,
		"resize_strategy": "pad",
		"background":      "#000000",
	})

	res, err := assembly.Upload()
	if err == nil {
		t.Fatal("no error returned")
	}

	if !strings.Contains(err.Error(), "GET_ACCOUNT_UNKNOWN_AUTH_KEY") {
		t.Fatal("error doesn't contain error message")
	}

	if res["error"] != "GET_ACCOUNT_UNKNOWN_AUTH_KEY" {
		t.Fatal("reponse doesn't contain error message")
	}

}

func TestGetAssembly(t *testing.T) {

	client := setup(t)

	assembly, err := client.GetAssembly(assemblyUrl)
	if err != nil {
		t.Fatal(err)
	}

	if assembly["assembly_url"] != assemblyUrl {
		t.Fatal("assembly urls don't match")
	}

}

func TestReplayAssembly(t *testing.T) {

	client := setup(t)

	assembly := client.ReplayAssembly(assemblyId)

	assembly.NotifyUrl = "http://requestb.in/1kwp6lx1"
	assembly.ReparseTemplate()

	res, err := assembly.Start()
	if err != nil {
		t.Fatal(err)
	}

	if res["ok"] != "ASSEMBLY_REPLAYING" {
		t.Fatal("wrong status code returned")
	}

}

func TestAssemblyUsingTemplate(t *testing.T) {

	client := setup(t)

	assembly := client.CreateAssembly()

	assembly.TemplateId = "64c11b20308811e4b5548d4f316c150f"

	res, err := assembly.Upload()
	if err != nil {
		t.Fatal(err)
	}

	if res["assembly_id"] == nil {
		t.Fatal("response doesn't contain assembly_id")
	}

	if !strings.Contains(res["params"].(string), "64c11b20308811e4b5548d4f316c150f") {
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

	res, err := assembly.Upload()
	if err != nil {
		t.Fatal(err)
	}

	if res["assembly_url"] == nil {
		t.Fatal("response doesn't contain assembly_url")
	}

	res, err = client.CancelAssembly(res["assembly_url"].(string))
	if err != nil {
		t.Fatal(err)
	}

}
