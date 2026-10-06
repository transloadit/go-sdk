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
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
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
	BusyCodes                 []string `json:"busyCodes"`
	TerminalOkCodes           []string `json:"terminalOkCodes"`
	CancelableTerminalOkCodes []string `json:"cancelableTerminalOkCodes"`
	Error                     struct {
		MinLength int `json:"minLength"`
	} `json:"error"`
	PublicHostPattern    string   `json:"publicHostPattern"`
	RejectedHostPrefixes []string `json:"rejectedHostPrefixes"`
	IdentityField        string   `json:"identityField"`
	AssemblyField        string   `json:"assemblyField"`
	Path                 string   `json:"path"`
	Parameter            string   `json:"parameter"`
	Pattern              string   `json:"pattern"`
}

// ErrAssemblyWorkflowUnconfirmed means explicit cancellation could not discover an uploader.
var ErrAssemblyWorkflowUnconfirmed = errors.New("Assembly cancellation could not be confirmed; no uploader destination is available")

// AssemblyCancellationConfirmationError retains a failed DELETE and failed confirmation GET.
// Its message excludes response text. The cancellation is the primary cause; both failures remain
// directly available even when errors.As would match their same concrete type.
type AssemblyCancellationConfirmationError struct {
	CancellationError error
	ConfirmationError error
}

func (err *AssemblyCancellationConfirmationError) Error() string {
	return "Assembly cancellation could not be confirmed"
}

func (err *AssemblyCancellationConfirmationError) Unwrap() error { return err.CancellationError }

func (err *AssemblyCancellationConfirmationError) Is(target error) bool {
	return errors.Is(err.CancellationError, target) || errors.Is(err.ConfirmationError, target)
}

func (err *AssemblyCancellationConfirmationError) As(target interface{}) bool {
	// Go 1.15 supports only single-error Unwrap. Preserve cancellation-first inspection of both.
	return errors.As(err.CancellationError, target) || errors.As(err.ConfirmationError, target)
}

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
	allowed := origin == assemblyOrigin(entry) && (entry.Path == "" || entry.Path == "/")
	for _, trusted := range config.AssemblyOrigins {
		allowed = allowed || origin == trusted
	}
	// A proxy path is part of the trusted endpoint, not implicit permission for its bare origin.
	if origin == assemblyOrigin(entry) && !allowed {
		return "", invalidAssemblyWorkflow()
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
	// Fieldname defaults to the saved session's field name when resuming, otherwise "file".
	Fieldname string
	// ChunkSize defaults to 5 MiB and is limited to 64 MiB.
	ChunkSize int64
	// Timeout defaults to five minutes, including hashing and discovery.
	Timeout time.Duration
	// OnSession synchronously persists before any bytes, using the upload's deadline-bound context.
	// It must return promptly; the SDK cannot interrupt caller-owned synchronous I/O. Errors stop uploading.
	OnSession func(context.Context, AssemblyUploadSession) error
	// MaxRetries defaults to five; a pointer to zero disables recovery. Creation never retries.
	MaxRetries *int
	RetryDelay time.Duration
}

// AssemblyUploadError preserves a checkpoint and cause without printing the secret upload URL.
type AssemblyUploadError struct {
	Session *AssemblyUploadSession
	Cause   error
	// AssemblyCode is the observed terminal state, when it prevented upload writes.
	AssemblyCode string
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
		Version   string                    `json:"version"`
		Headers   map[string]string         `json:"headers"`
		MediaType string                    `json:"mediaType"`
		Filename  string                    `json:"filename"`
		Fieldname string                    `json:"fieldname"`
		Identity  tusMetadataIdentityPolicy `json:"identity"`
		Receipt   struct {
			MatchCount    int  `json:"matchCount"`
			FinishedValue bool `json:"finishedValue"`
		} `json:"receipt"`
	} `json:"wire"`
	CollectionField string       `json:"collectionField"`
	MetadataName    string       `json:"metadataName"`
	Create          tusOperation `json:"create"`
	Head            tusOperation `json:"head"`
	Patch           tusOperation `json:"patch"`
}

type tusMetadataIdentityPolicy struct {
	KeyPattern   string `json:"keyPattern"`
	ValuePattern string `json:"valuePattern"`
}

// The generator binds wire fields to these native roles; orchestration owns no field inventory.
type tusUploadReceipt struct {
	URL, Filename, Fieldname string
	Size, Offset             float64
	Finished                 bool
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
	// Protocol replies are small. Draining bounded bodies permits connection reuse without
	// accepting unbounded error data or embedding that data in diagnostic messages.
	const limit = 64 * 1024
	data, err := ioutil.ReadAll(io.LimitReader(response.Body, limit+1))
	if len(data) > limit {
		return nil, fmt.Errorf("tus response exceeds size limit")
	}
	if err != nil {
		// Keep failure status/backoff even when draining the protocol body loses its connection.
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, &ResponseError{Status: response.StatusCode, RetryAfter: retryAfterDuration(response.Header.Get("Retry-After"), time.Now()), Cause: redactRequestURL(err)}
		}
		return nil, &TransportError{Cause: redactRequestURL(err)}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if !json.Valid(data) {
			data = nil
		}
		return nil, &ResponseError{Status: response.StatusCode, Data: data, RetryAfter: retryAfterDuration(response.Header.Get("Retry-After"), time.Now())}
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
		// These bounded helpers exclude encoded/nested IDs, rather than guessing a store's layout.
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

func verifyUploadMetadata(raw string, expected map[string]string, policy tusMetadataIdentityPolicy) bool {
	keyPattern, err := regexp.Compile(policy.KeyPattern)
	if err != nil {
		return false
	}
	valuePattern, err := regexp.Compile(policy.ValuePattern)
	if err != nil {
		return false
	}
	actual := make(map[string]string)
	for _, entry := range strings.Split(raw, ",") {
		pair := strings.SplitN(entry, " ", 2)
		encoded := ""
		if len(pair) == 2 {
			encoded = pair[1]
		}
		if !keyPattern.MatchString(pair[0]) || !valuePattern.MatchString(encoded) {
			return false
		}
		if _, duplicate := actual[pair[0]]; duplicate {
			return false
		}
		value, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil {
			return false
		}
		actual[pair[0]] = string(value)
	}
	for key, value := range expected {
		if decoded, present := actual[key]; !present || decoded != value {
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
		if resume != nil {
			input.Fieldname = resume.Fieldname
		}
	}
	if input.Filename == "" || input.Fieldname == "" || strings.ContainsAny(input.Filename+input.Fieldname, "\r\n\x00") {
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
	assemblyCode := ""
	if resume != nil {
		copy := *resume
		session = &copy
	}
	defer func() {
		if failure != nil {
			if deadline := workflowDeadline(ctx); deadline != nil {
				failure = deadline
			}
			failure = &AssemblyUploadError{Session: session, Cause: failure, AssemblyCode: assemblyCode}
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
	recover := func(failure error, allowOffsetConflict bool) error {
		if err := workflowDeadline(ctx); err != nil {
			return err
		}
		var response *ResponseError
		retry := retryableWorkflowTransport(failure)
		if errors.As(failure, &response) {
			retry = (allowOffsetConflict && response.Status == 409) || response.Status == 429 || (response.Status >= 500 && response.Status <= 599)
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
	discover := func() (*AssemblyWorkflowResult, error) {
		for {
			status, err := connection.readWorkflowAssembly(ctx, input.AssemblyID)
			if err == nil {
				return status, nil
			}
			if err := recover(err, false); err != nil {
				return nil, err
			}
		}
	}
	inspect := func(status *AssemblyWorkflowResult) (bool, error) {
		state, err := inspectAssemblyState(status, input.AssemblyID, policy.Assembly)
		if err != nil {
			return false, err
		}
		if state.busy {
			return true, nil
		}
		assemblyCode = state.code
		// File receipt and processing success are separate outcomes. A saved session can verify
		// completion even after processing fails, but no stopped state authorizes another write.
		if session == nil || workflowOwner(status) == "" || workflowCollection(status) == "" {
			return false, fmt.Errorf("Assembly is not accepting upload writes")
		}
		return false, nil
	}
	status, err := discover()
	if err != nil {
		return nil, err
	}
	canWrite, err := inspect(status)
	if err != nil {
		return nil, err
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
			if err := input.OnSession(ctx, *session); err != nil {
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
				var missing *ResponseError
				if errors.As(err, &missing) && missing.Status == 404 {
					// API2 retains finished tus receipts after temporary files disappear. Absence or
					// Assembly completion alone is not proof, and must never create another upload.
					refreshed, refreshErr := discover()
					if refreshErr != nil {
						return 0, refreshErr
					}
					if _, refreshErr := inspect(refreshed); refreshErr != nil {
						return 0, refreshErr
					}
					matches, complete := 0, false
					for _, upload := range workflowUploadReceipts(refreshed) {
						receiptURL, admissionErr := admitUploadURL(upload.URL, policy.Head, policy, client.config)
						if admissionErr != nil || receiptURL != uploadURL {
							continue
						}
						matches++
						complete = upload.Finished == wire.Receipt.FinishedValue && upload.Size == float64(input.Size) && upload.Offset == float64(input.Size) &&
							upload.Filename == input.Filename && upload.Fieldname == input.Fieldname
					}
					if matches == wire.Receipt.MatchCount && complete {
						return input.Size, workflowDeadline(ctx)
					}
					return 0, err
				}
				if err := recover(err, true); err != nil {
					return 0, err
				}
				continue
			}
			length, err := tusOffset(response, wire.Headers["length"], input.Size)
			metadata := response.Values(wire.Headers["metadata"])
			if err != nil || length != input.Size || response.Get(wire.Headers["resumable"]) != wire.Version || len(metadata) != 1 || !verifyUploadMetadata(metadata[0], expectedMetadata, wire.Identity) {
				return 0, invalidUpload()
			}
			return tusOffset(response, wire.Headers["offset"], input.Size)
		}
	}
	position, err := head()
	if err != nil {
		return nil, err
	}
	// A completed transfer may be confirmed idempotently, but a stopped Assembly must not
	// receive any more bytes. This is the observed state; the server still arbitrates later races.
	if position < input.Size && !canWrite {
		return nil, fmt.Errorf("Assembly is not accepting upload writes")
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
			if err := recover(err, true); err != nil {
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

type assemblyReaderState struct {
	code         string
	busy, failed bool
}

// The generated wire schema owns error admission; known values are not an exhaustive inventory.
func inspectAssemblyState(status *AssemblyWorkflowResult, id string, policy assemblyWorkflowPolicy) (assemblyReaderState, error) {
	if status == nil || workflowIdentity(status) != id {
		return assemblyReaderState{}, invalidAssemblyWorkflow()
	}
	// Generated union decoding rejects ambiguous ok+error bodies. Read its typed variant without
	// serializing the complete files/results graph again on every poll.
	if status.WithError != nil {
		code := string(status.WithError.Error)
		if utf8.RuneCountInString(code) < policy.Error.MinLength {
			return assemblyReaderState{}, invalidAssemblyWorkflow()
		}
		return assemblyReaderState{code: code, failed: true}, nil
	}
	ok := status.GetOk()
	if hasWorkflowCode(policy.TerminalOkCodes, ok) {
		return assemblyReaderState{code: ok}, nil
	}
	if hasWorkflowCode(policy.BusyCodes, ok) {
		return assemblyReaderState{code: ok, busy: true}, nil
	}
	return assemblyReaderState{}, invalidAssemblyWorkflow()
}

func inspectWorkflowStatus(ctx context.Context, status *AssemblyWorkflowResult, id string, policy assemblyWorkflowPolicy, requireCancellation bool) (bool, string, error) {
	if err := workflowDeadline(ctx); err != nil {
		return false, "", err
	}
	state, err := inspectAssemblyState(status, id, policy)
	if err != nil {
		return false, "", err
	}
	if state.failed {
		return true, "", workflowDeadline(ctx)
	}
	if requireCancellation && hasWorkflowCode(policy.CancelableTerminalOkCodes, state.code) {
		// A failed request is finite for waiters, but an explicit cancel must still reach its owner.
		return false, workflowOwner(status), nil
	}
	if !state.busy {
		return true, "", workflowDeadline(ctx)
	}
	owner := workflowOwner(status)
	if owner == "" {
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
	if cancelAssembly && hasWorkflowCode(policy.CancelableTerminalOkCodes, result.GetOk()) && rawOwner == "" {
		return nil, ErrAssemblyWorkflowUnconfirmed
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
			if errors.As(err, &responseError) || retryableWorkflowTransport(err) {
				// A DELETE can race completion or lose its reply after it was applied. Confirm
				// through the generated GET, never an error-body cast or another write.
				confirmed, confirmationErr := owner.readWorkflowStatus(ctx, input.AssemblyID, input.Interval)
				if confirmationErr == nil {
					terminal, _, confirmationErr = inspectWorkflowStatus(ctx, confirmed, input.AssemblyID, policy, true)
				}
				if confirmationErr != nil {
					if deadline := workflowDeadline(ctx); deadline != nil {
						return nil, deadline
					}
					// Failed reads and unusable confirmations both retain the original DELETE diagnostic.
					return nil, &AssemblyCancellationConfirmationError{CancellationError: err, ConfirmationError: confirmationErr}
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
