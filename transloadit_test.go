package transloadit

import (
	"os"
	"strings"
	"testing"
)

func TestCreateClient(t *testing.T) {

	client, err := NewClient(&DefaultConfig)
	if client != nil {
		t.Fatal("client should be nil")
	}

	if !strings.Contains(err.Error(), "missing AuthKey") {
		t.Fatal("error should contain message")
	}

	config := DefaultConfig
	config.AuthKey = "fooo"
	client, err = NewClient(&config)
	if client != nil {
		t.Fatal("client should be nil")
	}

	if !strings.Contains(err.Error(), "missing AuthSecret") {
		t.Fatal("error should contain message")
	}

	config = DefaultConfig
	config.AuthKey = "fooo"
	config.AuthSecret = "bar"
	client, err = NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}

	if client == nil {
		t.Fatal("client should not be nil")
	}

}

func setup(t *testing.T) *Client {

	config := DefaultConfig
	config.AuthKey = os.Getenv("TRANSLOADIT_KEY")
	config.AuthSecret = os.Getenv("TRANSLOADIT_SECRET")

	client, err := NewClient(&config)
	if err != nil {
		t.Fatal(err)
	}

	return client

}
