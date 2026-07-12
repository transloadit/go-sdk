// Run the API2 contract TUS resume scenario against a devdock API2 server.
//
// This example is intentionally checked into the SDK repository: it reads the
// API/TUS facts from API2's injected scenario JSON, interrupts an upload like
// an unlucky user would, and resumes it through the public SDK method.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	transloadit "github.com/transloadit/go-sdk"
)

type resumeUploadScenario struct {
	ExampleInput struct {
		ScenarioID string `json:"scenarioId"`
	} `json:"exampleInput"`
	Prepared struct {
		CreateResponse map[string]interface{} `json:"createResponse"`
	} `json:"prepared"`
	Upload struct {
		Metadata []metadataField `json:"metadata"`
		Resume   resumePlan      `json:"resume"`
		Source   uploadSource    `json:"source"`
		TusURL   valueSpec       `json:"tusUrl"`
	} `json:"upload"`
}

type metadataField struct {
	Name  string    `json:"name"`
	Value valueSpec `json:"value"`
}

type resumePlan struct {
	Fingerprint                string `json:"fingerprint"`
	RemoveFingerprintOnSuccess bool   `json:"removeFingerprintOnSuccess"`
	StopAfterAcceptedBytes     int    `json:"stopAfterAcceptedBytes"`
}

type uploadSource struct {
	Encoding string `json:"encoding"`
	Kind     string `json:"kind"`
	Value    string `json:"value"`
}

type valueSpec struct {
	Source *valueSpecSource `json:"source"`
	Value  interface{}      `json:"value"`
}

type valueSpecSource struct {
	Path []string `json:"path"`
	Root string   `json:"root"`
}

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		panic(fmt.Sprintf("%s must be set", name))
	}

	return value
}

func fail(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}

func loadScenario() (resumeUploadScenario, map[string]interface{}, error) {
	scenarioPath := os.Getenv("API2_SDK_EXAMPLE_SCENARIO")
	if scenarioPath == "" {
		scenarioPath = filepath.Join("examples", "api2-devdock-tus-resume-upload", "api2-scenario.json")
	}

	contents, err := ioutil.ReadFile(scenarioPath)
	if err != nil {
		return resumeUploadScenario{}, nil, err
	}

	var scenario resumeUploadScenario
	if err := json.Unmarshal(contents, &scenario); err != nil {
		return resumeUploadScenario{}, nil, err
	}

	var rawScenario map[string]interface{}
	if err := json.Unmarshal(contents, &rawScenario); err != nil {
		return resumeUploadScenario{}, nil, err
	}

	return scenario, rawScenario, nil
}

func resolveValue(spec valueSpec, context map[string]interface{}, label string) interface{} {
	if spec.Source == nil {
		return spec.Value
	}

	current, ok := context[spec.Source.Root]
	if !ok {
		fail("%s value source root is unavailable", label)
	}
	for _, part := range spec.Source.Path {
		record, ok := current.(map[string]interface{})
		if !ok {
			fail("%s value source cannot read %s", label, part)
		}
		current, ok = record[part]
		if !ok {
			fail("%s value source cannot read %s", label, part)
		}
	}

	return current
}

func resolveString(spec valueSpec, context map[string]interface{}, label string) string {
	value, ok := resolveValue(spec, context, label).(string)
	if !ok {
		fail("%s must be a string", label)
	}

	return value
}

func scenarioBytes(source uploadSource) []byte {
	if source.Kind != "bytes" {
		fail("upload.source.kind must be bytes")
	}
	if source.Encoding != "utf8" {
		fail("upload.source.encoding must be utf8")
	}

	return []byte(source.Value)
}

func uploadMetadata(fields []metadataField, context map[string]interface{}) map[string]string {
	metadata := make(map[string]string, len(fields))
	for _, field := range fields {
		metadata[field.Name] = fmt.Sprintf("%v", resolveValue(field.Value, context, field.Name))
	}

	return metadata
}

// createInterruptedUpload creates a TUS upload and only sends the first chunk,
// leaving the upload interrupted the way a dropped connection would.
func createInterruptedUpload(
	ctx context.Context,
	tusURL string,
	content []byte,
	metadata map[string]string,
	stopAfterAcceptedBytes int,
) string {
	metadataNames := make([]string, 0, len(metadata))
	for name := range metadata {
		metadataNames = append(metadataNames, name)
	}
	sort.Strings(metadataNames)
	metadataParts := make([]string, 0, len(metadata))
	for _, name := range metadataNames {
		encodedValue := base64.StdEncoding.EncodeToString([]byte(metadata[name]))
		metadataParts = append(metadataParts, fmt.Sprintf("%s %s", name, encodedValue))
	}

	createRequest, err := http.NewRequestWithContext(ctx, "POST", tusURL, nil)
	if err != nil {
		fail("TUS create request: %v", err)
	}
	createRequest.Header.Set("Tus-Resumable", "1.0.0")
	createRequest.Header.Set("Upload-Length", strconv.Itoa(len(content)))
	createRequest.Header.Set("Upload-Metadata", strings.Join(metadataParts, ","))
	createResponse, err := http.DefaultClient.Do(createRequest)
	if err != nil {
		fail("TUS create request failed: %v", err)
	}
	defer createResponse.Body.Close()
	if createResponse.StatusCode != 201 {
		fail("TUS create returned HTTP %d, expected 201", createResponse.StatusCode)
	}
	location := createResponse.Header.Get("Location")
	if location == "" {
		fail("TUS create did not return a Location header")
	}
	tusBase, err := url.Parse(tusURL)
	if err != nil {
		fail("parse TUS URL: %v", err)
	}
	uploadURL, err := tusBase.Parse(location)
	if err != nil {
		fail("resolve upload URL: %v", err)
	}
	uploadURLText := uploadURL.String()

	patchRequest, err := http.NewRequestWithContext(
		ctx,
		"PATCH",
		uploadURLText,
		bytes.NewReader(content[:stopAfterAcceptedBytes]),
	)
	if err != nil {
		fail("TUS first chunk request: %v", err)
	}
	patchRequest.Header.Set("Tus-Resumable", "1.0.0")
	patchRequest.Header.Set("Upload-Offset", "0")
	patchRequest.Header.Set("Content-Type", "application/offset+octet-stream")
	patchResponse, err := http.DefaultClient.Do(patchRequest)
	if err != nil {
		fail("TUS first chunk request failed: %v", err)
	}
	defer patchResponse.Body.Close()
	if patchResponse.StatusCode != 204 {
		fail("TUS first chunk returned HTTP %d, expected 204", patchResponse.StatusCode)
	}
	acceptedBytes, err := strconv.Atoi(patchResponse.Header.Get("Upload-Offset"))
	if err != nil || acceptedBytes != stopAfterAcceptedBytes {
		fail("TUS first chunk accepted %d bytes, expected %d", acceptedBytes, stopAfterAcceptedBytes)
	}

	return uploadURLText
}

func writeResult(
	firstUploadURL string,
	previousUploadCount int,
	remainingPreviousUploadCount int,
) error {
	resultPath := os.Getenv("API2_SDK_EXAMPLE_RESULT")
	if resultPath == "" {
		return nil
	}

	contents, err := json.MarshalIndent(
		map[string]interface{}{
			"firstUploadUrl":               firstUploadURL,
			"previousUploadCount":          previousUploadCount,
			"remainingPreviousUploadCount": remainingPreviousUploadCount,
			"uploadUrl":                    firstUploadURL,
		},
		"",
		"  ",
	)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(resultPath, append(contents, '\n'), 0o644)
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	scenario, rawScenario, err := loadScenario()
	if err != nil {
		fail("load scenario: %v", err)
	}
	resume := scenario.Upload.Resume

	client := transloadit.NewClient(transloadit.Config{
		AuthKey:    requiredEnv("TRANSLOADIT_KEY"),
		AuthSecret: requiredEnv("TRANSLOADIT_SECRET"),
		Endpoint:   requiredEnv("TRANSLOADIT_ENDPOINT"),
	})

	valueContext := map[string]interface{}{
		"createResponse": scenario.Prepared.CreateResponse,
		"scenario":       rawScenario,
	}
	content := scenarioBytes(scenario.Upload.Source)
	tusURL := resolveString(scenario.Upload.TusURL, valueContext, "upload.tusUrl")
	metadata := uploadMetadata(scenario.Upload.Metadata, valueContext)

	firstUploadURL := createInterruptedUpload(
		ctx,
		tusURL,
		content,
		metadata,
		resume.StopAfterAcceptedBytes,
	)

	// Remember the interrupted upload by fingerprint, like a TUS client URL storage would.
	storedUploads := map[string]string{resume.Fingerprint: firstUploadURL}
	previousUploadCount := len(storedUploads)

	assemblySSLURL, ok := scenario.Prepared.CreateResponse["assembly_ssl_url"].(string)
	if !ok || assemblySSLURL == "" {
		fail("prepared.createResponse.assembly_ssl_url must be a string")
	}
	completedAssembly, err := client.ResumeTusUpload(
		ctx,
		storedUploads[resume.Fingerprint],
		content,
		&transloadit.AssemblyInfo{AssemblySSLURL: assemblySSLURL},
	)
	if err != nil {
		fail("resume TUS upload: %v", err)
	}
	if completedAssembly.Error != "" {
		fail("resumeTusUpload returned %s: %s", completedAssembly.Error, completedAssembly.Message)
	}

	if resume.RemoveFingerprintOnSuccess {
		delete(storedUploads, resume.Fingerprint)
	}
	remainingPreviousUploadCount := len(storedUploads)

	if err := writeResult(firstUploadURL, previousUploadCount, remainingPreviousUploadCount); err != nil {
		fail("write result: %v", err)
	}

	fmt.Printf(
		"Go Transloadit SDK devdock scenario %s resumed %s\n",
		scenario.ExampleInput.ScenarioID,
		firstUploadURL,
	)
}
