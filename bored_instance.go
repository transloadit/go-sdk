package transloadit

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

func (client *Client) getBoredInstance() (string, error) {

	res, err := http.Get(client.config.Endpoint + "/instances/bored")
	defer res.Body.Close()
	if err != nil {
		return "", fmt.Errorf("failed to get bored instance: %s", err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to get bored instance: %s", err)
	}

	var obj map[string]interface{}
	err = json.Unmarshal(body, &obj)
	if err != nil {
		return "", fmt.Errorf("failed to get bored instance: %s", err)
	}

	if res.StatusCode != 200 {
		return "", fmt.Errorf("failed to get bored instance: server responded with %s", obj["error"])
	}

	if obj["api2_host"] == nil {
		return "", fmt.Errorf("failed to get bored instance: server responded without api2_host")
	}

	return obj["api2_host"].(string), nil

}
