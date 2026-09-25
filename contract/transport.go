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
	"math/big"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

// UploadFile transfers ownership of a stream to one API call, which closes it before returning.
// Close must unblock a concurrent Read, as for an HTTP request body. For in-memory readers,
// ioutil.NopCloser is sufficient; blocking sources must implement cancellation in Close.
type UploadFile struct {
	Reader   io.ReadCloser
	Filename string
}

type multipartBody struct {
	reader     *io.PipeReader
	closeFiles func()
	done       <-chan struct{}
	once       sync.Once
}

func (body *multipartBody) Read(data []byte) (int, error) { return body.reader.Read(data) }
func (body *multipartBody) Close() error {
	body.once.Do(func() {
		body.reader.Close()
		body.closeFiles()
		// Returning ownership before the producer exits races callers that reuse upload state.
		<-body.done
	})
	return nil
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
	if err != nil || (origin.Scheme != "https" && origin.Scheme != "http") || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, fmt.Errorf("contract client requires an HTTP(S) endpoint without credentials, query or fragment")
	}
	address := net.ParseIP(origin.Hostname())
	localhost := strings.TrimSuffix(strings.ToLower(origin.Hostname()), ".") == "localhost"
	if origin.Scheme == "http" && !localhost && (address == nil || !address.IsLoopback()) {
		return nil, fmt.Errorf("HTTPS is required except for loopback development endpoints")
	}
	if config.BearerToken == "" && (config.AuthKey == "" || config.AuthSecret == "") {
		return nil, fmt.Errorf("Auth Key credentials or bearer token required")
	}
	if config.BearerToken != "" && (config.AuthKey != "" || config.AuthSecret != "") {
		return nil, fmt.Errorf("choose signed or bearer authentication")
	}
	config.Origin = strings.TrimSuffix(origin.String(), "/")
	if config.SignatureAlgorithm == "" {
		config.SignatureAlgorithm = defaultAlgorithm
	}
	// A total default timeout also limits the time spent streaming uploads. Callers own deadlines.
	transport := http.Client{}
	if config.HTTPClient != nil {
		transport = *config.HTTPClient
	}
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return fmt.Errorf("API redirects are not followed") }
	return &Client{config: config, httpClient: &transport}, nil
}

type operation struct {
	ID              string
	Method          string
	Path            string
	RawPathPatterns map[string]string
	Auth            string
	Bearer          bool
	Encoding        string
	ParamsField     string
	SignatureField  string
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

func unmarshalInteger(data []byte, destination *int64) error {
	text := strings.TrimSpace(string(data))
	if !json.Valid(data) || len(text) == 0 || (text[0] != '-' && (text[0] < '0' || text[0] > '9')) {
		return fmt.Errorf("expected a JSON integer")
	}
	if !strings.ContainsAny(text, ".eE") {
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return fmt.Errorf("expected a signed 64-bit JSON integer")
		}
		*destination = value
		return nil
	}
	if index := strings.IndexAny(text, "eE"); index >= 0 {
		mantissa := text[:index]
		if strings.Trim(mantissa, "-0.") == "" {
			*destination = 0
			return nil
		}
		exponent, err := strconv.ParseInt(text[index+1:], 10, 64)
		// Go 1.15's abs(MinInt64) exponent guard overflows. A nonzero signed-64-bit integer cannot
		// need a decimal shift beyond its encoded mantissa length plus 19 digits; reject it first.
		limit := int64(len(mantissa)) + 19
		if err != nil || exponent < -limit || exponent > limit {
			return fmt.Errorf("expected a signed 64-bit JSON integer")
		}
	}
	// Exact rational parsing avoids rounding fractions or large values through float64.
	value, valid := new(big.Rat).SetString(text)
	if !valid || !value.IsInt() || !value.Num().IsInt64() {
		return fmt.Errorf("expected a signed 64-bit JSON integer")
	}
	*destination = value.Num().Int64()
	return nil
}

// Tolerant response decoding must not let an earlier alternative swallow another one's fields.
// Compare retained field paths, not values: integer normalization must not lose numeric precision.
func unmarshalUnion(data []byte, candidates []func() interface{}, nullable []bool) (int, error) {
	if len(candidates) != len(nullable) {
		return -1, fmt.Errorf("invalid union metadata")
	}
	isNull := strings.TrimSpace(string(data)) == "null"
	combined := make(map[string]bool)
	best, bestCount := -1, -1
	for index, create := range candidates {
		if isNull && !nullable[index] {
			continue
		}
		candidate := create()
		if err := json.Unmarshal(data, candidate); err != nil {
			continue
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.UseNumber()
		var retained interface{}
		if err := decoder.Decode(&retained); err != nil {
			continue
		}
		paths := make(map[string]bool)
		unionFieldPaths(retained, "", paths)
		for path := range paths {
			combined[path] = true
		}
		if len(paths) > bestCount {
			best, bestCount = index, len(paths)
		}
	}
	if best == -1 {
		return -1, fmt.Errorf("invalid union JSON shape")
	}
	// Every candidate's paths are a subset of the union, so equal sizes prove complete coverage.
	// Unknown additive fields absent from every model remain tolerated, as in ordinary responses.
	if bestCount != len(combined) {
		return -1, fmt.Errorf("union alternatives cannot retain all modeled fields")
	}
	return best, nil
}

func unionFieldPaths(value interface{}, prefix string, paths map[string]bool) {
	switch value := value.(type) {
	case map[string]interface{}:
		for key, child := range value {
			path := prefix + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			paths[path] = true
			unionFieldPaths(child, path, paths)
		}
	case []interface{}:
		for index, child := range value {
			path := prefix + "/" + strconv.Itoa(index)
			paths[path] = true
			unionFieldPaths(child, path, paths)
		}
	}
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
		// Client decoding must tolerate additive server fields, including OAuth token extensions.
		// This is not a full JSON Schema validator; known required fields and values still decode.
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
	var closeOnce sync.Once
	closeFiles := func() {
		closeOnce.Do(func() {
			for _, file := range files {
				if file.Reader != nil {
					file.Reader.Close()
				}
			}
		})
	}
	defer closeFiles()
	target := operation.Path
	for name, value := range path {
		// The generator only supplies the owner's portable ASCII path grammar here, not arbitrary
		// JSON Schema regexes. Native transport contains no endpoint-specific path inventory.
		if pattern, ok := operation.RawPathPatterns[name]; ok && strings.IndexFunc(value, func(character rune) bool { return character < 32 || character == 127 }) < 0 {
			matched, err := regexp.MatchString(pattern, value)
			if err != nil {
				return fmt.Errorf("invalid generated path grammar")
			}
			if matched {
				target = strings.ReplaceAll(target, "{"+name+"}", value)
				continue
			}
		}
		if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\") || strings.IndexFunc(value, func(character rune) bool { return character < 32 || character == 127 }) >= 0 {
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
			authFields := make(map[string]json.RawMessage)
			if raw, ok := params["auth"]; ok {
				if err := json.Unmarshal(raw, &authFields); err != nil || authFields == nil {
					return fmt.Errorf("expected auth metadata object")
				}
				if _, present := authFields["key"]; present {
					return fmt.Errorf("the SDK owns auth credentials")
				}
				if _, present := authFields["expires"]; present {
					return fmt.Errorf("the SDK owns auth credentials")
				}
			}
			if operation.Auth == "api-key" && client.config.BearerToken == "" {
				key, err := json.Marshal(client.config.AuthKey)
				if err != nil {
					return err
				}
				expires, err := json.Marshal(time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339))
				if err != nil {
					return err
				}
				authFields["key"], authFields["expires"] = key, expires
				auth, err := json.Marshal(authFields)
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
		done := make(chan struct{})
		upload := &multipartBody{reader: reader, closeFiles: closeFiles, done: done}
		defer upload.Close()
		multipartWriter := multipart.NewWriter(writer)
		contentType = multipartWriter.FormDataContentType()
		body = upload
		go func() {
			defer close(done)
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
		return &TransportError{Cause: err}
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
