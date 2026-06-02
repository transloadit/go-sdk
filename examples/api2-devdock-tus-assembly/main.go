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
	"strconv"
	"strings"
	"time"

	transloadit "github.com/transloadit/go-sdk"
)

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

func loadScenario() (map[string]interface{}, error) {
	scenarioPath := os.Getenv("API2_SDK_EXAMPLE_SCENARIO")
	if scenarioPath == "" {
		scenarioPath = filepath.Join("examples", "api2-devdock-tus-assembly", "api2-scenario.json")
	}

	contents, err := ioutil.ReadFile(scenarioPath)
	if err != nil {
		return nil, err
	}

	var scenario map[string]interface{}
	if err := json.Unmarshal(contents, &scenario); err != nil {
		return nil, err
	}

	return scenario, nil
}

func objectValue(value interface{}, label string) (map[string]interface{}, error) {
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be an object", label)
	}

	return object, nil
}

func arrayValue(value interface{}, label string) ([]interface{}, error) {
	array, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be an array", label)
	}

	return array, nil
}

func stringValue(value interface{}, label string) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", label)
	}

	return text, nil
}

func intValue(value interface{}, label string) (int, error) {
	switch number := value.(type) {
	case float64:
		if float64(int(number)) != number {
			return 0, fmt.Errorf("%s must be an integer", label)
		}

		return int(number), nil
	case int:
		return number, nil
	case string:
		parsed, err := strconv.Atoi(number)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", label)
		}

		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be an integer", label)
	}
}

func scalarString(value interface{}) string {
	switch typed := value.(type) {
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case string:
		return typed
	default:
		serialized, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}

		return string(serialized)
	}
}

func readPath(value interface{}, pathParts []interface{}, label string) (interface{}, error) {
	current := value
	for _, part := range pathParts {
		if object, ok := current.(map[string]interface{}); ok {
			key, ok := part.(string)
			if !ok {
				return nil, fmt.Errorf("%s path cannot read non-string key %v from object", label, part)
			}
			next, ok := object[key]
			if !ok {
				return nil, fmt.Errorf("%s path is missing key %q", label, key)
			}
			current = next
			continue
		}

		if array, ok := current.([]interface{}); ok {
			index, err := intValue(part, label)
			if err != nil {
				return nil, err
			}
			if index < 0 || index >= len(array) {
				return nil, fmt.Errorf("%s path index %d is out of range", label, index)
			}
			current = array[index]
			continue
		}

		return nil, fmt.Errorf("%s path cannot read %v from %v", label, part, current)
	}

	return current, nil
}

func resolveValue(
	valueSpec interface{},
	context map[string]interface{},
	label string,
) (interface{}, error) {
	spec, err := objectValue(valueSpec, label)
	if err != nil {
		return nil, err
	}
	if literal, ok := spec["value"]; ok {
		return literal, nil
	}

	source, err := objectValue(spec["source"], label+".source")
	if err != nil {
		return nil, err
	}
	root, err := stringValue(source["root"], label+".source.root")
	if err != nil {
		return nil, err
	}
	rootValue, ok := context[root]
	if !ok {
		return nil, fmt.Errorf("%s source root %q is unavailable", label, root)
	}
	pathParts, err := arrayValue(source["path"], label+".source.path")
	if err != nil {
		return nil, err
	}

	return readPath(rootValue, pathParts, label)
}

func asJsonObject(value interface{}, label string) (map[string]interface{}, error) {
	contents, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(contents, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func createAssembly(
	ctx context.Context,
	client transloadit.Client,
	scenario map[string]interface{},
) (*transloadit.AssemblyInfo, map[string]interface{}, error) {
	createConfig, err := objectValue(scenario["createTusAssembly"], "createTusAssembly")
	if err != nil {
		return nil, nil, err
	}
	input, err := objectValue(createConfig["input"], "createTusAssembly.input")
	if err != nil {
		return nil, nil, err
	}
	fileCount, err := intValue(input["file_count"], "createTusAssembly.input.file_count")
	if err != nil {
		return nil, nil, err
	}

	info, err := client.CreateTusAssembly(ctx, fileCount)
	if err != nil {
		return nil, nil, err
	}
	createResponse, err := asJsonObject(info, "create response")
	if err != nil {
		return nil, nil, err
	}

	requiredPaths, err := arrayValue(
		createConfig["requiredResponsePaths"],
		"createTusAssembly.requiredResponsePaths",
	)
	if err != nil {
		return nil, nil, err
	}
	for index, rawPath := range requiredPaths {
		pathParts, err := arrayValue(
			rawPath,
			fmt.Sprintf("createTusAssembly.requiredResponsePaths[%d]", index),
		)
		if err != nil {
			return nil, nil, err
		}
		value, err := readPath(
			createResponse,
			pathParts,
			fmt.Sprintf("createTusAssembly.requiredResponsePaths[%d]", index),
		)
		if err != nil {
			return nil, nil, err
		}
		if scalarString(value) == "" {
			return nil, nil, fmt.Errorf("create response path %v is empty", pathParts)
		}
	}

	return info, createResponse, nil
}

func scenarioBytes(scenario map[string]interface{}) ([]byte, error) {
	upload, err := objectValue(scenario["upload"], "upload")
	if err != nil {
		return nil, err
	}
	source, err := objectValue(upload["source"], "upload.source")
	if err != nil {
		return nil, err
	}
	kind, err := stringValue(source["kind"], "upload.source.kind")
	if err != nil {
		return nil, err
	}
	if kind != "bytes" {
		return nil, fmt.Errorf("unsupported scenario source kind %q", kind)
	}
	encoding, err := stringValue(source["encoding"], "upload.source.encoding")
	if err != nil {
		return nil, err
	}
	if encoding != "utf8" {
		return nil, fmt.Errorf("unsupported scenario source encoding %q", encoding)
	}
	value, err := stringValue(source["value"], "upload.source.value")
	if err != nil {
		return nil, err
	}

	return []byte(value), nil
}

func uploadMetadata(
	scenario map[string]interface{},
	createResponse map[string]interface{},
) (map[string]string, error) {
	upload, err := objectValue(scenario["upload"], "upload")
	if err != nil {
		return nil, err
	}
	fields, err := arrayValue(upload["metadata"], "upload.metadata")
	if err != nil {
		return nil, err
	}

	context := map[string]interface{}{
		"createResponse": createResponse,
		"scenario":       scenario,
	}
	metadata := map[string]string{}
	for index, rawField := range fields {
		label := fmt.Sprintf("upload.metadata[%d]", index)
		field, err := objectValue(rawField, label)
		if err != nil {
			return nil, err
		}
		name, err := stringValue(field["name"], label+".name")
		if err != nil {
			return nil, err
		}
		value, err := resolveValue(field["value"], context, label+".value")
		if err != nil {
			return nil, err
		}
		metadata[name] = scalarString(value)
	}

	return metadata, nil
}

func tusMetadataHeader(metadata map[string]string) string {
	parts := make([]string, 0, len(metadata))
	for name, value := range metadata {
		encoded := base64.StdEncoding.EncodeToString([]byte(value))
		parts = append(parts, fmt.Sprintf("%s %s", name, encoded))
	}

	return strings.Join(parts, ",")
}

func checkedResponse(response *http.Response, expectedStatus int, label string) error {
	defer response.Body.Close()
	if response.StatusCode == expectedStatus {
		return nil
	}

	body, _ := ioutil.ReadAll(response.Body)
	return fmt.Errorf("%s returned HTTP %d: %s", label, response.StatusCode, string(body))
}

func uploadWithTus(
	ctx context.Context,
	scenario map[string]interface{},
	createResponse map[string]interface{},
) (string, error) {
	uploadConfig, err := objectValue(scenario["upload"], "upload")
	if err != nil {
		return "", err
	}
	context := map[string]interface{}{
		"createResponse": createResponse,
		"scenario":       scenario,
	}
	endpointValue, err := resolveValue(uploadConfig["tusUrl"], context, "upload.tusUrl")
	if err != nil {
		return "", err
	}
	endpointURL, err := url.Parse(scalarString(endpointValue))
	if err != nil {
		return "", err
	}
	content, err := scenarioBytes(scenario)
	if err != nil {
		return "", err
	}
	chunkSize, err := stringValue(uploadConfig["chunkSize"], "upload.chunkSize")
	if err != nil {
		return "", err
	}
	if chunkSize != "full-file" {
		return "", fmt.Errorf("unsupported chunk size policy %q", chunkSize)
	}
	metadata, err := uploadMetadata(scenario, createResponse)
	if err != nil {
		return "", err
	}

	createRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL.String(), nil)
	if err != nil {
		return "", err
	}
	createRequest.Header.Set("Tus-Resumable", "1.0.0")
	createRequest.Header.Set("Upload-Length", strconv.Itoa(len(content)))
	createRequest.Header.Set("Upload-Metadata", tusMetadataHeader(metadata))

	createResponseHttp, err := http.DefaultClient.Do(createRequest)
	if err != nil {
		return "", err
	}
	if err := checkedResponse(createResponseHttp, http.StatusCreated, "TUS create"); err != nil {
		return "", err
	}
	location := createResponseHttp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("TUS create did not return a Location header")
	}
	uploadURL, err := endpointURL.Parse(location)
	if err != nil {
		return "", err
	}

	patchRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPatch,
		uploadURL.String(),
		bytes.NewReader(content),
	)
	if err != nil {
		return "", err
	}
	patchRequest.Header.Set("Tus-Resumable", "1.0.0")
	patchRequest.Header.Set("Upload-Offset", "0")
	patchRequest.Header.Set("Content-Type", "application/offset+octet-stream")

	patchResponse, err := http.DefaultClient.Do(patchRequest)
	if err != nil {
		return "", err
	}
	if err := checkedResponse(patchResponse, http.StatusNoContent, "TUS upload"); err != nil {
		return "", err
	}
	remoteOffset, err := intValue(patchResponse.Header.Get("Upload-Offset"), "Upload-Offset")
	if err != nil {
		return "", err
	}
	if remoteOffset != len(content) {
		return "", fmt.Errorf("TUS upload offset %d, expected %d", remoteOffset, len(content))
	}

	return uploadURL.String(), nil
}

func writeResult(
	createResponse map[string]interface{},
	status map[string]interface{},
	uploadURL string,
) error {
	resultPath := os.Getenv("API2_SDK_EXAMPLE_RESULT")
	if resultPath == "" {
		return nil
	}

	contents, err := json.MarshalIndent(
		map[string]interface{}{
			"createResponse": createResponse,
			"status":         status,
			"uploadUrl":      uploadURL,
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

	scenario, err := loadScenario()
	if err != nil {
		fail("load scenario: %v", err)
	}

	client := transloadit.NewClient(transloadit.Config{
		AuthKey:    requiredEnv("TRANSLOADIT_KEY"),
		AuthSecret: requiredEnv("TRANSLOADIT_SECRET"),
		Endpoint:   requiredEnv("TRANSLOADIT_ENDPOINT"),
	})

	info, createResponse, err := createAssembly(ctx, client, scenario)
	if err != nil {
		fail("create TUS assembly: %v", err)
	}
	uploadURL, err := uploadWithTus(ctx, scenario, createResponse)
	if err != nil {
		fail("upload: %v", err)
	}
	statusInfo, err := client.WaitForAssembly(ctx, info)
	if err != nil {
		fail("wait for assembly: %v", err)
	}
	status, err := asJsonObject(statusInfo, "assembly status")
	if err != nil {
		fail("serialize assembly status: %v", err)
	}
	if err := writeResult(createResponse, status, uploadURL); err != nil {
		fail("write result: %v", err)
	}

	scenarioID, err := stringValue(scenario["scenarioId"], "scenarioId")
	if err != nil {
		fail("read scenario id: %v", err)
	}
	fmt.Printf("Go Transloadit SDK devdock scenario %s uploaded to %s\n", scenarioID, uploadURL)
}
