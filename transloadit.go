package transloadit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

type Instance struct {
	apikey   string
	secret   string
	endpoint string
}

type Request struct {
	Files  []string `json:"files"`
	Params Params   `json:"params"`
}

type Params struct {
	Auth        Auth                   `json:"auth"`
	Steps       map[string]interface{} `json:"steps"`
	TemplateId  string                 `json:"template_id,omitempty"`
	NotifyUrl   string                 `json:"notify_url,omitempty"`
	RedirectUrl string                 `json:"redirect_url,omitempty"`
}

type Auth struct {
	Key     string `json:"key"`
	Expires string `json:"expires,omitempty"`
}

func NewInstance(apikey, secret string) (instance *Instance, err error) {
	instance = &Instance{
		apikey:   apikey,
		secret:   secret,
		endpoint: "http://api2.transloadit.com/assemblies",
	}
	return instance, nil
}

func (self *Instance) SendRequest(p Params, filepath string) (*bytes.Buffer, error) {
	result, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}

	request, err := newfileUploadRequest(self.endpoint, result, filepath)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	} else {
		body := &bytes.Buffer{}
		_, err := body.ReadFrom(resp.Body)
		if err != nil {
			return nil, err
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			err := fmt.Errorf("Server responded with code %d:%v", resp.StatusCode, body)
			return nil, err
		}
		log.Println(resp.StatusCode)
		log.Println(resp.Header)
		log.Println(body)
		return body, nil
	}
}

func newfileUploadRequest(uri string, params []byte, path string) (*http.Request, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	//body := &bytes.Buffer{}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return nil, err
	}

	err = writer.WriteField("params", string(params))
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest("POST", uri, &body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request, nil
}
