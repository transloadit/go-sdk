package transloadit

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

type Assembly struct {
	client *Client
	// Notify url to send a request to once the assembly finishes.
	// See https://transloadit.com/docs#notifications.
	NotifyUrl string
	// Optional template id to use instead of adding steps.
	TemplateId string
	// Wait until the assembly completes (or is canceled).
	Blocking bool
	steps    map[string]map[string]interface{}
	readers  []*upload
}

type upload struct {
	Field  string
	Name   string
	Reader io.ReadCloser
}

type AssemblyReplay struct {
	assemblyUrl string
	client      *Client
	// Notify url to send a request to once the assembly finishes.
	// See https://transloadit.com/docs#notifications.
	NotifyUrl string
	// Wait until the assembly completes (or is canceled).
	Blocking bool
	// Reparse the template when replaying. Useful if the template has changed
	// since the orignal assembly was created.
	ReparseTemplate bool
	steps           map[string]map[string]interface{}
}

type AssemblyList struct {
	Assemblies []*AssemblyListItem `json:"items"`
	Count      int                 `json:"count"`
}

type AssemblyListItem struct {
	AssemblyId        string     `json:"id"`
	AccountId         string     `json:"account_id"`
	TemplateId        string     `json:"template_id"`
	Instance          string     `json:"instance"`
	NotifyUrl         string     `json:"notify_url"`
	RedirectUrl       string     `json:"redirect_url"`
	ExecutionDuration float32    `json:"execution_duration"`
	ExecutionStart    *time.Time `json:"execution_start"`
	Created           time.Time  `json:"created"`
	Ok                string     `json:"ok"`
	Error             string     `json:"error"`
	Files             string     `json:"files"`
}

type AssemblyInfo struct {
	AssemblyId             string                 `json:"assembly_id"`
	ParentId               string                 `json:"parent_id"`
	AssemblyUrl            string                 `json:"assembly_url"`
	AssemblySslUrl         string                 `json:"assembly_ssl_url"`
	BytesReceived          int                    `json:"bytes_received"`
	BytesExpected          int                    `json:"bytes_expected"`
	ClientAgent            string                 `json:"client_agent"`
	ClientIp               string                 `json:"client_ip"`
	ClientReferer          string                 `json:"client_referer"`
	StartDate              string                 `json:"start_date"`
	IsInfinite             bool                   `json:"is_infinite"`
	HasDupeJobs            bool                   `json:"has_dupe_jobs"`
	UploadDuration         float32                `json:"upload_duration"`
	NotifyUrl              string                 `json:"notify_url"`
	NotifyStart            string                 `json:"notify_start"`
	NotifyStatus           string                 `json:"notify_status"`
	NotifyDuation          float32                `json:"notify_duration"`
	LastJobCompleted       string                 `json:"last_job_completed"`
	ExecutionDuration      float32                `json:"execution_duration"`
	ExecutionStart         string                 `json:"execution_start"`
	Created                string                 `json:"created"`
	Ok                     string                 `json:"ok"`
	Message                string                 `json:"message"`
	Files                  string                 `json:"files"`
	Fields                 map[string]interface{} `json:"fields"`
	BytesUsage             int                    `json:"bytes_usage"`
	FilesToStoreOnS3       int                    `json:"files_to_store_on_s3"`
	QueuedFilesToStoreOnS3 int                    `json:"queued_files_to_store_on_s3"`
	ExecutingJobs          []string               `json:"executing_jobs"`
	StartedJobs            []string               `json:"started_jobs"`
	ParentAssemblyStatus   *AssemblyInfo          `json:"parent_assembly_status"`
	Uploads                []*FileInfo            `json:"uploads"`
	Results                map[string][]*FileInfo `json:"results"`
	Params                 string                 `json:"params"`
	Error                  string                 `json:"error"`
}

type FileInfo struct {
	Id               string                 `json:"id"`
	Name             string                 `json:"name"`
	Basename         string                 `json:"basename"`
	Ext              string                 `json:"ext"`
	Size             int                    `json:"size"`
	Mime             string                 `json:"mime"`
	Type             string                 `json:"type"`
	Field            string                 `json:"field"`
	Md5Hash          string                 `json:"md5hash"`
	OriginalMd5Hash  string                 `json:"original_md5hash"`
	OriginalId       string                 `json:"original_id"`
	OriginalBasename string                 `json:"original_basename"`
	Url              string                 `json:"url"`
	SslUrl           string                 `json:"ssl_url"`
	Meta             map[string]interface{} `json:"meta"`
}

// Create a new assembly instance which can be executed later.
func (client *Client) NewAssembly() *Assembly {
	return &Assembly{
		client:  client,
		steps:   make(map[string]map[string]interface{}),
		readers: make([]*upload, 0),
	}
}

// Add another reader to upload later.
func (assembly *Assembly) AddReader(field, name string, reader io.ReadCloser) {
	assembly.readers = append(assembly.readers, &upload{
		Field:  field,
		Name:   name,
		Reader: reader,
	})
}

// Add another file to upload later.
func (assembly *Assembly) AddFile(field, name string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}

	assembly.AddReader(field, name, file)
	return nil
}

// Add a step to the assembly.
func (assembly *Assembly) AddStep(name string, details map[string]interface{}) {
	assembly.steps[name] = details
}

// Start the assembly and upload all files.
// When an error is returned you should also check AssemblyInfo.Error for more
// information about the error. This happens when there is an error returned by
// the Transloadit API:
//  info, err := assembly.Upload()
//  if err != nil {
//  	if info != nil && info.Error != "" {
//  		// See info.Error
//  	}
//  	panic(err)
//  }
func (assembly *Assembly) Upload() (*AssemblyInfo, error) {
	req, err := assembly.makeRequest()
	if err != nil {
		return nil, fmt.Errorf("failed to create assembly request: %s", err)
	}

	var info AssemblyInfo
	// TODO: add context.Context
	if err = assembly.client.doRequest(req, &info); err != nil {
		return nil, err
	}

	if info.Error != "" {
		return &info, fmt.Errorf("failed to create assembly: %s", info.Error)
	}

	if !assembly.Blocking {
		return &info, err
	}

	waiter := assembly.client.WaitForAssembly(info.AssemblyUrl)

	select {
	case res := <-waiter.Response:
		// Assembly completed
		return res, nil
	case err := <-waiter.Error:
		// Error appeared
		return nil, err
	}
}

func (assembly *Assembly) makeRequest() (*http.Request, error) {
	// TODO: test with huge files
	url := assembly.client.config.Endpoint + "/assemblies"
	bodyReader, bodyWriter := io.Pipe()
	multiWriter := multipart.NewWriter(bodyWriter)

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

	// All writes to the multipart.Writer multiWriter _must_ happen inside this
	// goroutine because the writer is connected to the HTTP requst using an
	// in-memory pipe. Therefore a write to the multipart.Writer will block until
	// a corresponding read is happening from the HTTP request. The gist is that
	// the writes and reads must not occur sequentially but in parallel.
	go func() {
		defer bodyWriter.Close()
		defer multiWriter.Close()
		// Add additional keys and values

		if err := multiWriter.WriteField("params", params); err != nil {
			fmt.Println(fmt.Errorf("unable to write params field: %s", err))
		}
		if err := multiWriter.WriteField("signature", signature); err != nil {
			fmt.Println(fmt.Errorf("unable to write signature field: %s", err))
		}

		// Add files to upload
		for _, reader := range assembly.readers {
			defer reader.Reader.Close()

			part, err := multiWriter.CreateFormFile(reader.Field, reader.Name)
			if err != nil {
				fmt.Println(fmt.Errorf("unable to create form field: %s", err))
			}

			if _, err := io.Copy(part, reader.Reader); err != nil {
				fmt.Println(fmt.Errorf("unable to create upload request: %s", err))
			}
		}
	}()

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("unable to create upload request: %s", err)
	}

	req.Header.Set("Content-Type", multiWriter.FormDataContentType())

	return req, nil
}

// Get information about an assembly using its url.
func (client *Client) GetAssembly(assemblyUrl string) (*AssemblyInfo, error) {
	var info AssemblyInfo
	err := client.request("GET", assemblyUrl, nil, &info)

	return &info, err
}

// Cancel an assembly using its URL. This function will return the updated
// information about the assembly after the cancellation.
func (client *Client) CancelAssembly(assemblyUrl string) (*AssemblyInfo, error) {
	var info AssemblyInfo
	err := client.request("DELETE", assemblyUrl, nil, &info)

	return &info, err
}

// Create a new AssemblyReplay instance.
func (client *Client) NewAssemblyReplay(assemblyUrl string) *AssemblyReplay {
	return &AssemblyReplay{
		client:      client,
		steps:       make(map[string]map[string]interface{}),
		assemblyUrl: assemblyUrl,
	}
}

// Add a step to override the original ones.
func (assembly *AssemblyReplay) AddStep(name string, details map[string]interface{}) {
	assembly.steps[name] = details
}

// Start the assembly replay.
func (assembly *AssemblyReplay) Start() (*AssemblyInfo, error) {
	options := map[string]interface{}{
		"steps": assembly.steps,
	}

	if assembly.ReparseTemplate {
		options["reparse_template"] = 1
	}

	if assembly.NotifyUrl != "" {
		options["notify_url"] = assembly.NotifyUrl
	}

	var info AssemblyInfo
	err := assembly.client.request("POST", assembly.assemblyUrl+"/replay", options, &info)
	if err != nil {
		return nil, err
	}

	if info.Error != "" {
		return &info, fmt.Errorf("failed to start assembly replay: %s", info.Error)
	}

	if !assembly.Blocking {
		return &info, nil
	}

	waiter := assembly.client.WaitForAssembly(info.AssemblyUrl)

	select {
	case res := <-waiter.Response:
		// Assembly completed
		return res, nil
	case err := <-waiter.Error:
		// Error appeared
		return nil, err
	}
}

// List all assemblies matching the criterias.
func (client *Client) ListAssemblies(options *ListOptions) (*AssemblyList, error) {
	var assemblies AssemblyList
	err := client.listRequest("assemblies", options, &assemblies)

	return &assemblies, err
}
