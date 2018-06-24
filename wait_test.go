package transloadit

import (
	"context"
	"strings"
	"testing"
	"time"
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

	if info.AssemblyURL == "" {
		t.Fatal("response doesn't contain assembly_url")
	}

	finishedInfo, err := client.WaitForAssembly(ctx, info)
	if err != nil {
		t.Fatal(err)
	}

	// Assembly completed
	if finishedInfo.AssemblyID != info.AssemblyID {
		t.Fatal("unmatching assembly ids")
	}
}

func TestWaitForAssembly_Cancel(t *testing.T) {
	t.Parallel()
	client := setup(t)

	ctx, cancel := context.WithTimeout(ctx, 100*time.Nanosecond)
	defer cancel()

	_, err := client.WaitForAssembly(ctx, &AssemblyInfo{
		AssemblySSLURL: "https://api2.transloadit.com/assemblies/foo",
	})

	// Go 1.8 and Go 1.7 have different error messages if a request get canceled.
	// Therefore we test for both cases.
	// Sometimes, a "dial tcp: i/o timeout" error is thrown if the context times
	// out shortly before the dialing is started, see:
	// https://sourcegraph.com/github.com/golang/go@d6a27e8edcd992b36446c5021a3c7560d983e9a6/-/blob/src/net/dial.go#L123-125
	// Therefore we also accept i/o timeouts as errors here.
	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "request canceled") && !strings.Contains(err.Error(), "i/o timeout") {
		t.Fatalf("operation's deadline should be exceeded: %s", err)
	}
}
