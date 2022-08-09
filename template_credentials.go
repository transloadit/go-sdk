package transloadit

import (
	"context"
)

// TemplateCredential contains details about a single template credential.
type TemplateCredential struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Content     map[string]interface{} `json:"content"`
	Created     string                 `json:"created",omitempty`
	Modified    string                 `json:"modified",omitempty`
	Deleted     string                 `json:"deleted",omitempty`
	Stringified string                 `json:"stringified",omitempty`
}

type templateCredentialResponseBody struct {
	Credential TemplateCredential `json:"credential"`
	OK         string             `json:"ok"`
	Message    string             `json:"message"`
}

// TemplateCredentialList contains a list of template credentials.
type TemplateCredentialList struct {
	TemplateCredential []TemplateCredential `json:"credentials"`
	OK                 string     `json:"ok"`
	Message            string     `json:"message"`
}

// NewTemplateCredential returns a new TemplateCredential struct with initialized values. This
// template credential will not be saved to Transloadit. To do so, please use the
// Client.CreateTemplateCredential function.
func NewTemplateCredential() TemplateCredential {
	return TemplateCredential{
		Content: make(map[string]interface{}),
	}
}

var templateCredentialPrefix = "template_credentials"

// CreateTemplateCredential will save the provided template credential struct to the server
// and return the ID of the new template credential.
func (client *Client) CreateTemplateCredential(ctx context.Context, templateCredential TemplateCredential) (string, error) {
	content := map[string]interface{}{
		"name":    templateCredential.Name,
		"type":    templateCredential.Type,
		"content": templateCredential.Content,
	}
	var response templateCredentialResponseBody
	if err := client.request(ctx, "POST", templateCredentialPrefix, content, &response); err != nil {
		return "", err
	}
	return response.Credential.ID, nil
}

// GetTemplateCredential will retrieve details about the template credential associated with the
// provided template credential ID.
func (client *Client) GetTemplateCredential(ctx context.Context, templateID string) (template TemplateCredential, err error) {
	var response templateCredentialResponseBody
	err = client.request(ctx, "GET", templateCredentialPrefix+"/"+templateID, nil, &response)
	template = response.Credential
	return template, err
}

// DeleteTemplateCredential will delete the template credential associated with the provided
// template ID.
func (client *Client) DeleteTemplateCredential(ctx context.Context, templateID string) error {
	return client.request(ctx, "DELETE", templateCredentialPrefix+"/"+templateID, nil, nil)
}

// ListTemplateCredential will retrieve all templates credential matching the criteria.
func (client *Client) ListTemplateCredential(ctx context.Context, options *ListOptions) (list TemplateCredentialList, err error) {
	err = client.listRequest(ctx, templateCredentialPrefix, options, &list)
	return list, err
}
