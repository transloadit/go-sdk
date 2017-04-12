package transloadit

import (
	"testing"
)

func TestWaitForAssembly(t *testing.T) {
	t.Parallel()

	client := setup(t)

	assembly := NewAssembly()

	assembly.AddStep("import", map[string]interface{}{
		"robot": "/http/import",
		"url":   "http://mirror.nl.leaseweb.net/speedtest/100mb.bin",
	})

	info, err := client.StartAssembly(ctx, assembly)
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyUrl == "" {
		t.Fatal("response doesn't contain assembly_url")
	}

	finishedInfo, err := client.WaitForAssembly(ctx, info)
	if err != nil {
		t.Fatal(err)
	}

	// Assembly completed
	if finishedInfo.AssemblyId != info.AssemblyId {
		t.Fatal("unmatching assembly ids")
	}
}
