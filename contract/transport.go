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
	Origin string
	// AssemblyOrigins contains deployment-owned origins, never values copied from API responses.
	AssemblyOrigins []string
	AuthKey         string
	AuthSecret      string
	BearerToken     string
	// NoAccountCredentials explicitly selects only operations that need no account authentication.
	NoAccountCredentials bool
	SignatureAlgorithm   string
	// HTTPClient.Transport also carries uploader/tus requests. The SDK does not add API
	// credentials to these capability requests and excludes the configured cookie jar.
	HTTPClient *http.Client
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
	// RetryAfter is the server-requested delay, or zero when absent or invalid.
	RetryAfter time.Duration
	// Cause retains an interrupted error-body read without exposing it in Error().
	Cause error
}

func retryAfterDuration(header string, now time.Time) time.Duration {
	value := strings.TrimSpace(header)
	if value != "" && strings.Trim(value, "0123456789") == "" {
		seconds, err := strconv.ParseUint(value, 10, 64)
		const maximum = time.Duration(1<<63 - 1)
		if err != nil || seconds > uint64(maximum/time.Second) {
			return maximum
		}
		return time.Duration(seconds) * time.Second
	}
	date, err := http.ParseTime(value)
	if err != nil || !date.After(now) {
		return 0
	}
	return date.Sub(now)
}

func (err *ResponseError) Error() string {
	return fmt.Sprintf("API request failed with HTTP %d", err.Status)
}

func (err *ResponseError) Unwrap() error { return err.Cause }

// Code returns a recognized public contract error code, or empty for an unknown body/code.
// It does not expose arbitrary response text through routine diagnostics.
func (err *ResponseError) Code() string {
	if err == nil {
		return ""
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(err.Data, &body) != nil {
		return ""
	}
	var code string
	if json.Unmarshal(body["error"], &code) != nil || !isResponseErrorCode(code) {
		return ""
	}
	return code
}

// TransportError preserves the cause with the standard HTTP request URL redacted.
type TransportError struct{ Cause error }

func (err *TransportError) Error() string { return "API transport failed" }
func (err *TransportError) Unwrap() error { return err.Cause }

func redactRequestURL(err error) error {
	requestError, ok := err.(*url.Error)
	if !ok {
		return err
	}
	// Do not mutate the caller's error. The request URL can contain signed parameters;
	// the nested cause retains cancellation, timeout and connection error identities.
	return &url.Error{Op: requestError.Op, URL: "[redacted]", Err: redactRequestURL(requestError.Err)}
}

func normalizeEndpointAuthority(origin *url.URL) error {
	hostname := strings.ToLower(origin.Hostname())
	port := origin.Port()
	if port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value > 65535 {
			return fmt.Errorf("invalid endpoint port")
		}
		if (origin.Scheme == "https" && value == 443) || (origin.Scheme == "http" && value == 80) {
			port = ""
		} else {
			port = strconv.Itoa(value)
		}
	}
	origin.Host = hostname
	if strings.Contains(hostname, ":") {
		origin.Host = "[" + hostname + "]"
	}
	if port != "" {
		origin.Host = net.JoinHostPort(hostname, port)
	}
	return nil
}

func parseConfiguredEndpoint(value string) (*url.URL, error) {
	origin, err := url.Parse(value)
	if err != nil || (origin.Scheme != "https" && origin.Scheme != "http") || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" {
		return nil, fmt.Errorf("contract client requires an HTTP(S) endpoint without credentials, query or fragment")
	}
	address := net.ParseIP(origin.Hostname())
	localhost := strings.TrimSuffix(strings.ToLower(origin.Hostname()), ".") == "localhost"
	if origin.Scheme == "http" && !localhost && (address == nil || !address.IsLoopback()) {
		return nil, fmt.Errorf("HTTPS is required except for loopback development endpoints")
	}
	// Go's URL parser preserves host case and default ports. Compare configured and returned
	// authorities consistently, without normalizing away significant proxy path/encoding bytes.
	if err := normalizeEndpointAuthority(origin); err != nil {
		return nil, err
	}
	return origin, nil
}

// NewClient creates a client without changing the caller's HTTP client or following redirects.
func NewClient(config Config) (*Client, error) {
	config.AssemblyOrigins = append([]string(nil), config.AssemblyOrigins...)
	for index, value := range config.AssemblyOrigins {
		origin, err := parseConfiguredEndpoint(value)
		if err != nil {
			return nil, err
		}
		if origin.Path != "" && origin.Path != "/" {
			return nil, fmt.Errorf("Assembly uploader origins cannot include a path")
		}
		config.AssemblyOrigins[index] = assemblyOrigin(origin)
	}
	if config.Origin == "" {
		config.Origin = defaultOrigin
	}
	origin, err := parseConfiguredEndpoint(config.Origin)
	if err != nil {
		return nil, err
	}
	if config.NoAccountCredentials && (config.AuthKey != "" || config.AuthSecret != "" || config.BearerToken != "" || config.SignatureAlgorithm != "") {
		return nil, fmt.Errorf("choose signed, bearer or no account authentication")
	}
	if !config.NoAccountCredentials && config.BearerToken == "" && (config.AuthKey == "" || config.AuthSecret == "") {
		return nil, fmt.Errorf("Auth Key credentials or bearer token required")
	}
	if config.BearerToken != "" && (config.AuthKey != "" || config.AuthSecret != "") {
		return nil, fmt.Errorf("choose signed or bearer authentication")
	}
	// Normalize only the join boundary; internal proxy path segments remain significant.
	config.Origin = strings.TrimRight(origin.String(), "/")
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
	AuthFormField   string
	AuthFormValues  map[string]string
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
	negative := strings.HasPrefix(text, "-")
	mantissa := strings.TrimPrefix(text, "-")
	exponentText := "0"
	if index := strings.IndexAny(mantissa, "eE"); index >= 0 {
		exponentText, mantissa = mantissa[index+1:], mantissa[:index]
	}
	fractionDigits := 0
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		fractionDigits = len(mantissa) - index - 1
	}
	digits := strings.TrimLeft(strings.ReplaceAll(mantissa, ".", ""), "0")
	if digits == "" {
		*destination = 0
		return nil
	}
	significant := strings.TrimRight(digits, "0")
	// Cancel decimal zeros before conversion. Arbitrary-precision parsing would allocate huge
	// integers even for a tiny result such as 1 followed by many zeros with a cancelling exponent.
	if len(significant) > 19 {
		return fmt.Errorf("expected a signed 64-bit JSON integer")
	}
	exponent, err := strconv.ParseInt(exponentText, 10, 64)
	required := int64(fractionDigits - (len(digits) - len(significant)))
	if err != nil || exponent < required || exponent > required+19-int64(len(significant)) {
		return fmt.Errorf("expected a signed 64-bit JSON integer")
	}
	normalized := significant + strings.Repeat("0", int(exponent-required))
	if negative {
		normalized = "-" + normalized
	}
	value, err := strconv.ParseInt(normalized, 10, 64)
	if err != nil {
		return fmt.Errorf("expected a signed 64-bit JSON integer")
	}
	*destination = value
	return nil
}

// Tolerant response decoding must not let an earlier alternative swallow another one's fields.
// Compare retained field paths, not values: integer normalization must not lose numeric precision.
func unmarshalUnion(data []byte, candidates []func() interface{}, nullable []bool, discriminants []map[string]string) (int, interface{}, error) {
	if len(candidates) != len(nullable) || (discriminants != nil && len(discriminants) != len(candidates)) {
		return -1, nil, fmt.Errorf("invalid union metadata")
	}
	if err := checkUnionDepth(data); err != nil {
		return -1, nil, err
	}
	trimmed := bytes.TrimSpace(data)
	isNull := bytes.Equal(trimmed, []byte("null"))
	var fields map[string]json.RawMessage
	if discriminants != nil && len(trimmed) > 0 && trimmed[0] == '{' {
		if err := json.Unmarshal(data, &fields); err != nil {
			return -1, nil, err
		}
	}
	combined := make(map[unionPath]int)
	best, bestCount := -1, -1
	var winner interface{}
	for index, create := range candidates {
		if isNull && !nullable[index] {
			continue
		}
		// Required singleton string fields come from the generator, not an SDK Robot registry.
		// Reject other branches before decoding their potentially large shared subtrees.
		if !isNull && discriminants != nil && !matchesDiscriminants(fields, discriminants[index]) {
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
		count := unionFieldPaths(retained, 0, combined)
		if count > bestCount {
			best, bestCount = index, count
			winner = candidate
		}
	}
	if best == -1 {
		return -1, nil, fmt.Errorf("invalid union JSON shape")
	}
	// Every candidate's paths are a subset of the union, so equal sizes prove complete coverage.
	// Unknown additive fields absent from every model remain tolerated, as in ordinary responses.
	if bestCount != len(combined) {
		return -1, nil, fmt.Errorf("union alternatives cannot retain all modeled fields")
	}
	// Retain only the best candidate. Decoding it again would multiply work in recursive unions.
	return best, winner, nil
}

func matchesDiscriminants(fields map[string]json.RawMessage, expected map[string]string) bool {
	for key, wanted := range expected {
		var actual string
		if err := json.Unmarshal(fields[key], &actual); err != nil || actual != wanted || bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return false
		}
	}
	return true
}

// This native resource limit bounds repeated subtree inspection, not the server's JSON schema.
// The allocation-free scan ignores delimiters inside strings; encoding/json still validates JSON.
func checkUnionDepth(data []byte) error {
	depth := 0
	inString, escaped := false, false
	for _, character := range data {
		if inString {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				inString = false
			}
			continue
		}
		switch character {
		case '"':
			inString = true
		case '[', '{':
			depth++
			if depth > 64 {
				return fmt.Errorf("union JSON exceeds 64 nested containers")
			}
		case ']', '}':
			depth--
		}
	}
	return nil
}

// Intern parent identities instead of copying full JSON pointers for every descendant.
// Raw property names need no escaping, and array positions remain distinct from object keys.
type unionPath struct {
	parent int
	key    string
	array  bool
}

func unionFieldPath(value interface{}, path unionPath, paths map[unionPath]int) int {
	id, exists := paths[path]
	if !exists {
		id = len(paths) + 1
		paths[path] = id
	}
	return 1 + unionFieldPaths(value, id, paths)
}

func unionFieldPaths(value interface{}, parent int, paths map[unionPath]int) int {
	count := 0
	switch value := value.(type) {
	case map[string]interface{}:
		for key, child := range value {
			count += unionFieldPath(child, unionPath{parent: parent, key: key}, paths)
		}
	case []interface{}:
		for index, child := range value {
			count += unionFieldPath(child, unionPath{parent: parent, key: strconv.Itoa(index), array: true}, paths)
		}
	}
	return count
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
	if operation.Auth == "api-key" && client.config.NoAccountCredentials {
		return fmt.Errorf("this operation requires account authentication")
	}
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
	accountAuth := operation.Auth
	if operation.AuthFormField != "" {
		values := fields[operation.AuthFormField]
		if operation.Auth != "basic" || operation.Encoding != "form" || len(values) != 1 {
			return fmt.Errorf("invalid account authentication selector")
		}
		selected, ok := operation.AuthFormValues[values[0]]
		if !ok || (selected != "basic" && selected != "none") {
			return fmt.Errorf("invalid account authentication selector")
		}
		accountAuth = selected
	}
	// Reject before constructing an upload body or starting any request-owned goroutine.
	if accountAuth == "basic" && (client.config.NoAccountCredentials || client.config.BearerToken != "") {
		return fmt.Errorf("this operation requires Auth Key credentials")
	}
	uri := client.config.Origin + target
	var body io.Reader
	contentType := ""
	if operation.Encoding == "query" {
		uri += "?" + fields.Encode()
	} else if operation.Encoding == "multipart/form-data" {
		for key := range extraFields {
			if strings.ContainsAny(key, "\r\n") {
				return fmt.Errorf("multipart field names cannot contain line breaks")
			}
			if _, ok := fields[key]; ok {
				return fmt.Errorf("reserved form field: %s", key)
			}
		}
		for key, file := range files {
			if strings.ContainsAny(key, "\r\n") || strings.ContainsAny(file.Filename, "\r\n") {
				return fmt.Errorf("multipart file names and field names cannot contain line breaks")
			}
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
	if accountAuth == "basic" {
		req.SetBasicAuth(client.config.AuthKey, client.config.AuthSecret)
	} else if operation.Auth == "api-key" && client.config.BearerToken != "" {
		if !operation.Bearer {
			return fmt.Errorf("this operation requires signed authentication")
		}
		req.Header.Set("Authorization", "Bearer "+client.config.BearerToken)
	}
	transport := client.httpClient
	if accountAuth == "none" {
		// A public grant must neither send ambient account cookies nor update that account's jar.
		credentialless := *transport
		credentialless.Jar = nil
		transport = &credentialless
	}
	response, err := transport.Do(req)
	if err != nil {
		return &TransportError{Cause: redactRequestURL(err)}
	}
	defer response.Body.Close()
	// Bounded independently of Content-Length, which can be absent or untrusted.
	const limit = 128 * 1024 * 1024
	data, err := ioutil.ReadAll(io.LimitReader(response.Body, limit+1))
	if len(data) > limit {
		// This resource-safety failure is deliberately not a ResponseError: workflow retries of
		// 429/5xx must not repeatedly download oversized bodies. Status-bearing size-limit errors
		// would need a separate, explicitly non-retriable error contract.
		return fmt.Errorf("API response exceeds size limit")
	}
	if err != nil {
		// An interrupted error body must not erase received status or server-requested backoff.
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return &ResponseError{Status: response.StatusCode, RetryAfter: retryAfterDuration(response.Header.Get("Retry-After"), time.Now()), Cause: redactRequestURL(err)}
		}
		return &TransportError{Cause: redactRequestURL(err)}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if !json.Valid(data) {
			data = nil
		}
		return &ResponseError{Status: response.StatusCode, Data: data, RetryAfter: retryAfterDuration(response.Header.Get("Retry-After"), time.Now())}
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
