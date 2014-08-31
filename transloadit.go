package transloadit

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
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

type ListOptions struct {
	Page       int        `json:"page,omitempty"`
	PageSize   int        `json:"pagesize,omitempty"`
	Sort       string     `json:"sort,omitempty"`
	Order      string     `json:"order,omitempty"`
	Fields     []string   `json:"fields,omitempty"`
	Type       string     `json:"type,omitempty"`
	Keywords   []string   `json:"keyword,omitempty"`
	AssemblyId string     `json:"assembly_id,omitempty"`
	FromDate   *time.Time `json:"fromdate,omitempty"`
	ToDate     *time.Time `json:"todate,omitempty"`
	Auth       struct {
		Key     string `json:"key"`
		Expires string `json:"expires"`
	} `json:"auth"`
}

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

	params["auth"] = map[string]string{
		"key":     client.config.AuthKey,
		"expires": getExpireString(),
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

func (client *Client) request(method string, path string, content map[string]interface{}) (Response, error) {

	uri := path
	// Don't add host for absolute urls
	if u, err := url.Parse(path); err == nil && u.Scheme == "" {
		uri = client.config.Endpoint + "/" + path
	}

	// Ensure content is a map
	if content == nil {
		content = make(map[string]interface{})
	}

	// Create signature
	params, signature, err := client.sign(content)
	if err != nil {
		return nil, fmt.Errorf("request: %s", err)
	}

	v := url.Values{}
	v.Set("params", params)
	v.Set("signature", signature)

	var body io.Reader
	if method == "GET" {
		uri += "?" + v.Encode()
	} else {
		body = strings.NewReader(v.Encode())
	}
	req, err := http.NewRequest(method, uri, body)
	if err != nil {
		return nil, fmt.Errorf("request: %s", err)
	}

	if method != "GET" {
		// Add content type header
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	return client.doRequest(req)
}

func (client *Client) listRequest(path string, options *ListOptions) ([]byte, error) {

	uri := client.config.Endpoint + "/" + path

	options.Auth = struct {
		Key     string `json:"key"`
		Expires string `json:"expires"`
	}{
		Key:     client.config.AuthKey,
		Expires: getExpireString(),
	}

	b, err := json.Marshal(options)
	if err != nil {
		return nil, fmt.Errorf("unable to create signature: %s", err)
	}

	hash := hmac.New(sha1.New, []byte(client.config.AuthSecret))
	hash.Write(b)

	params := string(b)
	signature := hex.EncodeToString(hash.Sum(nil))

	v := url.Values{}
	v.Set("params", params)
	v.Set("signature", signature)

	uri += "?" + v.Encode()

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, fmt.Errorf("request: %s", err)
	}

	res, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed execute http request: %s", err)
	}

	return ioutil.ReadAll(res.Body)

}

func getExpireString() string {

	// Expires in 1 hour
	expires := time.Now().UTC().Add(time.Hour)
	expiresStr, _ := expires.MarshalText()
	return string(expiresStr)

}
