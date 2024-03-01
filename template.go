package transloadit

import (
	"context"
	"encoding/json"
	"fmt"
)

// Template contains details about a single template.
type Template struct {
	ID                   string
	Name                 string
	Content              TemplateContent
	RequireSignatureAuth bool
}

// TemplateContent contains details about the content of a single template.
// The Steps fields maps to the `steps` key in the JSON format. The AdditionalProperties
// field allows you to store additional keys (such as `notify_url`) on the same
// level as the `steps` key.
// For example, the following instance
//
//	TemplateContent{
//		Steps: map[string]interface{}{
//			":original": map[string]interface{}{
//				"robot": "/upload/handle",
//			},
//			"resize": map[string]interface{}{
//				"robot": "/image/resize",
//			},
//		},
//		AdditionalProperties: map[string]interface{}{
//			"notify_url":           "https://example.com",
//			"allow_steps_override": false,
//		},
//	}
//
// is represented by following JSON:
//
//	 {
//		"steps": {
//			":original": {
//				"robot": "/upload/handle"
//			},
//			"resize": {
//				"robot": "/image/resize"
//			}
//		},
//		"allow_steps_override": false,
//		"notify_url": "https://example.com"
//	}
type TemplateContent struct {
	Steps                map[string]interface{}
	AdditionalProperties map[string]interface{}
}

func (content *TemplateContent) UnmarshalJSON(b []byte) error {
	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}

	if stepsRaw, ok := data["steps"]; ok {
		steps, ok := stepsRaw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("transloadit: steps property in template content is not an object but %v", stepsRaw)
		}

		content.Steps = steps
		delete(data, "steps")
	}

	if content.AdditionalProperties == nil {
		content.AdditionalProperties = make(map[string]interface{}, len(data))
	}

	for key, val := range data {
		content.AdditionalProperties[key] = val
	}

	return nil
}

func (content TemplateContent) MarshalJSON() ([]byte, error) {
	// Add a hint for the size of the map to reduce the number of necessary allocations
	// when filling the map.
	numKeys := len(content.AdditionalProperties) + 1
	data := make(map[string]interface{}, numKeys)

	data["steps"] = content.Steps

	for key, val := range content.AdditionalProperties {
		data[key] = val
	}

	return json.Marshal(data)
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
			make(map[string]interface{}),
		},
	}
}

// AddStep will add the provided step to the Template.Content.Steps map.
func (template *Template) AddStep(name string, step map[string]interface{}) {
	template.Content.Steps[name] = step
}

// templateInternal is the struct we use for encoding/decoding the Template
// JSON since we need to convert between boolean and integer.
type templateInternal struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	Content              TemplateContent `json:"content"`
	RequireSignatureAuth int             `json:"require_signature_auth"`
}

func (template *Template) UnmarshalJSON(b []byte) error {
	var internal templateInternal
	if err := json.Unmarshal(b, &internal); err != nil {
		return err
	}

	template.Name = internal.Name
	template.Content = internal.Content
	template.ID = internal.ID
	if internal.RequireSignatureAuth == 1 {
		template.RequireSignatureAuth = true
	} else {
		template.RequireSignatureAuth = false
	}

	return nil
}

func (template Template) MarshalJSON() ([]byte, error) {
	var internal templateInternal

	internal.Name = template.Name
	internal.Content = template.Content
	internal.ID = template.ID
	if template.RequireSignatureAuth {
		internal.RequireSignatureAuth = 1
	} else {
		internal.RequireSignatureAuth = 0
	}

	return json.Marshal(internal)
}

// CreateTemplate will save the provided template struct as a new template
// and return the ID of the new template.
func (client *Client) CreateTemplate(ctx context.Context, template Template) (string, error) {
	content := map[string]interface{}{
		"name":     template.Name,
		"template": template.Content,
	}
	if template.RequireSignatureAuth {
		content["require_signature_auth"] = 1
	}

	if err := client.request(ctx, "POST", "templates", content, &template); err != nil {
		return "", err
	}

	return template.ID, nil
}

// GetTemplate will retrieve details about the template associated with the
// provided template ID.
func (client *Client) GetTemplate(ctx context.Context, templateID string) (template Template, err error) {
	err = client.request(ctx, "GET", "templates/"+templateID, nil, &template)
	return template, err
}

// DeleteTemplate will delete the template associated with the provided
// template ID.
func (client *Client) DeleteTemplate(ctx context.Context, templateID string) error {
	return client.request(ctx, "DELETE", "templates/"+templateID, nil, nil)
}

// UpdateTemplate will update the template associated with the provided
// template ID to match the new name and  new content. Please be aware that you
// are not able to change a template's ID.
func (client *Client) UpdateTemplate(ctx context.Context, templateID string, newTemplate Template) error {
	// Create signature
	content := map[string]interface{}{
		"name":     newTemplate.Name,
		"template": newTemplate.Content,
	}
	if newTemplate.RequireSignatureAuth {
		content["require_signature_auth"] = 1
	} else {
		content["require_signature_auth"] = 0
	}

	return client.request(ctx, "PUT", "templates/"+templateID, content, nil)
}

// ListTemplates will retrieve all templates matching the criteria.
func (client *Client) ListTemplates(ctx context.Context, options *ListOptions) (list TemplateList, err error) {
	err = client.listRequest(ctx, "templates", options, &list)
	return list, err
}
