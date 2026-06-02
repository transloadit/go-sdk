package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
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

func featurePreparation(
	scenario map[string]interface{},
	featureID string,
) (map[string]interface{}, string, error) {
	preparations, err := arrayValue(scenario["preparations"], "preparations")
	if err != nil {
		return nil, "", err
	}

	for index, rawPreparation := range preparations {
		label := fmt.Sprintf("preparations[%d]", index)
		preparation, err := objectValue(rawPreparation, label)
		if err != nil {
			return nil, "", err
		}
		currentFeatureID, err := stringValue(preparation["featureId"], label+".featureId")
		if err != nil {
			return nil, "", err
		}
		if currentFeatureID != featureID {
			continue
		}
		kind, err := stringValue(preparation["kind"], label+".kind")
		if err != nil {
			return nil, "", err
		}
		if kind != "feature-call" {
			return nil, "", fmt.Errorf("%s must be a feature-call preparation", label)
		}

		return preparation, label, nil
	}

	return nil, "", fmt.Errorf("scenario has no preparation for feature %q", featureID)
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

func scenarioFileCount(scenario map[string]interface{}) (int, error) {
	createConfig, createConfigLabel, err := featurePreparation(scenario, "createTusAssembly")
	if err != nil {
		return 0, err
	}
	input, err := objectValue(createConfig["input"], createConfigLabel+".input")
	if err != nil {
		return 0, err
	}

	return intValue(input["file_count"], createConfigLabel+".input.file_count")
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

func uploadInfo(scenario map[string]interface{}) (string, string, map[string]string, error) {
	uploadConfig, err := objectValue(scenario["upload"], "upload")
	if err != nil {
		return "", "", nil, err
	}
	chunkSize, err := stringValue(uploadConfig["chunkSize"], "upload.chunkSize")
	if err != nil {
		return "", "", nil, err
	}
	if chunkSize != "full-file" {
		return "", "", nil, fmt.Errorf("unsupported chunk size policy %q", chunkSize)
	}
	fieldName, err := stringValue(uploadConfig["fieldName"], "upload.fieldName")
	if err != nil {
		return "", "", nil, err
	}
	fileName, err := stringValue(uploadConfig["fileName"], "upload.fileName")
	if err != nil {
		return "", "", nil, err
	}

	userMeta := map[string]string{}
	if rawUserMeta, ok := uploadConfig["userMeta"]; ok {
		userMetaObject, err := objectValue(rawUserMeta, "upload.userMeta")
		if err != nil {
			return "", "", nil, err
		}
		for name, value := range userMetaObject {
			userMeta[name], err = stringValue(value, "upload.userMeta."+name)
			if err != nil {
				return "", "", nil, err
			}
		}
	}

	return fieldName, fileName, userMeta, nil
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

	client := transloadit.NewClient(transloadit.Config{
		AuthKey:    requiredEnv("TRANSLOADIT_KEY"),
		AuthSecret: requiredEnv("TRANSLOADIT_SECRET"),
		Endpoint:   requiredEnv("TRANSLOADIT_ENDPOINT"),
	})

	fileCount, err := scenarioFileCount(scenario)
	if err != nil {
		fail("read file count: %v", err)
	}
	content, err := scenarioBytes(scenario)
	if err != nil {
		fail("read upload bytes: %v", err)
	}
	fieldName, fileName, userMeta, err := uploadInfo(scenario)
	if err != nil {
		fail("read upload info: %v", err)
	}

	statusInfo, uploadURL, err := client.UploadTusAssembly(
		ctx,
		fileCount,
		content,
		fieldName,
		fileName,
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

	scenarioID, err := stringValue(scenario["scenarioId"], "scenarioId")
	if err != nil {
		fail("read scenario id: %v", err)
	}
	fmt.Printf("Go Transloadit SDK devdock scenario %s uploaded to %s\n", scenarioID, uploadURL)
}
