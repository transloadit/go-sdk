package contract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
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
	ErrorCodes           []string `json:"errorCodes"`
	PublicHostPattern    string   `json:"publicHostPattern"`
	RejectedHostPrefixes []string `json:"rejectedHostPrefixes"`
	IdentityField        string   `json:"identityField"`
	AssemblyField        string   `json:"assemblyField"`
	Path                 string   `json:"path"`
	Parameter            string   `json:"parameter"`
	Pattern              string   `json:"pattern"`
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
	port := parsed.Port()
	if port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value > 65535 {
			return nil, invalidAssemblyWorkflow()
		}
		if (parsed.Scheme == "https" && value == 443) || (parsed.Scheme == "http" && value == 80) {
			port = ""
		} else {
			port = strconv.Itoa(value)
		}
	}
	parsed.Host = hostname
	if port != "" {
		parsed.Host += ":" + port
	}
	return parsed, nil
}

func assemblyOrigin(value *url.URL) string { return value.Scheme + "://" + value.Host }

func admittedAssemblyOwner(raw, id string, policy assemblyWorkflowPolicy, config Config) (string, error) {
	expectedPath := strings.ReplaceAll(policy.Path, "{"+policy.Parameter+"}", id)
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
		if !errors.As(err, &responseError) || !(responseError.Status == 429 || (responseError.Status >= 500 && responseError.Status <= 599)) {
			return nil, err
		}
		// Only safe reads retry. The context deadline bounds even a very long server hint.
		delay := interval
		if responseError.RetryAfter > delay {
			delay = responseError.RetryAfter
		}
		if err := waitWorkflowDelay(ctx, delay); err != nil {
			return nil, err
		}
	}
}

func inspectWorkflowStatus(ctx context.Context, status *AssemblyWorkflowResult, id string, policy assemblyWorkflowPolicy) (bool, string, error) {
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
		if !hasWorkflowCode(policy.ErrorCodes, string(status.WithError.Error)) {
			return false, "", invalidAssemblyWorkflow()
		}
		return true, "", workflowDeadline(ctx)
	}
	ok := status.GetOk()
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
	terminal, rawOwner, err := inspectWorkflowStatus(ctx, result, input.AssemblyID, policy)
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
				terminal, _, inspectionErr := inspectWorkflowStatus(ctx, confirmed, input.AssemblyID, policy)
				if inspectionErr != nil {
					return nil, inspectionErr
				}
				if terminal {
					return confirmed, nil
				}
			}
			return nil, err
		}
		terminal, rawOwner, err = inspectWorkflowStatus(ctx, result, input.AssemblyID, policy)
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
		terminal, rawOwner, err = inspectWorkflowStatus(ctx, result, input.AssemblyID, policy)
	}
}
