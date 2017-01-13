package transloadit

import (
	"testing"
)

func TestWait(t *testing.T) {
	client := setup(t)

	assembly := client.CreateAssembly()

	assembly.AddStep("import", map[string]interface{}{
		"robot": "/http/import",
		"url":   "http://mirror.nl.leaseweb.net/speedtest/100mb.bin",
	})

	info, err := assembly.Upload()
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyUrl == "" {
		t.Fatal("response doesn't contain assembly_url")
	}

	waiter := client.WaitForAssembly(info.AssemblyUrl)

	select {
	case res := <-waiter.Response:
		// Assembly completed
		if res.AssemblyId != info.AssemblyId {
			t.Fatal("unmatching assembly ids")
		}
	case err := <-waiter.Error:
		// Error appeared
		t.Fatal(err)
	}
}
