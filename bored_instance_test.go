package transloadit

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	config := DefaultConfig
	config.AuthKey = "foo"
	config.AuthSecret = "bar"

	client, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}

	bored, err := client.getBoredInstance()
	if err != nil {
		t.Fatal(err)
	}

	if bored == "" {
		t.Fatal("no bored instance provided")
	}
}
