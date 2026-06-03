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

func sdkFeatureCall(
	scenario map[string]interface{},
	featureID string,
) (map[string]interface{}, string, error) {
	featureCalls, err := arrayValue(scenario["sdkFeatureCalls"], "sdkFeatureCalls")
	if err != nil {
		return nil, "", err
	}

	for index, rawFeatureCall := range featureCalls {
		label := fmt.Sprintf("sdkFeatureCalls[%d]", index)
		featureCall, err := objectValue(rawFeatureCall, label)
		if err != nil {
			return nil, "", err
		}
		currentFeatureID, err := stringValue(featureCall["featureId"], label+".featureId")
		if err != nil {
			return nil, "", err
		}
		if currentFeatureID != featureID {
			continue
		}
		kind, err := stringValue(featureCall["kind"], label+".kind")
		if err != nil {
			return nil, "", err
		}
		if kind != "sdk-feature-call" {
			return nil, "", fmt.Errorf("%s must be an sdk-feature-call", label)
		}

		return featureCall, label, nil
	}

	return nil, "", fmt.Errorf("scenario has no SDK feature call for feature %q", featureID)
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

func uploadTusAssemblyInput(scenario map[string]interface{}) (map[string]interface{}, error) {
	featureCall, featureCallLabel, err := sdkFeatureCall(scenario, "uploadTusAssembly")
	if err != nil {
		return nil, err
	}
	input, err := objectValue(featureCall["input"], featureCallLabel+".input")
	if err != nil {
		return nil, err
	}

	return input, nil
}

func scenarioFileCount(input map[string]interface{}) (int, error) {
	return intValue(input["file_count"], "sdkFeatureCalls.uploadTusAssembly.input.file_count")
}

func scenarioBytes(uploadConfig map[string]interface{}) ([]byte, error) {
	content, err := stringValue(
		uploadConfig["content"],
		"sdkFeatureCalls.uploadTusAssembly.input.upload.content",
	)
	if err != nil {
		return nil, err
	}

	return []byte(content), nil
}

func uploadInfo(input map[string]interface{}) (string, string, map[string]string, error) {
	uploadConfig, err := objectValue(
		input["upload"],
		"sdkFeatureCalls.uploadTusAssembly.input.upload",
	)
	if err != nil {
		return "", "", nil, err
	}
	fieldName, err := stringValue(
		uploadConfig["fieldname"],
		"sdkFeatureCalls.uploadTusAssembly.input.upload.fieldname",
	)
	if err != nil {
		return "", "", nil, err
	}
	fileName, err := stringValue(
		uploadConfig["filename"],
		"sdkFeatureCalls.uploadTusAssembly.input.upload.filename",
	)
	if err != nil {
		return "", "", nil, err
	}

	userMeta := map[string]string{}
	if rawUserMeta, ok := uploadConfig["user_meta"]; ok {
		userMetaObject, err := objectValue(
			rawUserMeta,
			"sdkFeatureCalls.uploadTusAssembly.input.upload.user_meta",
		)
		if err != nil {
			return "", "", nil, err
		}
		for name, value := range userMetaObject {
			userMeta[name], err = stringValue(
				value,
				"sdkFeatureCalls.uploadTusAssembly.input.upload.user_meta."+name,
			)
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
	input, err := uploadTusAssemblyInput(scenario)
	if err != nil {
		fail("read SDK feature call input: %v", err)
	}

	client := transloadit.NewClient(transloadit.Config{
		AuthKey:    requiredEnv("TRANSLOADIT_KEY"),
		AuthSecret: requiredEnv("TRANSLOADIT_SECRET"),
		Endpoint:   requiredEnv("TRANSLOADIT_ENDPOINT"),
	})

	fileCount, err := scenarioFileCount(input)
	if err != nil {
		fail("read file count: %v", err)
	}
	fieldName, fileName, userMeta, err := uploadInfo(input)
	if err != nil {
		fail("read upload info: %v", err)
	}
	uploadConfig, err := objectValue(
		input["upload"],
		"sdkFeatureCalls.uploadTusAssembly.input.upload",
	)
	if err != nil {
		fail("read upload config: %v", err)
	}
	content, err := scenarioBytes(uploadConfig)
	if err != nil {
		fail("read upload bytes: %v", err)
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
