package transloadit

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

type boredInstanceResponse struct {
	Host string `json:"api2_host"`
}

type cachedBoredInstanceResponse struct {
	Uploaders []string `json:"uploaders"`
}

func (client *Client) getBoredInstance() (string, error) {
	var res boredInstanceResponse
	err := client.request("GET", "instances/bored", nil, &res)
	if err != nil || res.Host == "" {
		return client.getCachedBoredInstance()
	}

	return res.Host, nil
}

func (client *Client) getCachedBoredInstance() (string, error) {
	var res cachedBoredInstanceResponse
	err := client.request("GET", "http://infra-"+client.config.Region+".transloadit.com.s3.amazonaws.com/cached_instances.json", nil, &res)
	if err != nil {
		return "", fmt.Errorf("failed to get cached bored instance: %s", err)
	}

	if res.Uploaders == nil {
		return "", fmt.Errorf("failed to get cached bored instance: server responded without uploaders")
	}

	uploaders := res.Uploaders
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(uploaders))))
	if err != nil {
		return "", fmt.Errorf("failed to get cached bored instance: %s", err)
	}

	return uploaders[index.Sign()], nil
}
