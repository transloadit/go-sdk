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

type assemblyLifecycleScenario struct {
	Assembly struct {
		FileCount int `json:"fileCount"`
	} `json:"assembly"`
	List struct {
		PageSize int `json:"pageSize"`
	} `json:"list"`
	ScenarioID string `json:"scenarioId"`
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

func loadScenario() (assemblyLifecycleScenario, error) {
	scenarioPath := os.Getenv("API2_SDK_EXAMPLE_SCENARIO")
	if scenarioPath == "" {
		scenarioPath = filepath.Join("examples", "api2-devdock-assembly-lifecycle", "api2-scenario.json")
	}

	contents, err := ioutil.ReadFile(scenarioPath)
	if err != nil {
		return assemblyLifecycleScenario{}, err
	}

	var scenario assemblyLifecycleScenario
	if err := json.Unmarshal(contents, &scenario); err != nil {
		return assemblyLifecycleScenario{}, err
	}

	return scenario, nil
}

func assemblyResult(info *transloadit.AssemblyInfo) map[string]interface{} {
	return map[string]interface{}{
		"assemblyId":     info.AssemblyID,
		"assemblySslUrl": info.AssemblySSLURL,
		"assemblyUrl":    info.AssemblyURL,
		"ok":             info.Ok,
	}
}

func writeResult(result map[string]interface{}) error {
	resultPath := os.Getenv("API2_SDK_EXAMPLE_RESULT")
	if resultPath == "" {
		return nil
	}

	contents, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(resultPath, append(contents, '\n'), 0o644)
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	created, err := client.CreateTusAssembly(ctx, scenario.Assembly.FileCount)
	if err != nil {
		fail("create TUS assembly: %v", err)
	}

	cancelOnExit := true
	defer func() {
		if cancelOnExit {
			_, _ = client.CancelAssembly(context.Background(), created.AssemblySSLURL)
		}
	}()

	fetched, err := client.GetAssembly(ctx, created.AssemblySSLURL)
	if err != nil {
		fail("get assembly: %v", err)
	}

	// The Assembly list is eventually consistent: the API acknowledges creation before the
	// list storage row lands, so poll briefly until the created Assembly shows up.
	var assemblies transloadit.AssemblyList
	listContainsCreated := false
	for attempt := 0; attempt < 20; attempt++ {
		assemblies, err = client.ListAssemblies(ctx, &transloadit.ListOptions{
			AssemblyID: created.AssemblyID,
			PageSize:   scenario.List.PageSize,
		})
		if err != nil {
			fail("list assemblies: %v", err)
		}

		for _, assembly := range assemblies.Assemblies {
			if assembly.AssemblyID == created.AssemblyID {
				listContainsCreated = true
			}
		}
		if listContainsCreated {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	cancelled, err := client.CancelAssembly(ctx, created.AssemblySSLURL)
	if err != nil {
		fail("cancel assembly: %v", err)
	}
	cancelOnExit = false

	if err := writeResult(map[string]interface{}{
		"cancelled":           assemblyResult(cancelled),
		"created":             assemblyResult(created),
		"fetched":             assemblyResult(fetched),
		"listContainsCreated": listContainsCreated,
		"listCount":           assemblies.Count,
	}); err != nil {
		fail("write result: %v", err)
	}

	fmt.Printf(
		"Go Transloadit SDK devdock scenario %s canceled Assembly %s\n",
		scenario.ScenarioID,
		created.AssemblyID,
	)
}
