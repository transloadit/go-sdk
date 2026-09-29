package contract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
)

// AssemblyWorkflowOptions bounds discovery, cancellation and polling as one workflow.
type AssemblyWorkflowOptions struct {
	AssemblyID string
	// Timeout defaults to five minutes. An earlier context deadline always wins.
	Timeout time.Duration
	// Interval defaults to one second. Negative durations are rejected.
	Interval time.Duration
}

type assemblyWorkflowPolicy struct {
	BusyCodes            []string `json:"busyCodes"`
	TerminalOkCodes      []string `json:"terminalOkCodes"`
	UnconfirmedOkCodes   []string `json:"unconfirmedOkCodes"`
	ErrorCodes           []string `json:"errorCodes"`
	PublicHostPattern    string   `json:"publicHostPattern"`
	RejectedHostPrefixes []string `json:"rejectedHostPrefixes"`
	IdentityField        string   `json:"identityField"`
	AssemblyField        string   `json:"assemblyField"`
	Path                 string   `json:"path"`
	Parameter            string   `json:"parameter"`
	Pattern              string   `json:"pattern"`
}

// ErrAssemblyWorkflowUnconfirmed means the connection outcome cannot establish remote cleanup.
var ErrAssemblyWorkflowUnconfirmed = errors.New("Assembly connection ended; remote completion or cleanup is not confirmed")

func invalidAssemblyWorkflow() error {
	// URLs are capabilities. Do not include raw destinations, parser errors or response data.
	return fmt.Errorf("invalid Assembly workflow response or uploader destination")
}

var assemblyAuthority = regexp.MustCompile(`(?i)^https?://([a-z0-9.-]+(?::[0-9]+)?)(/[^?#]*)?$`)

func parseAssemblyDestination(raw string) (*url.URL, error) {
	match := assemblyAuthority.FindStringSubmatch(raw)
	if match == nil || strings.ContainsAny(raw, `\%`) || strings.IndexFunc(raw, func(c rune) bool { return c <= 32 || c == 127 || unicode.IsSpace(c) }) >= 0 {
		return nil, invalidAssemblyWorkflow()
	}
	hostname := strings.ToLower(strings.Split(match[1], ":")[0])
	if strings.HasSuffix(hostname, ".") {
		return nil, invalidAssemblyWorkflow()
	}
	for _, label := range strings.Split(hostname, ".") {
		if strings.HasPrefix(label, "xn--") {
			return nil, invalidAssemblyWorkflow()
		}
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == "." || segment == ".." {
			return nil, invalidAssemblyWorkflow()
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil || strings.ToLower(parsed.Hostname()) != hostname {
		return nil, invalidAssemblyWorkflow()
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (hostname == "localhost" || hostname == "127.0.0.1")) {
		return nil, invalidAssemblyWorkflow()
	}
	if err := normalizeEndpointAuthority(parsed); err != nil {
		return nil, invalidAssemblyWorkflow()
	}
	return parsed, nil
}

func assemblyOrigin(value *url.URL) string { return value.Scheme + "://" + value.Host }

func admittedAssemblyOwner(raw, id string, policy assemblyWorkflowPolicy, config Config) (string, error) {
	expectedPath := strings.ReplaceAll(policy.Path, "{"+policy.Parameter+"}", id)
	return admittedWorkflowDestination(raw, expectedPath, policy, config)
}

func admittedWorkflowDestination(raw, expectedPath string, policy assemblyWorkflowPolicy, config Config) (string, error) {
	// NewClient already validated this caller-owned endpoint. An exact match preserves its proxy
	// prefix/encoding or loopback spelling without trusting any new response-supplied destination.
	if raw == config.Origin+expectedPath {
		return config.Origin, nil
	}
	// These deployment-owned origins received the same transport validation. Admit exact IPv6
	// matches without expanding the grammar for destinations supplied only by an API response.
	for _, origin := range config.AssemblyOrigins {
		if raw == origin+expectedPath {
			return origin, nil
		}
	}
	destination, err := parseAssemblyDestination(raw)
	if err != nil {
		return "", err
	}
	if destination.Path != expectedPath {
		return "", invalidAssemblyWorkflow()
	}
	origin := assemblyOrigin(destination)
	// The configured entrypoint need not satisfy the stricter grammar for untrusted owner URLs.
	entry, err := url.Parse(config.Origin)
	if err != nil {
		return "", invalidAssemblyWorkflow()
	}
	allowed := origin == assemblyOrigin(entry)
	for _, trusted := range config.AssemblyOrigins {
		allowed = allowed || origin == trusted
	}
	public, err := regexp.MatchString(policy.PublicHostPattern, destination.Hostname())
	if err != nil {
		return "", invalidAssemblyWorkflow()
	}
	for _, prefix := range policy.RejectedHostPrefixes {
		if strings.HasPrefix(destination.Hostname(), prefix) {
			public = false
		}
	}
	if !allowed && !(destination.Scheme == "https" && destination.Port() == "" && public) {
		return "", invalidAssemblyWorkflow()
	}
	return origin, nil
}

func hasWorkflowCode(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func workflowDeadline(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// A busy decoder can observe the deadline before the timer's cancellation goroutine runs.
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return nil
}

func waitWorkflowDelay(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return workflowDeadline(ctx)
	}
}

// AssemblyUploadSession is a private checkpoint. Its URL is a secret upload capability.
type AssemblyUploadSession struct {
	Version    int    `json:"version"`
	AssemblyID string `json:"assemblyId"`
	UploadURL  string `json:"uploadUrl"`
	Size       int64  `json:"size"`
	Filename   string `json:"filename"`
	Fieldname  string `json:"fieldname"`
	SHA256     string `json:"sha256"`
}

// AssemblyUploadOptions sends a fixed-size file with bounded memory. The caller owns Reader.
// ReaderAt must return promptly (as os.File/bytes.Reader do); its bytes must not change during use.
type AssemblyUploadOptions struct {
	AssemblyID string
	Reader     io.ReaderAt
	Size       int64
	Filename   string
	Fieldname  string
	// ChunkSize defaults to 5 MiB and is limited to 64 MiB.
	ChunkSize int64
	// Timeout defaults to five minutes, including hashing and discovery.
	Timeout time.Duration
	// OnSession synchronously persists before any bytes. It must return promptly and observe the
	// caller context; the SDK cannot interrupt caller-owned synchronous I/O. Errors stop uploading.
	OnSession func(AssemblyUploadSession) error
	// MaxRetries defaults to five; a pointer to zero disables recovery. Creation never retries.
	MaxRetries *int
	RetryDelay time.Duration
}

// AssemblyUploadError preserves a checkpoint and cause without printing the secret upload URL.
type AssemblyUploadError struct {
	Session *AssemblyUploadSession
	Cause   error
}

func (err *AssemblyUploadError) Error() string {
	return "Assembly upload stopped; remote cleanup is not confirmed"
}
func (err *AssemblyUploadError) Unwrap() error { return err.Cause }

type tusHeaderRule struct {
	Name      string   `json:"name"`
	Required  bool     `json:"required"`
	Values    []string `json:"values"`
	Pattern   string   `json:"pattern"`
	MinLength *int     `json:"minLength"`
	MaxLength *int     `json:"maxLength"`
}
type tusOperation struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Parameters []struct {
		Name string `json:"name"`
	} `json:"parameters"`
	Success int `json:"success"`
	Headers struct {
		Alternatives [][]tusHeaderRule `json:"alternatives"`
	} `json:"headers"`
}
type tusWorkflowPolicy struct {
	Assembly assemblyWorkflowPolicy `json:"assembly"`
	Wire     struct {
		Version   string            `json:"version"`
		Headers   map[string]string `json:"headers"`
		MediaType string            `json:"mediaType"`
		Filename  string            `json:"filename"`
		Fieldname string            `json:"fieldname"`
	} `json:"wire"`
	CollectionField string       `json:"collectionField"`
	MetadataName    string       `json:"metadataName"`
	Create          tusOperation `json:"create"`
	Head            tusOperation `json:"head"`
	Patch           tusOperation `json:"patch"`
}

func invalidUpload() error {
	return fmt.Errorf("invalid upload session, protocol response or destination")
}

func validateTusHeaders(operation tusOperation, headers map[string]string) bool {
	for _, rules := range operation.Headers.Alternatives {
		valid, named := true, make(map[string]bool)
		for _, rule := range rules {
			named[rule.Name] = true
			value, exists := headers[rule.Name]
			if !exists {
				valid = valid && !rule.Required
				continue
			}
			if rule.Values != nil && !hasWorkflowCode(rule.Values, value) {
				valid = false
			}
			if rule.Pattern != "" {
				matched, err := regexp.MatchString(rule.Pattern, value)
				valid = valid && err == nil && matched
			}
			length := len([]rune(value))
			valid = valid && (rule.MinLength == nil || length >= *rule.MinLength) && (rule.MaxLength == nil || length <= *rule.MaxLength)
		}
		for name := range headers {
			valid = valid && named[name]
		}
		if valid {
			return true
		}
	}
	return false
}

// requestTus makes one generated protocol request, with no ambient authentication or redirects.
func (client *Client) requestTus(ctx context.Context, operation tusOperation, target string, headers map[string]string, body []byte) (http.Header, error) {
	if !validateTusHeaders(operation, headers) {
		return nil, invalidUpload()
	}
	request, err := http.NewRequestWithContext(ctx, operation.Method, target, bytes.NewReader(body))
	if err != nil {
		return nil, invalidUpload()
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, &TransportError{Cause: redactRequestURL(err)}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &ResponseError{Status: response.StatusCode, RetryAfter: retryAfterDuration(response.Header.Get("Retry-After"), time.Now())}
	}
	if response.StatusCode != operation.Success {
		return nil, invalidUpload()
	}
	if err := workflowDeadline(ctx); err != nil {
		return nil, err
	}
	return response.Header.Clone(), nil
}

func admitUploadURL(raw string, operation tusOperation, policy tusWorkflowPolicy, config Config) (string, error) {
	if strings.ContainsAny(raw, "\\?#") || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return "", invalidUpload()
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == "." || segment == ".." {
			return "", invalidUpload()
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", invalidUpload()
	}
	path := operation.Path
	if len(operation.Parameters) == 1 {
		escaped := parsed.EscapedPath()
		id := escaped[strings.LastIndex(escaped, "/")+1:]
		if id == "" || strings.Contains(id, "%") || id == "." || id == ".." {
			return "", invalidUpload()
		}
		path = strings.ReplaceAll(path, "{"+operation.Parameters[0].Name+"}", id)
	} else if strings.HasSuffix(raw, "/") {
		// Collection trailing slashes are canonicalized like the producer's capability adapter.
		raw = strings.TrimSuffix(raw, "/")
	}
	origin, err := admittedWorkflowDestination(raw, path, policy.Assembly, config)
	if err != nil {
		return "", err
	}
	return origin + path, nil
}

func tusOffset(headers http.Header, name string, size int64) (int64, error) {
	values := headers.Values(name)
	if len(values) != 1 {
		return 0, invalidUpload()
	}
	text := values[0]
	if text == "" || strings.Trim(text, "0123456789") != "" || (len(text) > 1 && text[0] == '0') {
		return 0, invalidUpload()
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || value > size {
		return 0, invalidUpload()
	}
	return value, nil
}

func verifyUploadMetadata(raw string, expected map[string]string) bool {
	actual := make(map[string]string)
	for _, entry := range strings.Split(raw, ",") {
		pair := strings.SplitN(strings.TrimSpace(entry), " ", 2)
		if len(pair) != 2 {
			return false
		}
		if _, duplicate := actual[pair[0]]; duplicate {
			return false
		}
		value, err := base64.StdEncoding.DecodeString(pair[1])
		if err != nil {
			return false
		}
		actual[pair[0]] = string(value)
	}
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func retryableWorkflowTransport(failure error) bool {
	// url.Error implements net.Error even for certificate or redirect-policy failures.
	// Unwrap that context before asking whether the actual cause can recover.
	for {
		var requestError *url.Error
		if !errors.As(failure, &requestError) {
			break
		}
		failure = requestError.Err
	}
	var network net.Error
	return errors.Is(failure, io.EOF) || errors.Is(failure, io.ErrUnexpectedEOF) ||
		errors.Is(failure, syscall.ECONNRESET) || errors.Is(failure, syscall.ECONNREFUSED) || errors.Is(failure, syscall.EPIPE) ||
		(errors.As(failure, &network) && (network.Timeout() || network.Temporary()))
}

func (client *Client) runTusUpload(parent context.Context, input AssemblyUploadOptions, resume *AssemblyUploadSession) (result *AssemblyUploadSession, failure error) {
	if input.Timeout < 0 || input.RetryDelay < 0 || input.ChunkSize < 0 || input.Size < 0 || input.Reader == nil {
		return nil, invalidUpload()
	}
	if input.Timeout == 0 {
		input.Timeout = 5 * time.Minute
	}
	if input.RetryDelay == 0 {
		input.RetryDelay = time.Second
	}
	if input.ChunkSize == 0 {
		input.ChunkSize = 5 * 1024 * 1024
	}
	if input.ChunkSize > 64*1024*1024 || input.Size > 9007199254740991 {
		return nil, invalidUpload()
	}
	maxRetries := 5
	if input.MaxRetries != nil {
		maxRetries = *input.MaxRetries
	}
	if maxRetries < 0 || maxRetries > 100 {
		return nil, invalidUpload()
	}
	if input.Fieldname == "" {
		input.Fieldname = "file"
	}
	if input.Filename == "" || strings.ContainsAny(input.Filename+input.Fieldname, "\r\n\x00") {
		return nil, invalidUpload()
	}
	var policy tusWorkflowPolicy
	if json.Unmarshal([]byte(tusWorkflowPolicyJSON), &policy) != nil {
		return nil, invalidUpload()
	}
	valid, err := regexp.MatchString(policy.Assembly.Pattern, input.AssemblyID)
	if err != nil || !valid {
		return nil, invalidUpload()
	}
	ctx, stop := context.WithTimeout(parent, input.Timeout)
	defer stop()
	var session *AssemblyUploadSession
	if resume != nil {
		copy := *resume
		session = &copy
	}
	defer func() {
		if failure != nil {
			if deadline := workflowDeadline(ctx); deadline != nil {
				failure = deadline
			}
			failure = &AssemblyUploadError{Session: session, Cause: failure}
		}
	}()
	if err := workflowDeadline(ctx); err != nil {
		return nil, err
	}
	digest := sha256.New()
	buffer := make([]byte, input.ChunkSize)
	for position := int64(0); position < input.Size; {
		count := input.ChunkSize
		if remaining := input.Size - position; remaining < count {
			count = remaining
		}
		if _, err := io.ReadFull(io.NewSectionReader(input.Reader, position, count), buffer[:count]); err != nil {
			return nil, err
		}
		digest.Write(buffer[:count])
		position += count
		if err := workflowDeadline(ctx); err != nil {
			return nil, err
		}
	}
	sha := hex.EncodeToString(digest.Sum(nil))
	if session != nil && (session.Version != 1 || session.AssemblyID != input.AssemblyID || session.Size != input.Size || session.Filename != input.Filename || session.Fieldname != input.Fieldname || session.SHA256 != sha) {
		return nil, invalidUpload()
	}
	if session != nil {
		if _, err := admitUploadURL(session.UploadURL, policy.Head, policy, client.config); err != nil {
			return nil, err
		}
	}
	retries := 0
	recover := func(failure error) error {
		if err := workflowDeadline(ctx); err != nil {
			return err
		}
		var response *ResponseError
		retry := retryableWorkflowTransport(failure)
		if errors.As(failure, &response) {
			retry = response.Status == 409 || response.Status == 429 || (response.Status >= 500 && response.Status <= 599)
		}
		if !retry || retries >= maxRetries {
			return failure
		}
		retries++
		delay := input.RetryDelay
		if response != nil && response.RetryAfter > delay {
			delay = response.RetryAfter
		}
		return waitWorkflowDelay(ctx, delay)
	}
	httpClient := *client.httpClient
	httpClient.Jar = nil
	connection := &Client{config: client.config, httpClient: &httpClient}
	var status *AssemblyWorkflowResult
	for {
		status, err = connection.readWorkflowAssembly(ctx, input.AssemblyID)
		if err == nil {
			break
		}
		if err := recover(err); err != nil {
			return nil, err
		}
	}
	if status == nil || workflowIdentity(status) != input.AssemblyID {
		return nil, invalidUpload()
	}
	owner, err := admittedAssemblyOwner(workflowOwner(status), input.AssemblyID, policy.Assembly, client.config)
	if err != nil {
		return nil, err
	}
	assemblyURL := owner + strings.ReplaceAll(policy.Assembly.Path, "{"+policy.Assembly.Parameter+"}", input.AssemblyID)
	collection, err := admitUploadURL(workflowCollection(status), policy.Create, policy, client.config)
	if err != nil {
		return nil, err
	}
	wire := policy.Wire
	expectedMetadata := map[string]string{policy.MetadataName: assemblyURL, wire.Filename: input.Filename, wire.Fieldname: input.Fieldname}
	headers := func() map[string]string { return map[string]string{wire.Headers["resumable"]: wire.Version} }
	if session == nil {
		creation := headers()
		creation[wire.Headers["length"]] = strconv.FormatInt(input.Size, 10)
		var parts []string
		for _, key := range []string{policy.MetadataName, wire.Filename, wire.Fieldname} {
			parts = append(parts, key+" "+base64.StdEncoding.EncodeToString([]byte(expectedMetadata[key])))
		}
		creation[wire.Headers["metadata"]] = strings.Join(parts, ",")
		response, err := connection.requestTus(ctx, policy.Create, collection, creation, nil)
		if err != nil {
			return nil, err
		}
		if response.Get(wire.Headers["resumable"]) != wire.Version {
			return nil, invalidUpload()
		}
		locations := response.Values(wire.Headers["location"])
		if len(locations) != 1 || locations[0] == "" || strings.ContainsAny(locations[0], "\\?#") || strings.IndexFunc(locations[0], unicode.IsSpace) >= 0 {
			return nil, invalidUpload()
		}
		for _, segment := range strings.Split(locations[0], "/") {
			segment = strings.ReplaceAll(strings.ToLower(segment), "%2e", ".")
			if segment == "." || segment == ".." {
				return nil, invalidUpload()
			}
		}
		base, _ := url.Parse(collection)
		relative, err := url.Parse(locations[0])
		if err != nil {
			return nil, invalidUpload()
		}
		uploadURL, err := admitUploadURL(base.ResolveReference(relative).String(), policy.Head, policy, client.config)
		if err != nil {
			return nil, err
		}
		session = &AssemblyUploadSession{Version: 1, AssemblyID: input.AssemblyID, UploadURL: uploadURL, Size: input.Size, Filename: input.Filename, Fieldname: input.Fieldname, SHA256: sha}
		if input.OnSession != nil {
			if err := input.OnSession(*session); err != nil {
				return nil, err
			}
		}
	}
	uploadURL, err := admitUploadURL(session.UploadURL, policy.Head, policy, client.config)
	if err != nil {
		return nil, err
	}
	head := func() (int64, error) {
		for {
			response, err := connection.requestTus(ctx, policy.Head, uploadURL, headers(), nil)
			if err != nil {
				if err := recover(err); err != nil {
					return 0, err
				}
				continue
			}
			length, err := tusOffset(response, wire.Headers["length"], input.Size)
			if err != nil || length != input.Size || response.Get(wire.Headers["resumable"]) != wire.Version || !verifyUploadMetadata(response.Get(wire.Headers["metadata"]), expectedMetadata) {
				return 0, invalidUpload()
			}
			return tusOffset(response, wire.Headers["offset"], input.Size)
		}
	}
	position, err := head()
	if err != nil {
		return nil, err
	}
	for position < input.Size {
		if err := workflowDeadline(ctx); err != nil {
			return nil, err
		}
		count := input.ChunkSize
		if remaining := input.Size - position; remaining < count {
			count = remaining
		}
		// RoundTripper may finish reading a body after returning its response. Each PATCH owns its
		// bytes until transport closure, so a later chunk cannot overwrite an in-flight request.
		chunk := make([]byte, count)
		if _, err := io.ReadFull(io.NewSectionReader(input.Reader, position, count), chunk); err != nil {
			return nil, err
		}
		patchHeaders := headers()
		patchHeaders[wire.Headers["offset"]] = strconv.FormatInt(position, 10)
		patchHeaders[wire.Headers["contentType"]] = wire.MediaType
		response, err := connection.requestTus(ctx, policy.Patch, uploadURL, patchHeaders, chunk)
		if err != nil {
			if err := recover(err); err != nil {
				return nil, err
			}
			confirmed, err := head()
			if err != nil {
				return nil, err
			}
			if confirmed < position || confirmed > position+count {
				return nil, invalidUpload()
			}
			position = confirmed
			continue
		}
		next, err := tusOffset(response, wire.Headers["offset"], input.Size)
		// tus acknowledges bytes actually stored, which may be fewer than this request offered.
		if err != nil || next <= position || next > position+count || response.Get(wire.Headers["resumable"]) != wire.Version {
			return nil, invalidUpload()
		}
		position = next
	}
	if err := workflowDeadline(ctx); err != nil {
		return nil, err
	}
	return session, nil
}

func (client *Client) readWorkflowStatus(ctx context.Context, id string, interval time.Duration) (*AssemblyWorkflowResult, error) {
	for {
		if err := workflowDeadline(ctx); err != nil {
			return nil, err
		}
		result, err := client.readWorkflowAssembly(ctx, id)
		if err == nil {
			return result, nil
		}
		var responseError *ResponseError
		retry := retryableWorkflowTransport(err)
		if errors.As(err, &responseError) {
			retry = responseError.Status == 429 || (responseError.Status >= 500 && responseError.Status <= 599)
		}
		if !retry {
			return nil, err
		}
		// Only safe reads retry. The context deadline bounds even a very long server hint.
		delay := interval
		if responseError != nil && responseError.RetryAfter > delay {
			delay = responseError.RetryAfter
		}
		if err := waitWorkflowDelay(ctx, delay); err != nil {
			return nil, err
		}
	}
}

func inspectWorkflowStatus(ctx context.Context, status *AssemblyWorkflowResult, id string, policy assemblyWorkflowPolicy, allowUnconfirmed bool) (bool, string, error) {
	if err := workflowDeadline(ctx); err != nil {
		return false, "", err
	}
	if status == nil {
		return false, "", invalidAssemblyWorkflow()
	}
	if workflowIdentity(status) != id {
		return false, "", invalidAssemblyWorkflow()
	}
	// Generated union decoding rejects ambiguous ok+error bodies. Read its typed variant without
	// serializing the complete files/results graph again on every poll.
	if status.WithError != nil {
		// The producer and generated union use a closed error enum. Open-enum evolution needs a
		// source-owned type change, not an unchecked cast of new wire values into this snapshot.
		if !hasWorkflowCode(policy.ErrorCodes, string(status.WithError.Error)) {
			return false, "", invalidAssemblyWorkflow()
		}
		return true, "", workflowDeadline(ctx)
	}
	ok := status.GetOk()
	if hasWorkflowCode(policy.UnconfirmedOkCodes, ok) {
		// Only initial cancellation discovery can proceed to the one owner-routed DELETE.
		// A repeated connection outcome still cannot prove that remote cleanup succeeded.
		if !allowUnconfirmed {
			return false, "", ErrAssemblyWorkflowUnconfirmed
		}
		return false, workflowOwner(status), nil
	}
	if hasWorkflowCode(policy.TerminalOkCodes, ok) {
		return true, "", workflowDeadline(ctx)
	}
	owner := workflowOwner(status)
	if !hasWorkflowCode(policy.BusyCodes, ok) || owner == "" {
		return false, "", invalidAssemblyWorkflow()
	}
	return false, owner, workflowDeadline(ctx)
}

func (client *Client) runAssemblyWorkflow(parent context.Context, input AssemblyWorkflowOptions, cancelAssembly bool) (*AssemblyWorkflowResult, error) {
	if input.Timeout < 0 || input.Interval < 0 {
		return nil, fmt.Errorf("workflow durations must not be negative")
	}
	if input.Timeout == 0 {
		input.Timeout = 5 * time.Minute
	}
	if input.Interval == 0 {
		input.Interval = time.Second
	}
	var policy assemblyWorkflowPolicy
	if json.Unmarshal([]byte(assemblyWorkflowPolicyJSON), &policy) != nil {
		return nil, invalidAssemblyWorkflow()
	}
	validID, err := regexp.MatchString(policy.Pattern, input.AssemblyID)
	if err != nil || !validID {
		return nil, invalidAssemblyWorkflow()
	}
	ctx, stop := context.WithTimeout(parent, input.Timeout)
	defer stop()
	if err := workflowDeadline(ctx); err != nil {
		return nil, err
	}
	// Workflows use only generated credential-free operations. Do not carry an ambient cookie jar.
	httpClient := *client.httpClient
	httpClient.Jar = nil
	entry := &Client{config: client.config, httpClient: &httpClient}
	result, err := entry.readWorkflowStatus(ctx, input.AssemblyID, input.Interval)
	if err != nil {
		if deadline := workflowDeadline(ctx); deadline != nil {
			return nil, deadline
		}
		return nil, err
	}
	terminal, rawOwner, err := inspectWorkflowStatus(ctx, result, input.AssemblyID, policy, cancelAssembly)
	if err != nil {
		return nil, err
	}
	if terminal {
		return result, nil
	}
	origin, err := admittedAssemblyOwner(rawOwner, input.AssemblyID, policy, client.config)
	if err != nil {
		return nil, err
	}
	config := client.config
	config.Origin = origin
	owner := &Client{config: config, httpClient: &httpClient}
	if cancelAssembly {
		result, err = owner.cancelWorkflowAssembly(ctx, input.AssemblyID)
		if err != nil {
			if deadline := workflowDeadline(ctx); deadline != nil {
				return nil, deadline
			}
			var responseError *ResponseError
			if errors.As(err, &responseError) {
				// DELETE can race expiration/failure. Confirm through the generated GET, not an
				// unchecked error-body cast, and never repeat the cancellation write.
				confirmed, readErr := owner.readWorkflowStatus(ctx, input.AssemblyID, input.Interval)
				if readErr != nil {
					return nil, readErr
				}
				terminal, _, inspectionErr := inspectWorkflowStatus(ctx, confirmed, input.AssemblyID, policy, false)
				if inspectionErr != nil {
					return nil, inspectionErr
				}
				if terminal {
					return confirmed, nil
				}
			}
			return nil, err
		}
		terminal, rawOwner, err = inspectWorkflowStatus(ctx, result, input.AssemblyID, policy, false)
	}
	for {
		if err != nil {
			return nil, err
		}
		if terminal {
			return result, nil
		}
		current, admissionErr := admittedAssemblyOwner(rawOwner, input.AssemblyID, policy, client.config)
		if admissionErr != nil || current != origin {
			return nil, invalidAssemblyWorkflow()
		}
		if err := waitWorkflowDelay(ctx, input.Interval); err != nil {
			return nil, err
		}
		result, err = owner.readWorkflowStatus(ctx, input.AssemblyID, input.Interval)
		if err != nil {
			if deadline := workflowDeadline(ctx); deadline != nil {
				return nil, deadline
			}
			return nil, err
		}
		terminal, rawOwner, err = inspectWorkflowStatus(ctx, result, input.AssemblyID, policy, false)
	}
}
