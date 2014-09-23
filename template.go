package transloadit

import (
	"fmt"
)

type Template struct {
	Name string `json:"template_name"`
	// See AddStep for simple usage.
	Steps map[string]map[string]interface{} `json:"template_content"`
}

type TemplateList struct {
	Templates []TemplateListItem `json:"items"`
	Count     int                `json:"count"`
}

type TemplateListItem struct {
	Id    string                            `json:"id"`
	Name  string                            `json:"name"`
	Steps map[string]map[string]interface{} `json:"json"`
}

// Creates a new template instance which can be saved to transloadit.
func NewTemplate(name string) *Template {
	return &Template{
		Name:  name,
		Steps: make(map[string]map[string]interface{}),
	}
}

// Save the template.
func (client *Client) CreateTemplate(template *Template) (string, error) {

	// Create signature
	content := map[string]interface{}{
		"name":     template.Name,
		"template": template.Steps,
	}

	res, err := client.request("POST", "templates", content, nil)
	if err != nil {
		return "", fmt.Errorf("unable to create template: %s", err)
	}

	return res["template_id"].(string), nil

}

// Get information about a template using its id.
func (client *Client) GetTemplate(templateId string) (*Template, error) {

	var template Template
	_, err := client.request("GET", "templates/"+templateId, nil, &template)
	if err != nil {
		return nil, fmt.Errorf("unable to get template: %s", err)
	}

	return &template, nil
}

// Add another step to the template.
func (template *Template) AddStep(name string, step map[string]interface{}) {
	template.Steps[name] = step
}

// Delete a template from the list.
func (client *Client) DeleteTemplate(templateId string) error {

	_, err := client.request("DELETE", "templates/"+templateId, nil, nil)
	if err != nil {
		return fmt.Errorf("unable to delete template: %s", err)
	}

	return nil
}

// Update the name and content of the template defined using the id.
func (client *Client) EditTemplate(templateId string, newTemplate *Template) error {

	// Create signature
	content := map[string]interface{}{
		"name":     newTemplate.Name,
		"template": newTemplate.Steps,
	}

	_, err := client.request("PUT", "templates/"+templateId, content, nil)
	return err

}

// List all templates matching the criterias.
func (client *Client) ListTemplates(options *ListOptions) (*TemplateList, error) {

	var templates TemplateList
	_, err := client.listRequest("templates", options, &templates)
	return &templates, err

}
