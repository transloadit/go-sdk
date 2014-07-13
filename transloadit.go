package transloadit

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
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
	Expires string `json:"expires"`
}

func (self *Params) Init() {
	const layout = "2006/01/02 15:04:05+00:00"
	t := time.Now().AddDate(0, 0, 1)
	self.Auth.Expires = t.Format(layout)
}

func NewInstance(apikey, secret string) (instance *Instance, err error) {
	instance = &Instance{
		apikey:   apikey,
		secret:   secret,
		endpoint: "https://api2.transloadit.com",
	}
	url, err := instance.getBoredInstance()
	if err != nil {
		return nil, err
	}
	instance.endpoint = "https://" + url
	return instance, nil
}

func (self *Instance) getBoredInstance() (string, error) {
	res, err := http.Get(self.endpoint + "/instances/bored")
	if err != nil {
		return "", err
	}
	j, err := ioutil.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		return "", err
	}
	var decoded map[string]string
	json.Unmarshal(j, &decoded)

	return decoded["api2_host"], nil
}

func (self *Instance) CreateAssembly(p Params, filepath string) (*bytes.Buffer, error) {
	result, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}

	sig, err := newSignature(p, self.secret)
	if err != nil {
		return nil, err
	}

	request, err := newfileUploadRequest(self.endpoint+"/assemblies", result, filepath, string(sig))
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

func newSignature(p Params, secret string) ([]byte, error) {
	e, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(e)
	s := mac.Sum(nil)
	return s, nil
}

func newfileUploadRequest(uri string, params []byte, path string, sig string) (*http.Request, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

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

	err = writer.WriteField("signature", sig)
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
