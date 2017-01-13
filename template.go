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

// Creates a new template instance which can be saved to transloadit.
func NewTemplate() Template {
	return Template{
		Content: TemplateContent{
			make(map[string]interface{}),
		},
	}
}

func (template *Template) AddStep(name string, step map[string]interface{}) {
	template.Content.Steps[name] = step
}

// Save the template.
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

// Get information about a template using its id.
func (client *Client) GetTemplate(templateId string) (template Template, err error) {
	err = client.request("GET", "templates/"+templateId, nil, &template)
	return template, err
}

// Delete a template from the list.
func (client *Client) DeleteTemplate(templateId string) error {
	return client.request("DELETE", "templates/"+templateId, nil, nil)
}

// Update the name and content of the template defined using the id.
func (client *Client) EditTemplate(templateId string, newTemplate Template) error {
	// Create signature
	content := map[string]interface{}{
		"name":     newTemplate.Name,
		"template": newTemplate.Content,
	}

	return client.request("PUT", "templates/"+templateId, content, nil)
}

// List all templates matching the criterias.
func (client *Client) ListTemplates(options *ListOptions) (list TemplateList, err error) {
	err = client.listRequest("templates", options, &list)
	return list, err
}
