package transloadit

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// BearerTokenRequest contains options for exchanging a Client's Auth Key and
// Auth Secret for a scoped bearer token via Client.IssueBearerToken.
type BearerTokenRequest struct {
	// Scope restricts the token to a space-separated list of scopes, e.g.
	// "assemblies:read assemblies:write". If empty, the token inherits all
	// scopes granted to the Auth Key.
	Scope string
	// Audience sets the optional token audience. If empty, the deployment's
	// default audience is used.
	Audience string
}

// BearerToken contains a bearer token issued by the Transloadit API. Details
// about each value can be found at https://transloadit.com/docs/api/tokens/.
type BearerToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

// IssueBearerToken exchanges the Client's Auth Key and Auth Secret for a
// scoped bearer token by calling POST /token. Unlike other requests made by
// this Client, the token endpoint is authenticated using HTTP Basic auth
// rather than the usual HMAC request signature.
func (client *Client) IssueBearerToken(ctx context.Context, tokenRequest BearerTokenRequest) (*BearerToken, error) {
	form := url.Values{"grant_type": {"client_credentials"}}
	if tokenRequest.Scope != "" {
		form.Set("scope", tokenRequest.Scope)
	}
	if tokenRequest.Audience != "" {
		form.Set("aud", tokenRequest.Audience)
	}

	req, err := http.NewRequest("POST", client.config.Endpoint+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("issue bearer token: %s", err)
	}
	req = req.WithContext(ctx)
	req.SetBasicAuth(client.config.AuthKey, client.config.AuthSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var token BearerToken
	if err := client.doRequest(req, &token); err != nil {
		return nil, err
	}

	return &token, nil
}
