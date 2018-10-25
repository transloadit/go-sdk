// Package transloadit provides a client to interact with the Transloadt API.
package transloadit

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config defines the configuration options for a client.
type Config struct {
	AuthKey    string
	AuthSecret string
	Endpoint   string
}

// DefaultConfig is the recommended base configuration.
var DefaultConfig = Config{
	Endpoint: "https://api2.transloadit.com",
}

// Client provides an interface to the Transloadit REST API bound to a specific
// account.
type Client struct {
	config     Config
	httpClient *http.Client
}

// ListOptions defines criteria used when a list is being retrieved. Details
// about each property can be found at https://transloadit.com/docs/api-docs#retrieve-assembly-list.
type ListOptions struct {
	Page       int        `json:"page,omitempty"`
	PageSize   int        `json:"pagesize,omitempty"`
	Sort       string     `json:"sort,omitempty"`
	Order      string     `json:"order,omitempty"`
	Fields     []string   `json:"fields,omitempty"`
	Type       string     `json:"type,omitempty"`
	Keywords   []string   `json:"keyword,omitempty"`
	AssemblyID string     `json:"assembly_id,omitempty"`
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

// RequestError represents an error returned by the Transloadit API alongside
// additional service-specific information.
type RequestError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
}

// Error return a formatted message describing the error.
func (err RequestError) Error() string {
	return fmt.Sprintf("request failed due to %s: %s", err.Code, err.Message)
}

// NewClient creates a new client using the provided configuration struct.
// It will panic if no Config.AuthKey or Config.AuthSecret are empty.
func NewClient(config Config) Client {
	if config.AuthKey == "" {
		panic("failed to create Transloadit client: missing AuthKey")
	}

	if config.AuthSecret == "" {
		panic("failed to create Transloadit client: missing AuthSecret")
	}

	client := Client{
		config:     config,
		httpClient: &http.Client{},
	}

	return client
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
	req.Header.Set("Transloadit-Client", "go-sdk:"+Version)

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

func (client *Client) request(ctx context.Context, method string, path string, content map[string]interface{}, result interface{}) error {
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
	req = req.WithContext(ctx)

	if method != "GET" {
		// Add content type header
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	return client.doRequest(req, result)
}

func (client *Client) listRequest(ctx context.Context, path string, listOptions *ListOptions, result interface{}) error {
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
	req = req.WithContext(ctx)

	return client.doRequest(req, result)
}

func getExpireString() string {
	// Expires in 1 hour
	expires := time.Now().UTC().Add(time.Hour)
	expiresStr := fmt.Sprintf("%04d/%02d/%02d %02d:%02d:%02d+00:00", expires.Year(), expires.Month(), expires.Day(), expires.Hour(), expires.Minute(), expires.Second())
	return string(expiresStr)
}
