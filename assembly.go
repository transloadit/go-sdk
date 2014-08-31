package transloadit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type Assembly struct {
	client     *Client
	NotifyUrl  string
	TemplateId string
	steps      map[string]map[string]interface{}
	readers    map[string]io.Reader
}

type AssemblyReplay struct {
	assemblyId      string
	client          *Client
	NotifyUrl       string
	reparseTemplate bool
	steps           map[string]map[string]interface{}
}

type AssemblyList struct {
	Assemblies []*AssemblyListItem `json:"items"`
	Count      int                 `json:"count"`
}

type AssemblyListItem struct {
	Id                string    `json:"id"`
	AccountId         string    `json:"account_id"`
	TemplateId        string    `json:"template_id"`
	Instance          string    `json:"instance"`
	NotifyUrl         string    `json:"notify_url"`
	RedirectUrl       string    `json:"redirect_url"`
	ExecutionDuration float32   `json:"execution_duration"`
	ExecutionStart    time.Time `json:"execution_start"`
	Created           time.Time `json:"created"`
	Ok                string    `json:"ok"`
	Error             string    `json:"error"`
	Files             string    `json:"files"`
}

func (client *Client) CreateAssembly() *Assembly {
	return &Assembly{
		client:  client,
		steps:   make(map[string]map[string]interface{}),
		readers: make(map[string]io.Reader),
	}
}

func (assembly *Assembly) AddReader(name string, reader io.Reader) {
	assembly.readers[name] = reader
}

func (assembly *Assembly) AddStep(name string, details map[string]interface{}) {
	assembly.steps[name] = details
}

func (assembly *Assembly) Upload() (Response, error) {
	req, err := assembly.makeRequest()
	if err != nil {
		return nil, fmt.Errorf("failed to create assembly: %s", err)
	}

	return assembly.client.doRequest(req)
}

func (assembly *Assembly) makeRequest() (*http.Request, error) {

	// Get bored instance to upload files to
	bored, err := assembly.client.getBoredInstance()
	if err != nil {
		return nil, fmt.Errorf("unable to create upload request: %s", err)
	}
	url := "http://api2-" + bored + "/assemblies"

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add files to upload
	for index, reader := range assembly.readers {

		part, err := writer.CreateFormFile(index, index)
		if err != nil {
			return nil, fmt.Errorf("unable to create upload request: %s", err)
		}

		_, err = io.Copy(part, reader)
		if err != nil {
			return nil, fmt.Errorf("unable to create upload request: %s", err)
		}
	}

	options := make(map[string]interface{})

	if len(assembly.steps) != 0 {
		options["steps"] = assembly.steps
	}

	if assembly.TemplateId != "" {
		options["template_id"] = assembly.TemplateId
	}

	if assembly.NotifyUrl != "" {
		options["notify_url"] = assembly.NotifyUrl
	}

	params, signature, err := assembly.client.sign(options)
	if err != nil {
		return nil, fmt.Errorf("unable to create upload request: %s", err)
	}

	// Add additional keys and values
	err = writer.WriteField("params", params)
	if err != nil {
		return nil, fmt.Errorf("unable to create upload request: %s", err)
	}
	err = writer.WriteField("signature", signature)
	if err != nil {
		return nil, fmt.Errorf("unable to create upload request: %s", err)
	}

	// Close multipart writer
	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("unable to create upload request: %s", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("unable to create upload request: %s", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req, nil

}

func (client *Client) GetAssembly(assemblyUrl string) (Response, error) {

	return client.request("GET", assemblyUrl, nil)

}

func (client *Client) CancelAssembly(assemblyUrl string) (Response, error) {

	return client.request("DELETE", assemblyUrl, nil)

}

func (client *Client) ReplayAssembly(assemblyId string) *AssemblyReplay {
	return &AssemblyReplay{
		client:     client,
		steps:      make(map[string]map[string]interface{}),
		assemblyId: assemblyId,
	}
}

func (assembly *AssemblyReplay) AddStep(name string, details map[string]interface{}) {
	assembly.steps[name] = details
}

func (assembly *AssemblyReplay) ReparseTemplate() {
	assembly.reparseTemplate = true
}

func (assembly *AssemblyReplay) Start() (Response, error) {

	options := map[string]interface{}{
		"steps": assembly.steps,
	}

	if assembly.reparseTemplate {
		options["reparse_template"] = 1
	}

	if assembly.NotifyUrl != "" {
		options["notify_url"] = assembly.NotifyUrl
	}

	return assembly.client.request("POST", "assemblies/"+assemblyId+"/replay", options)

}

func (client *Client) ListAssemblies(options *ListOptions) (*AssemblyList, error) {

	body, err := client.listRequest("assemblies", options)
	if err != nil {
		return nil, fmt.Errorf("unable to list assemblies: %s", err)
	}

	var assemblies AssemblyList
	err = json.Unmarshal(body, &assemblies)
	if err != nil {
		return nil, fmt.Errorf("unable to list assemblies: %s", err)
	}

	return &assemblies, nil
}
