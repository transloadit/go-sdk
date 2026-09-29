package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/transloadit/go-sdk/contract"
)

func TestAssemblyPollingFailures(t *testing.T) {
	for _, test := range []struct {
		ok     string
		failed bool
	}{
		{"ASSEMBLY_UPLOADING", false},
		{"ASSEMBLY_EXECUTING", false},
		{"ASSEMBLY_REPLAYING", false},
		{"ASSEMBLY_COMPLETED", false},
		{"ASSEMBLY_CANCELED", true},
		{"REQUEST_ABORTED", true},
	} {
		t.Run(test.ok, func(t *testing.T) {
			var status contract.CreateAssemblyResult
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{"ok":%q}`, test.ok)), &status); err != nil {
				t.Fatal(err)
			}
			if got := assemblyFailed(&status); got != test.failed {
				t.Fatalf("failure classification: got %v, want %v", got, test.failed)
			}
		})
	}
	if !assemblyFailed(&contract.CreateAssemblyResult{WithError: &contract.CreateAssemblyResult_WithError{}}) {
		t.Fatal("an explicit error must stop polling")
	}
}
