// Package transloadit provides a client to interact with the Transloadt API.
package transloadit

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
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
	random     *rand.Rand
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
		random:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	return client
}

func (client *Client) sign(params map[string]interface{}) (string, string, error) {
	params["auth"] = authParams{
		Key:     client.config.AuthKey,
		Expires: getExpireString(),
	}
	// Add a random nonce to make signatures unique and prevent error about
	// signature reuse: https://github.com/transloadit/go-sdk/pull/35
	params["nonce"] = client.random.Int()
	contentToSign, err := json.Marshal(params)
	if err != nil {
		return "", "", fmt.Errorf("unable to create signature: %s", err)
	}

	hash := hmac.New(sha512.New384, []byte(client.config.AuthSecret))
	hash.Write(contentToSign)
	signature := "sha384:" + hex.EncodeToString(hash.Sum(nil))

	return string(contentToSign), signature, nil
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
	body, err := ioutil.ReadAll(reader)
	if err != nil {
		return err
	}

	if !(res.StatusCode >= 200 && res.StatusCode < 300) {
		var reqErr RequestError
		if err := json.Unmarshal(body, &reqErr); err != nil {
			return fmt.Errorf("failed unmarshal http request: %s", err)
		}

		return reqErr
	}

	if result != nil {
		if err := json.Unmarshal(body, result); err != nil {
			fmt.Println(string(body))
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

// SignedSmartCDNUrlOptions contains options for creating a signed Smart CDN URL
type SignedSmartCDNUrlOptions struct {
	// Workspace slug
	Workspace string
	// Template slug or template ID
	Template string
	// Input value that is provided as `${fields.input}` in the template
	Input string
	// Additional parameters for the URL query string. Can be nil.
	URLParams map[string]string
	// Expiration time of the signature in milliseconds. Defaults to 1 hour.
	ExpiresIn int64
}

// Allows us to overwrite `time.Now()` for testing purposes.
var now = time.Now

// CreateSignedSmartCDNUrl constructs a signed Smart CDN URL.
// See https://transloadit.com/docs/topics/signature-authentication/#smart-cdn
func (client *Client) CreateSignedSmartCDNUrl(opts SignedSmartCDNUrlOptions) string {
	workspaceSlug := url.PathEscape(opts.Workspace)
	templateSlug := url.PathEscape(opts.Template)
	inputField := url.PathEscape(opts.Input)

	expiresIn := opts.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int64(time.Hour.Milliseconds()) // 1 hour
	}

	// Convert URLParams to url.Values
	queryParams := make(map[string]string, len(opts.URLParams)+2)
	for key, value := range opts.URLParams {
		queryParams[key] = value
	}

	queryParams["auth_key"] = client.config.AuthKey
	queryParams["exp"] = fmt.Sprintf("%d", now().UnixMilli()+expiresIn)

	// Build query string with sorted keys
	queryParamsKeys := make([]string, 0, len(queryParams))
	for k := range queryParams {
		queryParamsKeys = append(queryParamsKeys, k)
	}
	sort.Strings(queryParamsKeys)

	var queryParts []string
	for _, k := range queryParamsKeys {
		queryParts = append(queryParts, url.QueryEscape(k)+"="+url.QueryEscape(queryParams[k]))
	}
	queryString := strings.Join(queryParts, "&")

	stringToSign := fmt.Sprintf("%s/%s/%s?%s", workspaceSlug, templateSlug, inputField, queryString)

	// Create signature using SHA-256
	hash := hmac.New(sha256.New, []byte(client.config.AuthSecret))
	hash.Write([]byte(stringToSign))
	signature := hex.EncodeToString(hash.Sum(nil))

	signedURL := fmt.Sprintf("https://%s.tlcdn.com/%s/%s?%s&sig=sha256:%s",
		workspaceSlug, templateSlug, inputField, queryString, signature)

	return signedURL
}
