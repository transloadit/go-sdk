package transloadit

import (
	"fmt"
)

type Template struct {
	Name  string
	Steps map[string]map[string]interface{}
}

func NewTemplate(name string) *Template {
	return &Template{
		Name:  name,
		Steps: make(map[string]map[string]interface{}),
	}
}

func (client *Client) CreateTemplate(template *Template) (string, error) {

	// Create signature
	content := map[string]interface{}{
		"name":     template.Name,
		"template": template.Steps,
	}

	res, err := client.request("POST", "templates", content)
	if err != nil {
		return "", fmt.Errorf("unable to create template: %s", err)
	}

	return res["template_id"].(string), nil

}

func (client *Client) GetTemplate(templateId string) (*Template, error) {

	res, err := client.request("GET", "templates/"+templateId, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to get template: %s", err)
	}

	steps := make(map[string]map[string]interface{})
	stepsRaw := res["template_content"].(map[string]interface{})
	for key, value := range stepsRaw {
		steps[key] = value.(map[string]interface{})
	}

	return &Template{
		Name:  res["template_name"].(string),
		Steps: steps,
	}, nil

}

func (template *Template) AddStep(name string, step map[string]interface{}) {
	template.Steps[name] = step
}

func (client *Client) DeleteTemplate(templateId string) error {

	_, err := client.request("DELETE", "templates/"+templateId, nil)
	if err != nil {
		return fmt.Errorf("unable to delete template: %s", err)
	}

	return nil
}

func (client *Client) EditTemplate(templateId string, newTemplate *Template) error {

	// Create signature
	content := map[string]interface{}{
		"name":     newTemplate.Name,
		"template": newTemplate.Steps,
	}

	_, err := client.request("PUT", "templates/"+templateId, content)
	return err

}
