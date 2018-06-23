package transloadit

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

// Assembly contains instructions used for starting assemblies.
type Assembly struct {
	// NotifiyURL specifies a URL to which a request will be sent once the
	// assembly finishes.
	// See https://transloadit.com/docs#notifications.
	NotifyURL string
	// TemplateID specifies a optional template from which the encoding
	// instructions will be fetched.
	// See https://transloadit.com/docs/#15-templates
	TemplateID string

	steps   map[string]map[string]interface{}
	readers []*upload
}

type upload struct {
	Field  string
	Name   string
	Reader io.ReadCloser
}

// AssemblyReplay contains instructions used for replaying assemblies.
type AssemblyReplay struct {
	// NotifiyURL specifies a URL to which a request will be sent once the
	// assembly finishes. This overwrites the notify url from the original
	// assembly instructions.
	// See https://transloadit.com/docs#notifications.
	NotifyURL string
	// ReparseTemplate specifies whether the template should be fetched again
	// before the assembly is replayed. This can be used if the template has
	// changed since the original assembly was created.
	ReparseTemplate bool

	assemblyURL string
	steps       map[string]map[string]interface{}
}

// AssemblyList contains a list of assemblies.
type AssemblyList struct {
	Assemblies []*AssemblyListItem `json:"items"`
	Count      int                 `json:"count"`
}

// AssemblyListItem contains reduced details about an assembly.
type AssemblyListItem struct {
	Ok    string `json:"ok"`
	Error string `json:"error"`

	AssemblyID        string     `json:"id"`
	AccountID         string     `json:"account_id"`
	TemplateID        string     `json:"template_id"`
	Instance          string     `json:"instance"`
	NotifyURL         string     `json:"notify_url"`
	RedirectURL       string     `json:"redirect_url"`
	ExecutionDuration float32    `json:"execution_duration"`
	ExecutionStart    *time.Time `json:"execution_start"`
	Created           time.Time  `json:"created"`
	Files             string     `json:"files"`
}

// AssemblyInfo contains details about an assemblies current status. Details
// about each value can be found at https://transloadit.com/docs/api-docs/#assembly-status-response
type AssemblyInfo struct {
	Ok      string `json:"ok"`
	Error   string `json:"error"`
	Message string `json:"message"`

	AssemblyID             string                 `json:"assembly_id"`
	ParentID               string                 `json:"parent_id"`
	AssemblyURL            string                 `json:"assembly_url"`
	AssemblySSLURL         string                 `json:"assembly_ssl_url"`
	BytesReceived          int                    `json:"bytes_received"`
	BytesExpected          int                    `json:"bytes_expected"`
	StartDate              string                 `json:"start_date"`
	IsInfinite             bool                   `json:"is_infinite"`
	HasDupeJobs            bool                   `json:"has_dupe_jobs"`
	UploadDuration         float32                `json:"upload_duration"`
	NotifyURL              string                 `json:"notify_url"`
	NotifyStart            string                 `json:"notify_start"`
	NotifyStatus           string                 `json:"notify_status"`
	NotifyDuation          float32                `json:"notify_duration"`
	LastJobCompleted       string                 `json:"last_job_completed"`
	ExecutionDuration      float32                `json:"execution_duration"`
	ExecutionStart         string                 `json:"execution_start"`
	Created                string                 `json:"created"`
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

	// Since 7 March 2018, the user agent, IP and referer are no longer
	// stored by Transloadit (see https://transloadit.com/blog/2018/03/gdpr/)
	// Therefore, these properties will always hold empty strings.
	ClientAgent   string
	ClientIp      string
	ClientReferer string
}

// FileInfo contains details about a file which was either uploaded or is the
// result of an executed assembly.
type FileInfo struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Basename         string                 `json:"basename"`
	Ext              string                 `json:"ext"`
	Size             int                    `json:"size"`
	Mime             string                 `json:"mime"`
	Type             string                 `json:"type"`
	Field            string                 `json:"field"`
	Md5Hash          string                 `json:"md5hash"`
	OriginalMd5Hash  string                 `json:"original_md5hash"`
	OriginalID       string                 `json:"original_id"`
	OriginalBasename string                 `json:"original_basename"`
	URL              string                 `json:"url"`
	SSLURL           string                 `json:"ssl_url"`
	Meta             map[string]interface{} `json:"meta"`
}

// NewAssembly will create a new Assembly struct which can be used to start
// an assembly using Client.StartAssembly.
func NewAssembly() Assembly {
	return Assembly{
		steps:   make(map[string]map[string]interface{}),
		readers: make([]*upload, 0),
	}
}

// AddReader will add the provided io.Reader to the list which will be uploaded
// once Client.StartAssembly is invoked. The corresponding field name can be
// used to reference the file in the assembly instructions.
func (assembly *Assembly) AddReader(fieldname, filename string, reader io.ReadCloser) {
	assembly.readers = append(assembly.readers, &upload{
		Field:  fieldname,
		Name:   filename,
		Reader: reader,
	})
}

// AddFile will open the provided file path and add it to the list which will be
// uploaded once Client.StartAssembly is invoked. The corresponding field name
// can be used to reference the file in the assembly instructions.
func (assembly *Assembly) AddFile(fieldname, filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}

	assembly.AddReader(fieldname, filepath, file)
	return nil
}

// AddStep will add the provided step to the assembly instructions. Details
// about possible values can be found at https://transloadit.com/docs/#14-assembly-instructions
func (assembly *Assembly) AddStep(name string, details map[string]interface{}) {
	assembly.steps[name] = details
}

// StartAssembly will upload all provided files and instruct the endpoint to
// start executing it. The function will return after all uploads complete and
// the remote server received the instructions (or the provided context times
// out). It won't wait until the execution has finished and results are
// available, which can be achieved using WaitForAssembly.
//
// When an error is returned you should also check AssemblyInfo.Error for more
// information about the error sent by the Transloadit API:
//  info, err := assembly.Upload()
//  if err != nil {
//  	if info != nil && info.Error != "" {
//  		// See info.Error
//  	}
//  	panic(err)
//  }
func (client *Client) StartAssembly(ctx context.Context, assembly Assembly) (*AssemblyInfo, error) {
	req, err := assembly.makeRequest(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create assembly request: %s", err)
	}

	var info AssemblyInfo
	// TODO: add context.Context
	if err = client.doRequest(req, &info); err != nil {
		return nil, err
	}

	if info.Error != "" {
		return &info, fmt.Errorf("failed to create assembly: %s", info.Error)
	}

	return &info, err
}

func (assembly *Assembly) makeRequest(ctx context.Context, client *Client) (*http.Request, error) {
	// TODO: test with huge files
	url := client.config.Endpoint + "/assemblies"
	bodyReader, bodyWriter := io.Pipe()
	multiWriter := multipart.NewWriter(bodyWriter)

	options := make(map[string]interface{})

	if len(assembly.steps) != 0 {
		options["steps"] = assembly.steps
	}

	if assembly.TemplateID != "" {
		options["template_id"] = assembly.TemplateID
	}

	if assembly.NotifyURL != "" {
		options["notify_url"] = assembly.NotifyURL
	}

	params, signature, err := client.sign(options)
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

	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", multiWriter.FormDataContentType())

	return req, nil
}

// GetAssembly fetches the full assembly status from the provided URL.
// The assembly URL must be absolute, for example:
// https://api2-amberly.transloadit.com/assemblies/15a6b3701d3811e78d7bfba4db1b053e
func (client *Client) GetAssembly(ctx context.Context, assemblyURL string) (*AssemblyInfo, error) {
	var info AssemblyInfo
	err := client.request(ctx, "GET", assemblyURL, nil, &info)

	return &info, err
}

// CancelAssembly cancels an assembly which will result in all corresponding
// uploads and encoding jobs to be aborted. Finally, the updated assembly
// information after the cancellation will be returned.
// The assembly URL must be absolute, for example:
// https://api2-amberly.transloadit.com/assemblies/15a6b3701d3811e78d7bfba4db1b053e
func (client *Client) CancelAssembly(ctx context.Context, assemblyURL string) (*AssemblyInfo, error) {
	var info AssemblyInfo
	err := client.request(ctx, "DELETE", assemblyURL, nil, &info)

	return &info, err
}

// NewAssemblyReplay will create a new AssemblyReplay struct which can be used
// to replay an assemblie's execution using Client.StartAssemblyReplay.
// The assembly URL must be absolute, for example:
// https://api2-amberly.transloadit.com/assemblies/15a6b3701d3811e78d7bfba4db1b053e
func NewAssemblyReplay(assemblyURL string) AssemblyReplay {
	return AssemblyReplay{
		steps:       make(map[string]map[string]interface{}),
		assemblyURL: assemblyURL,
	}
}

// AddStep will add the provided step to the new assembly instructions. When the
// assembly is replayed, those new steps will be used instead of the original
// ones. Details about possible values can be found at
// https://transloadit.com/docs/#14-assembly-instructions.
func (assembly *AssemblyReplay) AddStep(name string, details map[string]interface{}) {
	assembly.steps[name] = details
}

// StartAssemblyReplay will instruct the endpoint to replay the entire assembly
// execution.
func (client *Client) StartAssemblyReplay(ctx context.Context, assembly AssemblyReplay) (*AssemblyInfo, error) {
	options := map[string]interface{}{
		"steps": assembly.steps,
	}

	if assembly.ReparseTemplate {
		options["reparse_template"] = 1
	}

	if assembly.NotifyURL != "" {
		options["notify_url"] = assembly.NotifyURL
	}

	var info AssemblyInfo
	err := client.request(ctx, "POST", assembly.assemblyURL+"/replay", options, &info)
	if err != nil {
		return nil, err
	}

	if info.Error != "" {
		return &info, fmt.Errorf("failed to start assembly replay: %s", info.Error)
	}

	return &info, nil
}

// ListAssemblies will fetch all assemblies matching the provided criteria.
func (client *Client) ListAssemblies(ctx context.Context, options *ListOptions) (AssemblyList, error) {
	var assemblies AssemblyList
	err := client.listRequest(ctx, "assemblies", options, &assemblies)

	return assemblies, err
}
