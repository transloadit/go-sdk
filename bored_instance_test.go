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

func TestGetBoredInstanceFallback(t *testing.T) {
	config := DefaultConfig
	config.AuthKey = "auth_key"
	config.AuthSecret = "auth_secret"
	config.Endpoint = "http://doesnotexists/"

	client, err := NewClient(config)
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
