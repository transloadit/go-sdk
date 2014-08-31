package transloadit

import (
	"testing"
)

func TestGetBoredInstance(t *testing.T) {

	client := setup(t)

	bored, err := client.getBoredInstance()
	if err != nil {
		t.Fatal(err)
	}

	if bored == "" {
		t.Fatal("no bored instance provided")
	}
}

