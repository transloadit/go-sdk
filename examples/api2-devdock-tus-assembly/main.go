package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	transloadit "github.com/transloadit/go-sdk"
)

type tusAssemblyScenario struct {
	ExampleInput struct {
		ScenarioID       string `json:"scenarioId"`
		SdkFeatureInputs struct {
			UploadTusAssembly uploadTusAssemblyInput `json:"uploadTusAssembly"`
		} `json:"sdkFeatureInputs"`
	} `json:"exampleInput"`
}

type uploadTusAssemblyInput struct {
	FileCount int          `json:"file_count"`
	Upload    uploadConfig `json:"upload"`
}

type uploadConfig struct {
	Content  string            `json:"content"`
	Field    string            `json:"fieldname"`
	Filename string            `json:"filename"`
	UserMeta map[string]string `json:"user_meta"`
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

func loadScenario() (tusAssemblyScenario, error) {
	scenarioPath := os.Getenv("API2_SDK_EXAMPLE_SCENARIO")
	if scenarioPath == "" {
		scenarioPath = filepath.Join("examples", "api2-devdock-tus-assembly", "api2-scenario.json")
	}

	contents, err := ioutil.ReadFile(scenarioPath)
	if err != nil {
		return tusAssemblyScenario{}, err
	}

	var scenario tusAssemblyScenario
	if err := json.Unmarshal(contents, &scenario); err != nil {
		return tusAssemblyScenario{}, err
	}

	return scenario, nil
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

func writeResult(
	status map[string]interface{},
	uploadURL string,
) error {
	resultPath := os.Getenv("API2_SDK_EXAMPLE_RESULT")
	if resultPath == "" {
		return nil
	}

	contents, err := json.MarshalIndent(
		map[string]interface{}{
			"createResponse": status,
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
	input := scenario.ExampleInput.SdkFeatureInputs.UploadTusAssembly

	client := transloadit.NewClient(transloadit.Config{
		AuthKey:    requiredEnv("TRANSLOADIT_KEY"),
		AuthSecret: requiredEnv("TRANSLOADIT_SECRET"),
		Endpoint:   requiredEnv("TRANSLOADIT_ENDPOINT"),
	})

	userMeta := input.Upload.UserMeta
	if userMeta == nil {
		userMeta = map[string]string{}
	}

	statusInfo, uploadURL, err := client.UploadTusAssembly(
		ctx,
		input.FileCount,
		[]byte(input.Upload.Content),
		input.Upload.Field,
		input.Upload.Filename,
		userMeta,
	)
	if err != nil {
		fail("upload TUS assembly: %v", err)
	}
	status, err := asJsonObject(statusInfo, "assembly status")
	if err != nil {
		fail("serialize assembly status: %v", err)
	}
	if err := writeResult(status, uploadURL); err != nil {
		fail("write result: %v", err)
	}

	fmt.Printf(
		"Go Transloadit SDK devdock scenario %s uploaded to %s\n",
		scenario.ExampleInput.ScenarioID,
		uploadURL,
	)
}
