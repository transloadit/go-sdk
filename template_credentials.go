package transloadit

import (
	"context"
)

// Template contains details about a single template.
type TemplateCredential struct {
	ID                   string		`json:"id"`
	Name                 string     `json:"name"`
	Type              	 string 	`json:"type"`
	Content				 map[string]interface{} `json:"content"`
}

type TemplatePostCredential struct {
	Credentials 		TemplateCredential `json:"credentials"`
	OK 					string `json:"ok"`
	Message 			string `json:"message"`
}

// TemplateList contains a list of templates.
type TemplateCredentialList struct {
	TemplateCredential []Template `json:"credentials"`
	OK 					string `json:"ok"`
	Message 			string `json:"message"`
}

// NewTemplate returns a new Template struct with initialized values. This
// template will not be saved to Transloadit. To do so, please use the
// Client.CreateTemplate function.
func NewTemplateCredential() TemplateCredential {
	return TemplateCredential{
		Content: make(map[string]interface{}),
	}
}

var templateCredentialPrefix="template_credentials"

// CreateTemplate will save the provided template struct as a new template
// and return the ID of the new template.
func (client *Client) CreateTemplateCredential(ctx context.Context, templateCredential TemplateCredential) (string, error) {
	content := map[string]interface{}{
		"name":     templateCredential.Name,
		"type": 	templateCredential.Type,
		"content": templateCredential.Content,
	}
	var response TemplatePostCredential
	if err := client.request(ctx, "POST", templateCredentialPrefix, content, &response); err != nil {
		return "", err
	}
	templateCredential = response.Credentials
	return templateCredential.ID, nil
}

// GetTemplateCredential will retrieve details about the template credential associated with the
// provided template credential ID.
func (client *Client) GetTemplateCredential(ctx context.Context, templateID string) (template TemplateCredential, err error) {
	err = client.request(ctx, "GET", templateCredentialPrefix + "/" +templateID, nil, &template)
	return template, err
}

// DeleteTemplateCredential will delete the template credential associated with the provided
// template ID.
func (client *Client) DeleteTemplateCredential(ctx context.Context, templateID string) error {
	return client.request(ctx, "DELETE", templateCredentialPrefix +"/"+templateID, nil, nil)
}

// ListTemplatesCredential will retrieve all templates credential matching the criteria.
func (client *Client) ListTemplateCredential(ctx context.Context, options *ListOptions) (list TemplateCredentialList, err error) {
	err = client.listRequest(ctx, templateCredentialPrefix, options, &list)
	return list, err
}
