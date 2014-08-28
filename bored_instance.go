package transloadit

import (
	"fmt"
	"net/http"
)

func (client *Client) getBoredInstance() (string, error) {

	req, err := http.NewRequest("GET", client.config.Endpoint+"/instances/bored", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get bored instance: %s", err)
	}

	obj, err := client.doRequest(req)
	if err != nil {
		return "", fmt.Errorf("failed to get bored instance: %s", err)
	}

	if obj["api2_host"] == nil {
		return "", fmt.Errorf("failed to get bored instance: server responded without api2_host")
	}

	return obj["api2_host"].(string), nil

}
