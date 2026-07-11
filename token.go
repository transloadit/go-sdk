// Code generated from Transloadit API2 contracts; DO NOT EDIT.
// If it looks wrong, please report the issue instead of editing this file by hand;
// the source fix belongs in the contract generator so all SDKs stay in sync.

package transloadit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type BearerTokenOptions struct {
	Aud   string `json:"aud,omitempty"`
	Scope string `json:"scope,omitempty"`
}

type BearerTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

func (client *Client) IssueBearerToken(ctx context.Context, options BearerTokenOptions) (BearerTokenResponse, error) {
	form := url.Values{}
	if options.Aud != "" {
		form.Set("aud", options.Aud)
	}
	form.Set("grant_type", "client_credentials")
	if options.Scope != "" {
		form.Set("scope", options.Scope)
	}

	endpoint := strings.TrimRight(client.config.Endpoint, "/") + "/token"
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return BearerTokenResponse{}, fmt.Errorf("parse bearer token endpoint: %w", err)
	}
	hostname := endpointURL.Hostname()
	ip := net.ParseIP(hostname)
	loopback := hostname == "localhost" || (ip != nil && ip.IsLoopback())
	if endpointURL.User != nil || (endpointURL.Scheme != "https" && !(endpointURL.Scheme == "http" && loopback)) {
		return BearerTokenResponse{}, fmt.Errorf("refusing to send credentials to bearer token endpoint %q", endpointURL.Redacted())
	}
	request, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return BearerTokenResponse{}, fmt.Errorf("create bearer token request: %w", err)
	}
	request.SetBasicAuth(client.config.AuthKey, client.config.AuthSecret)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Transloadit-Client", "go-sdk:"+Version)

	httpClient := *client.httpClient
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return BearerTokenResponse{}, fmt.Errorf("issue bearer token: %w", err)
	}
	defer response.Body.Close()

	body := io.LimitReader(response.Body, 128*1024*1024)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var requestError RequestError
		if err := json.NewDecoder(body).Decode(&requestError); err != nil {
			return BearerTokenResponse{}, fmt.Errorf("decode bearer token error: %w", err)
		}
		return BearerTokenResponse{}, requestError
	}

	var token BearerTokenResponse
	if err := json.NewDecoder(body).Decode(&token); err != nil {
		return BearerTokenResponse{}, fmt.Errorf("decode bearer token response: %w", err)
	}
	return token, nil
}
