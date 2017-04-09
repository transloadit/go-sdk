package transloadit

type Template struct {
	Id      string          `json:"id"`
	Name    string          `json:"name"`
	Content TemplateContent `json:"content"`
}

type TemplateContent struct {
	Steps map[string]interface{} `json:"steps"`
}

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

// CreateTemplate will save the provided template sturct as a new template
// and return the ID of the new template.
func (client *Client) CreateTemplate(template Template) (string, error) {
	content := map[string]interface{}{
		"name":     template.Name,
		"template": template.Content,
	}

	if err := client.request("POST", "templates", content, &template); err != nil {
		return "", err
	}

	return template.Id, nil
}

// GetTemplate will retrieve details about the template associated with the
// provided template ID.
func (client *Client) GetTemplate(templateId string) (template Template, err error) {
	err = client.request("GET", "templates/"+templateId, nil, &template)
	return template, err
}

// DeleteTemplate will delete the template associated with the provided
// template ID.
func (client *Client) DeleteTemplate(templateId string) error {
	return client.request("DELETE", "templates/"+templateId, nil, nil)
}

// UpdateTemplate will update the template associated with the provided
// template ID to match the new name and  new content. Please be aware that you
// are not able to change a template's ID.
func (client *Client) UpdateTemplate(templateId string, newTemplate Template) error {
	// Create signature
	content := map[string]interface{}{
		"name":     newTemplate.Name,
		"template": newTemplate.Content,
	}

	return client.request("PUT", "templates/"+templateId, content, nil)
}

// ListTemplate will retrieve all templates matching the criteria.
func (client *Client) ListTemplates(options *ListOptions) (list TemplateList, err error) {
	err = client.listRequest("templates", options, &list)
	return list, err
}
