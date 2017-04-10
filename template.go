package transloadit

import "context"

// Template contains details about a single template.
type Template struct {
	Id      string          `json:"id"`
	Name    string          `json:"name"`
	Content TemplateContent `json:"content"`
}

// TemplateContent contains details about the content of a single template.
type TemplateContent struct {
	Steps map[string]interface{} `json:"steps"`
}

// TemplateList contains a list of templates.
type TemplateList struct {
	Templates []Template `json:"items"`
	Count     int        `json:"count"`
}

// NewTemplate returns a new Template struct with initialized values. This
// template will not be saved to Transloadit. To do so, please use the
// Client.CreateTemplate function.
func NewTemplate() Template {
	return Template{
		Content: TemplateContent{
			make(map[string]interface{}),
		},
	}
}

// AddStep will add the provided step to the Template.Content.Steps map.
func (template *Template) AddStep(name string, step map[string]interface{}) {
	template.Content.Steps[name] = step
}

// CreateTemplate will save the provided template struct as a new template
// and return the ID of the new template.
func (client *Client) CreateTemplate(ctx context.Context, template Template) (string, error) {
	content := map[string]interface{}{
		"name":     template.Name,
		"template": template.Content,
	}

	if err := client.request(ctx, "POST", "templates", content, &template); err != nil {
		return "", err
	}

	return template.Id, nil
}

// GetTemplate will retrieve details about the template associated with the
// provided template ID.
func (client *Client) GetTemplate(ctx context.Context, templateId string) (template Template, err error) {
	err = client.request(ctx, "GET", "templates/"+templateId, nil, &template)
	return template, err
}

// DeleteTemplate will delete the template associated with the provided
// template ID.
func (client *Client) DeleteTemplate(ctx context.Context, templateId string) error {
	return client.request(ctx, "DELETE", "templates/"+templateId, nil, nil)
}

// UpdateTemplate will update the template associated with the provided
// template ID to match the new name and  new content. Please be aware that you
// are not able to change a template's ID.
func (client *Client) UpdateTemplate(ctx context.Context, templateId string, newTemplate Template) error {
	// Create signature
	content := map[string]interface{}{
		"name":     newTemplate.Name,
		"template": newTemplate.Content,
	}

	return client.request(ctx, "PUT", "templates/"+templateId, content, nil)
}

// ListTemplate will retrieve all templates matching the criteria.
func (client *Client) ListTemplates(ctx context.Context, options *ListOptions) (list TemplateList, err error) {
	err = client.listRequest(ctx, "templates", options, &list)
	return list, err
}
