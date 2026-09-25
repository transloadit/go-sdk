// Package contract provides generated, typed ordinary API methods alongside the existing SDK.
package contract

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
)

// Config selects signed requests or bearer authentication, never an implicit token exchange.
type Config struct {
	Origin             string
	AuthKey            string
	AuthSecret         string
	BearerToken        string
	SignatureAlgorithm string
	HTTPClient         *http.Client
}

// Client binds generated methods to one explicitly configured API origin.
type Client struct {
	config     Config
	httpClient *http.Client
}

// UploadFile streams a caller-owned reader as one multipart field.
type UploadFile struct {
	Reader   io.Reader
	Filename string
}

// ResponseError retains decoded error data without including it in diagnostic messages.
type ResponseError struct {
	Status int
	Data   json.RawMessage
}

func (err *ResponseError) Error() string {
	return fmt.Sprintf("API request failed with HTTP %d", err.Status)
}

// TransportError preserves the cause without printing a signed query URL in ordinary logs.
type TransportError struct{ Cause error }

func (err *TransportError) Error() string { return "API transport failed" }
func (err *TransportError) Unwrap() error { return err.Cause }

// NewClient creates a client without changing the caller's HTTP client or following redirects.
func NewClient(config Config) (*Client, error) {
	if config.Origin == "" {
		config.Origin = defaultOrigin
	}
	origin, err := url.Parse(config.Origin)
	if err != nil || (origin.Scheme != "https" && origin.Scheme != "http") || origin.Host == "" || origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, fmt.Errorf("contract client requires an HTTP(S) origin without credentials, path or query")
	}
	if config.BearerToken == "" && (config.AuthKey == "" || config.AuthSecret == "") {
		return nil, fmt.Errorf("Auth Key credentials or bearer token required")
	}
	if config.BearerToken != "" && (config.AuthKey != "" || config.AuthSecret != "") {
		return nil, fmt.Errorf("choose signed or bearer authentication")
	}
	config.Origin = origin.Scheme + "://" + origin.Host
	if config.SignatureAlgorithm == "" {
		config.SignatureAlgorithm = defaultAlgorithm
	}
	transport := http.Client{Timeout: time.Minute}
	if config.HTTPClient != nil {
		transport = *config.HTTPClient
	}
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return fmt.Errorf("API redirects are not followed") }
	return &Client{config: config, httpClient: &transport}, nil
}

type operation struct {
	ID             string
	Method         string
	Path           string
	Auth           string
	Bearer         bool
	Encoding       string
	ParamsField    string
	SignatureField string
}

type impossibleValue struct{}

func (impossibleValue) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("value is forbidden by the contract")
}
func (*impossibleValue) UnmarshalJSON([]byte) error {
	return fmt.Errorf("value is forbidden by the contract")
}

func rejectNull(data []byte) error {
	if strings.TrimSpace(string(data)) == "null" {
		return fmt.Errorf("unexpected JSON null")
	}
	return nil
}

// Reflection operates only on generated structs and their JSON tags, not an API field inventory.
func marshalObject(value interface{}) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	object := reflect.ValueOf(value)
	extra := object.FieldByName("AdditionalProperties")
	if !extra.IsValid() || extra.Len() == 0 {
		return data, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	known := make(map[string]bool)
	for index := 0; index < object.NumField(); index++ {
		key := strings.Split(object.Type().Field(index).Tag.Get("json"), ",")[0]
		if key != "-" {
			known[key] = true
		}
	}
	iterator := extra.MapRange()
	for iterator.Next() {
		key := iterator.Key().String()
		if known[key] {
			return nil, fmt.Errorf("reserved additional property")
		}
		encoded, err := json.Marshal(iterator.Value().Interface())
		if err != nil {
			return nil, err
		}
		fields[key] = encoded
	}
	return json.Marshal(fields)
}

func unmarshalObject(data []byte, destination interface{}, required []string, nullable []string) error {
	if err := rejectNull(data); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range required {
		if _, present := fields[key]; !present {
			return fmt.Errorf("missing required property: %s", key)
		}
	}
	nullableKeys := make(map[string]bool)
	for _, key := range nullable {
		nullableKeys[key] = true
	}
	object := reflect.ValueOf(destination).Elem()
	for index := 0; index < object.NumField(); index++ {
		field := object.Field(index)
		key := strings.Split(object.Type().Field(index).Tag.Get("json"), ",")[0]
		if key == "-" {
			continue
		}
		raw, present := fields[key]
		if !present {
			continue
		}
		if strings.TrimSpace(string(raw)) == "null" && !nullableKeys[key] {
			return fmt.Errorf("unexpected null property: %s", key)
		}
		// Allocate an optional nullable wrapper before decoding null. encoding/json otherwise
		// resets the pointer to nil, losing the distinction between absence and explicit null.
		var target interface{}
		if field.Kind() == reflect.Ptr {
			field.Set(reflect.New(field.Type().Elem()))
			target = field.Interface()
		} else {
			target = field.Addr().Interface()
		}
		if err := json.Unmarshal(raw, target); err != nil {
			return err
		}
		delete(fields, key)
	}
	extra := object.FieldByName("AdditionalProperties")
	if !extra.IsValid() {
		if len(fields) != 0 {
			return fmt.Errorf("unknown object property")
		}
		return nil
	}
	if len(fields) != 0 {
		extra.Set(reflect.MakeMap(extra.Type()))
		for key, raw := range fields {
			value := reflect.New(extra.Type().Elem())
			if err := json.Unmarshal(raw, value.Interface()); err != nil {
				return err
			}
			extra.SetMapIndex(reflect.ValueOf(key), value.Elem())
		}
	}
	return nil
}

func (client *Client) signature(data []byte) (string, error) {
	algorithm := client.config.SignatureAlgorithm
	supported := false
	for _, candidate := range signatureAlgorithms {
		if candidate == algorithm {
			supported = true
		}
	}
	if !supported {
		return "", fmt.Errorf("unsupported request signature algorithm")
	}
	var constructor func() hash.Hash
	switch algorithm {
	case "sha384":
		constructor = sha512.New384
	case "sha256":
		constructor = sha256.New
	case "sha1":
		constructor = sha1.New
	default:
		return "", fmt.Errorf("unsupported native signature algorithm")
	}
	mac := hmac.New(constructor, []byte(client.config.AuthSecret))
	mac.Write(data)
	return algorithm + signatureSeparator + hex.EncodeToString(mac.Sum(nil)), nil
}

func (client *Client) request(ctx context.Context, operation operation, path map[string]string, input interface{}, files map[string]UploadFile, extraFields map[string]string, result interface{}) error {
	target := operation.Path
	for name, value := range path {
		if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\") || strings.ContainsFunc(value, func(character rune) bool { return character < 32 || character == 127 }) {
			return fmt.Errorf("invalid path parameter: %s", name)
		}
		target = strings.ReplaceAll(target, "{"+name+"}", url.PathEscape(value))
	}
	if strings.ContainsAny(target, "{}") || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		return fmt.Errorf("invalid generated request path")
	}
	fields := url.Values{}
	if operation.Encoding != "none" {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		var params map[string]json.RawMessage
		if err := json.Unmarshal(data, &params); err != nil {
			return err
		}
		if params == nil {
			return fmt.Errorf("request parameters must be an object")
		}
		if operation.Encoding == "form" {
			for key, raw := range params {
				var value string
				if err := rejectNull(raw); err != nil {
					return err
				}
				if err := json.Unmarshal(raw, &value); err != nil {
					return fmt.Errorf("invalid form field: %s", key)
				}
				fields.Set(key, value)
			}
		} else {
			if _, ok := params["auth"]; ok {
				return fmt.Errorf("the SDK owns params.auth; configure client authentication")
			}
			if operation.Auth == "api-key" && client.config.BearerToken == "" {
				auth, err := json.Marshal(map[string]string{"key": client.config.AuthKey, "expires": time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)})
				if err != nil {
					return err
				}
				params["auth"] = auth
				if _, ok := params["nonce"]; !ok {
					nonce := make([]byte, 16)
					if _, err := rand.Read(nonce); err != nil {
						return err
					}
					encoded, err := json.Marshal(hex.EncodeToString(nonce))
					if err != nil {
						return err
					}
					params["nonce"] = encoded
				}
			}
			data, err = json.Marshal(params)
			if err != nil {
				return err
			}
			fields.Set(operation.ParamsField, string(data))
			if operation.Auth == "api-key" && client.config.BearerToken == "" {
				signature, err := client.signature(data)
				if err != nil {
					return err
				}
				fields.Set(operation.SignatureField, signature)
			}
		}
	}
	uri := client.config.Origin + target
	var body io.Reader
	contentType := ""
	if operation.Encoding == "query" {
		uri += "?" + fields.Encode()
	} else if operation.Encoding == "multipart/form-data" {
		for key := range extraFields {
			if _, ok := fields[key]; ok {
				return fmt.Errorf("reserved form field: %s", key)
			}
		}
		for key, file := range files {
			if _, ok := fields[key]; ok {
				return fmt.Errorf("reserved file field: %s", key)
			}
			if _, ok := extraFields[key]; ok {
				return fmt.Errorf("duplicate form field: %s", key)
			}
			if file.Reader == nil {
				return fmt.Errorf("missing file reader")
			}
		}
		reader, writer := io.Pipe()
		defer reader.Close()
		multipartWriter := multipart.NewWriter(writer)
		contentType = multipartWriter.FormDataContentType()
		body = reader
		go func() {
			err := writeMultipart(multipartWriter, fields, extraFields, files)
			if err == nil {
				err = multipartWriter.Close()
			}
			writer.CloseWithError(err)
		}()
	} else if operation.Encoding != "none" {
		body = strings.NewReader(fields.Encode())
		contentType = "application/x-www-form-urlencoded"
	}
	req, err := http.NewRequestWithContext(ctx, operation.Method, uri, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if operation.Auth == "basic" {
		if client.config.BearerToken != "" {
			return fmt.Errorf("this operation requires Auth Key credentials")
		}
		req.SetBasicAuth(client.config.AuthKey, client.config.AuthSecret)
	} else if operation.Auth == "api-key" && client.config.BearerToken != "" {
		if !operation.Bearer {
			return fmt.Errorf("this operation requires signed authentication")
		}
		req.Header.Set("Authorization", "Bearer "+client.config.BearerToken)
	}
	response, err := client.httpClient.Do(req)
	if err != nil {
		return &TransportError{Cause: err}
	}
	defer response.Body.Close()
	// Bounded independently of Content-Length, which can be absent or untrusted.
	const limit = 128 * 1024 * 1024
	data, err := ioutil.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return err
	}
	if len(data) > limit {
		return fmt.Errorf("API response exceeds size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if !json.Valid(data) {
			data = nil
		}
		return &ResponseError{Status: response.StatusCode, Data: data}
	}
	if !json.Valid(data) {
		return fmt.Errorf("API returned an invalid JSON response")
	}
	return json.Unmarshal(bytes.TrimSpace(data), result)
}

func writeMultipart(writer *multipart.Writer, fields url.Values, extras map[string]string, files map[string]UploadFile) error {
	for key, values := range fields {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				return err
			}
		}
	}
	for key, value := range extras {
		if err := writer.WriteField(key, value); err != nil {
			return err
		}
	}
	for key, file := range files {
		part, err := writer.CreateFormFile(key, file.Filename)
		if err != nil {
			return err
		}
		if _, err := io.Copy(part, file.Reader); err != nil {
			return err
		}
	}
	return nil
}
