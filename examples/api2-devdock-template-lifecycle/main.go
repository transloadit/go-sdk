package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	transloadit "github.com/transloadit/go-sdk"
)

type scenarioContent struct {
	AdditionalProperties map[string]interface{}            `json:"additionalProperties"`
	Steps                map[string]map[string]interface{} `json:"steps"`
}

type templateLifecycleScenario struct {
	Delete struct {
		ErrorCodeIncludes string `json:"errorCodeIncludes"`
	} `json:"delete"`
	List struct {
		MinimumCount int `json:"minimumCount"`
		PageSize     int `json:"pageSize"`
	} `json:"list"`
	ScenarioID string `json:"scenarioId"`
	Template   struct {
		Content              scenarioContent `json:"content"`
		NamePrefix           string          `json:"namePrefix"`
		RequireSignatureAuth bool            `json:"requireSignatureAuth"`
	} `json:"template"`
	Update struct {
		Content              scenarioContent `json:"content"`
		NameSuffix           string          `json:"nameSuffix"`
		RequireSignatureAuth bool            `json:"requireSignatureAuth"`
	} `json:"update"`
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

func loadScenario() (templateLifecycleScenario, error) {
	scenarioPath := os.Getenv("API2_SDK_EXAMPLE_SCENARIO")
	if scenarioPath == "" {
		scenarioPath = filepath.Join(
			"examples",
			"api2-devdock-template-lifecycle",
			"api2-scenario.json",
		)
	}

	contents, err := ioutil.ReadFile(scenarioPath)
	if err != nil {
		return templateLifecycleScenario{}, err
	}

	var scenario templateLifecycleScenario
	if err := json.Unmarshal(contents, &scenario); err != nil {
		return templateLifecycleScenario{}, err
	}

	return scenario, nil
}

func applyTemplateContent(template *transloadit.Template, content scenarioContent) {
	for stepName, step := range content.Steps {
		template.AddStep(stepName, step)
	}

	for name, value := range content.AdditionalProperties {
		template.Content.AdditionalProperties[name] = value
	}
}

func newTemplate(name string, requireSignatureAuth bool, content scenarioContent) transloadit.Template {
	template := transloadit.NewTemplate()
	template.Name = name
	template.RequireSignatureAuth = requireSignatureAuth
	applyTemplateContent(&template, content)

	return template
}

func assertTemplateContent(label string, template transloadit.Template, expected scenarioContent) {
	for stepName, expectedStep := range expected.Steps {
		actualStep, ok := template.Content.Steps[stepName]
		if !ok {
			fail("%s response did not include step %q", label, stepName)
		}
		if !reflect.DeepEqual(actualStep, expectedStep) {
			fail("%s response step %q was %v, expected %v", label, stepName, actualStep, expectedStep)
		}
	}

	for name, expectedValue := range expected.AdditionalProperties {
		actualValue, ok := template.Content.AdditionalProperties[name]
		if !ok {
			fail("%s response did not include content property %q", label, name)
		}
		if !reflect.DeepEqual(actualValue, expectedValue) {
			fail(
				"%s response content property %q was %v, expected %v",
				label,
				name,
				actualValue,
				expectedValue,
			)
		}
	}
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	templateName := fmt.Sprintf("%s-%d", scenario.Template.NamePrefix, time.Now().UnixNano())
	template := newTemplate(
		templateName,
		scenario.Template.RequireSignatureAuth,
		scenario.Template.Content,
	)

	templateID, err := client.CreateTemplate(ctx, template)
	if err != nil {
		fail("create template: %v", err)
	}
	if templateID == "" {
		fail("create template returned an empty id")
	}

	deleteTemplate := true
	defer func() {
		if deleteTemplate {
			_ = client.DeleteTemplate(context.Background(), templateID)
		}
	}()

	fetched, err := client.GetTemplate(ctx, templateID)
	if err != nil {
		fail("get template: %v", err)
	}
	if fetched.ID != templateID {
		fail("get template returned id %q, expected %q", fetched.ID, templateID)
	}
	if fetched.Name != templateName {
		fail("get template returned name %q, expected %q", fetched.Name, templateName)
	}
	if fetched.RequireSignatureAuth != scenario.Template.RequireSignatureAuth {
		fail(
			"get template returned RequireSignatureAuth=%v, expected %v",
			fetched.RequireSignatureAuth,
			scenario.Template.RequireSignatureAuth,
		)
	}
	assertTemplateContent("get template", fetched, scenario.Template.Content)

	templateList, err := client.ListTemplates(ctx, &transloadit.ListOptions{
		PageSize: scenario.List.PageSize,
	})
	if err != nil {
		fail("list templates: %v", err)
	}
	if templateList.Count < scenario.List.MinimumCount {
		fail(
			"list templates returned count=%d, expected at least %d",
			templateList.Count,
			scenario.List.MinimumCount,
		)
	}

	updatedTemplate := newTemplate(
		templateName+scenario.Update.NameSuffix,
		scenario.Update.RequireSignatureAuth,
		scenario.Update.Content,
	)

	if err := client.UpdateTemplate(ctx, templateID, updatedTemplate); err != nil {
		fail("update template: %v", err)
	}

	fetchedUpdated, err := client.GetTemplate(ctx, templateID)
	if err != nil {
		fail("get updated template: %v", err)
	}
	if fetchedUpdated.Name != updatedTemplate.Name {
		fail("updated template returned name %q, expected %q", fetchedUpdated.Name, updatedTemplate.Name)
	}
	if fetchedUpdated.RequireSignatureAuth != scenario.Update.RequireSignatureAuth {
		fail(
			"updated template returned RequireSignatureAuth=%v, expected %v",
			fetchedUpdated.RequireSignatureAuth,
			scenario.Update.RequireSignatureAuth,
		)
	}
	assertTemplateContent("updated template", fetchedUpdated, scenario.Update.Content)

	if err := client.DeleteTemplate(ctx, templateID); err != nil {
		fail("delete template: %v", err)
	}
	deleteTemplate = false

	_, err = client.GetTemplate(ctx, templateID)
	if err == nil {
		fail("get deleted template succeeded unexpectedly")
	}
	var requestErr transloadit.RequestError
	if !errors.As(err, &requestErr) {
		fail("get deleted template returned %T, expected transloadit.RequestError", err)
	}
	if !strings.Contains(requestErr.Code, scenario.Delete.ErrorCodeIncludes) {
		fail("get deleted template returned unexpected error code %q", requestErr.Code)
	}

	fmt.Printf(
		"Go Transloadit SDK devdock scenario %s passed for %s\n",
		scenario.ScenarioID,
		requiredEnv("TRANSLOADIT_ENDPOINT"),
	)
}
