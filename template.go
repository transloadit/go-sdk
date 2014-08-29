package transloadit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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

	uri := client.config.Endpoint + "/templates"

	// Encode template content to JSON
	templateStr, err := json.Marshal(template.Steps)
	if err != nil {
		return "", fmt.Errorf("unable to create template: %s", err)
	}

	// Create signature
	params, signature, err := client.sign(map[string]interface{}{
		"name":     template.Name,
		"template": string(templateStr),
	})
	if err != nil {
		return "", fmt.Errorf("unable to create template: %s", err)
	}

	// Transform values to querystring
	v := url.Values{}
	v.Set("params", params)
	v.Set("signature", signature)

	req, err := http.NewRequest("POST", uri, strings.NewReader(v.Encode()))
	if err != nil {
		return "", fmt.Errorf("unable to create template: %s", err)
	}

	// Add content type header
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.doRequest(req)
	if err != nil {
		return "", fmt.Errorf("unable to create template: %s", err)
	}

	return res["template_id"].(string), nil

}

func (client *Client) GetTemplate(templateId string) (*Template, error) {

	uri := client.config.Endpoint + "/templates/" + templateId

	// Create signature
	params, signature, err := client.sign(map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("unable to get template: %s", err)
	}

	v := url.Values{}
	v.Set("params", params)
	v.Set("signature", signature)

	// Add params and signature to uri
	uri += "?" + v.Encode()

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to get template: %s", err)
	}

	res, err := client.doRequest(req)
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

	uri := client.config.Endpoint + "/templates/" + templateId

	// Create signature
	params, signature, err := client.sign(map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("unable to delete template: %s", err)
	}

	v := url.Values{}
	v.Set("params", params)
	v.Set("signature", signature)

	req, err := http.NewRequest("DELETE", uri, strings.NewReader(v.Encode()))
	if err != nil {
		return fmt.Errorf("unable to delete template: %s", err)
	}

	// Add content type header
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err = client.doRequest(req)
	if err != nil {
		return fmt.Errorf("unable to delete template: %s", err)
	}

	return nil
}
