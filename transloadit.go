package transloadit

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type Config struct {
	AuthKey    string
	AuthSecret string
	Region     string
	Endpoint   string
}

var DefaultConfig = Config{
	Region:   "us-east-1",
	Endpoint: "http://api2.transloadit.com",
}

type Client struct {
	config     *Config
	httpClient *http.Client
}

type Response map[string]interface{}

func NewClient(config *Config) (*Client, error) {

	if config.AuthKey == "" {
		return nil, errors.New("failed to create client: missing AuthKey")
	}

	if config.AuthSecret == "" {
		return nil, errors.New("failed to create client: missing AuthSecret")
	}

	client := &Client{
		config:     config,
		httpClient: &http.Client{},
	}

	return client, nil

}

func (client *Client) sign(params map[string]interface{}) (string, string, error) {

	// Expires in 1 hour
	expires := time.Now().UTC().Add(time.Hour)
	expiresStr := fmt.Sprintf("%04d/%02d/%02d %02d:%02d:%02d+00:00", expires.Year(), expires.Month(), expires.Day(), expires.Hour(), expires.Minute(), expires.Second())

	params["auth"] = map[string]string{
		"key":     client.config.AuthKey,
		"expires": expiresStr,
	}

	b, err := json.Marshal(params)
	if err != nil {
		return "", "", fmt.Errorf("unable to create signature: %s", err)
	}

	hash := hmac.New(sha1.New, []byte(client.config.AuthSecret))
	hash.Write(b)
	return string(b), hex.EncodeToString(hash.Sum(nil)), nil

}

func (client *Client) doRequest(req *http.Request) (Response, error) {

	res, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed execute http request: %s", err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed execute http request: %s", err)
	}

	var obj Response
	err = json.Unmarshal(body, &obj)
	if err != nil {
		return nil, fmt.Errorf("failed execute http request: %s", err)
	}

	if res.StatusCode != 200 {
		return obj, fmt.Errorf("failed execute http request: server responded with %s", obj["error"])
	}

	return obj, nil
}
