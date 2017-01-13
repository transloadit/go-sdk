package transloadit

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	config     Config
	httpClient *http.Client
}

// Options when retrieving a list.
// Look at the documentation which properties are accepted
// and to see their meaining, e.g. https://transloadit.com/docs/api-docs#retrieve-assembly-list
// for listing assemblies.
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
}

type authParams struct {
	Key     string `json:"key"`
	Expires string `json:"expires"`
}

type authListOptions struct {
	*ListOptions

	// For internal use only!
	Auth authParams `json:"auth"`
}

type RequestError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
}

func (err RequestError) Error() string {
	return fmt.Sprintf("request failed due to %s: %s", err.Code, err.Message)
}

// Create a new client using the provided configuration object.
// An error will be returned if no AuthKey or AuthSecret is found in config.
func NewClient(config Config) (*Client, error) {
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
	params["auth"] = authParams{
		Key:     client.config.AuthKey,
		Expires: getExpireString(),
	}

	b, err := json.Marshal(params)
	if err != nil {
		return "", "", fmt.Errorf("unable to create signature: %s", err)
	}

	hash := hmac.New(sha1.New, []byte(client.config.AuthSecret))
	hash.Write(b)
	return string(b), hex.EncodeToString(hash.Sum(nil)), nil
}

func (client *Client) doRequest(req *http.Request, result interface{}) error {
	req.Header.Set("User-Agent", "Transloadit Go SDK "+Version)

	res, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed execute http request: %s", err)
	}
	defer res.Body.Close()

	// Limit response to 128MB
	reader := io.LimitReader(res.Body, 128*1024*1024)
	decoder := json.NewDecoder(reader)

	if !(res.StatusCode >= 200 && res.StatusCode < 300) {
		var reqErr RequestError
		if err := decoder.Decode(&reqErr); err != nil {
			return fmt.Errorf("failed unmarshal http request: %s", err)
		}

		return reqErr
	}

	if result != nil {
		if err := decoder.Decode(result); err != nil {
			return fmt.Errorf("failed unmarshal http request: %s", err)
		}
	}

	return nil
}

func (client *Client) request(method string, path string, content map[string]interface{}, result interface{}) error {
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
		return fmt.Errorf("request: %s", err)
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
		return fmt.Errorf("request: %s", err)
	}

	if method != "GET" {
		// Add content type header
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	return client.doRequest(req, result)
}

func (client *Client) listRequest(path string, listOptions *ListOptions, result interface{}) error {
	uri := client.config.Endpoint + "/" + path

	options := authListOptions{
		ListOptions: listOptions,
		Auth: authParams{
			Key:     client.config.AuthKey,
			Expires: getExpireString(),
		},
	}

	b, err := json.Marshal(options)
	if err != nil {
		return fmt.Errorf("unable to create signature: %s", err)
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
		return fmt.Errorf("request: %s", err)
	}

	return client.doRequest(req, result)
}

func getExpireString() string {
	// Expires in 1 hour
	expires := time.Now().UTC().Add(time.Hour)
	expiresStr := fmt.Sprintf("%04d/%02d/%02d %02d:%02d:%02d+00:00", expires.Year(), expires.Month(), expires.Day(), expires.Hour(), expires.Minute(), expires.Second())
	return string(expiresStr)
}
