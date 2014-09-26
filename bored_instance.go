package transloadit

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

func (client *Client) getBoredInstance() (string, error) {
	obj, err := client.request("GET", "instances/bored", nil, nil)
	if err != nil {
		return client.getCachedBoredInstance()
	}

	if obj["api2_host"] == nil {
		return client.getCachedBoredInstance()
	}

	return obj["api2_host"].(string), nil
}

func (client *Client) getCachedBoredInstance() (string, error) {
	obj, err := client.request("GET", "http://infra-"+client.config.Region+".transloadit.com.s3.amazonaws.com/cached_instances.json", nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get cached bored instance: %s", err)
	}

	if obj["uploaders"] == nil {
		return "", fmt.Errorf("failed to get cached bored instance: server responded without uploaders")
	}

	uploaders := obj["uploaders"].([]interface{})
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(uploaders))))
	if err != nil {
		return "", fmt.Errorf("failed to get cached bored instance: %s", err)
	}

	return uploaders[index.Sign()].(string), nil
}
