package transloadit

import (
	"fmt"
)

func (client *Client) getBoredInstance() (string, error) {

	obj, err := client.request("GET", "instances/bored", nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get bored instance: %s", err)
	}

	if obj["api2_host"] == nil {
		return "", fmt.Errorf("failed to get bored instance: server responded without api2_host")
	}

	return obj["api2_host"].(string), nil

}
