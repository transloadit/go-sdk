package transloadit

import (
	"os"
	"strings"
	"testing"
)

var assemblyId string

func TestAssembly(t *testing.T) {

	config := DefaultConfig
	config.AuthKey = os.Getenv("AUTH_KEY")
	config.AuthSecret = os.Getenv("AUTH_SECRET")

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

	assemblyId = res["assembly_id"].(string)
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

	config := DefaultConfig
	config.AuthKey = "does not exist"
	config.AuthSecret = "does not matter"

	client, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}

	assembly, err := client.GetAssembly(assemblyId)
	if err != nil {
		t.Fatal(err)
	}

	if assembly["assembly_id"] != assemblyId {
		t.Fatal("assembly ids don't match")
	}

}

func TestReplayAssembly(t *testing.T) {

	config := DefaultConfig
	config.AuthKey = os.Getenv("AUTH_KEY")
	config.AuthSecret = os.Getenv("AUTH_SECRET")

	client, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}

	assembly := client.ReplayAssembly(assemblyId)

	assembly.NotifyUrl = "http://requestb.in/1kwp6lx1"

	res, err := assembly.Start()
	if err != nil {
		t.Fatal(err)
	}

	if res["ok"] != "ASSEMBLY_REPLAYING" {
		t.Fatal("wrong status code returned")
	}

}
