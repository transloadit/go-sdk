package transloadit

import (
	"fmt"
	"testing"
)

func TestWait(t *testing.T) {

	client := setup(t)

	assembly := client.CreateAssembly()

	assembly.AddStep("import", map[string]interface{}{
		"robot": "/http/import",
		"url":   "http://mirror.nl.leaseweb.net/speedtest/100mb.bin",
	})

	res, err := assembly.Upload()
	if err != nil {
		t.Fatal(err)
	}

	if res["assembly_url"] == nil {
		t.Fatal("response doesn't contain assembly_url")
	}

	watcher := client.WaitForAssembly(res["assembly_url"].(string))

	select {
	case res := <-watcher.Response:
		// Assembly completed
		fmt.Println(res)
	case err := <-watcher.Error:
		// Error appeared
		t.Fatal(err)
	}

}
