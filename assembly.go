package transloadit

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

type Assembly struct {
	client     *Client
	NotifyUrl  string
	TemplateId string
	steps      map[string]map[string]interface{}
	readers    map[string]io.Reader
}

type AssemblyResponse map[string]interface{}

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

	params, signature, err := assembly.client.sign(map[string]interface{}{
		"steps": assembly.steps,
	})
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
