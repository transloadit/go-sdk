package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	contents, err := os.ReadFile(scenarioPath)
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

	templateList, err := client.ListTemplates(ctx, &transloadit.ListOptions{
		PageSize: scenario.List.PageSize,
	})
	if err != nil {
		fail("list templates: %v", err)
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

	if err := client.DeleteTemplate(ctx, templateID); err != nil {
		fail("delete template: %v", err)
	}
	deleteTemplate = false

	_, err = client.GetTemplate(ctx, templateID)
	deletedGetSucceeded := err == nil
	deletedErrorCode := ""
	var requestErr transloadit.RequestError
	if err != nil && !errors.As(err, &requestErr) {
		fail("get deleted template returned %T, expected transloadit.RequestError", err)
	}
	if err != nil {
		deletedErrorCode = requestErr.Code
	}

	result := map[string]interface{}{
		"deletedErrorCode":    deletedErrorCode,
		"deletedGetSucceeded": deletedGetSucceeded,
		"fetched":             templateResult(fetched),
		"listCount":           templateList.Count,
		"templateId":          templateID,
		"templateName":        templateName,
		"updated":             templateResult(fetchedUpdated),
		"updatedTemplateName": updatedTemplate.Name,
	}
	if err := writeResult(result); err != nil {
		fail("write result: %v", err)
	}

	fmt.Printf(
		"Go Transloadit SDK devdock scenario %s passed for %s\n",
		scenario.ScenarioID,
		requiredEnv("TRANSLOADIT_ENDPOINT"),
	)
}

func templateResult(template transloadit.Template) map[string]interface{} {
	content := map[string]interface{}{
		"steps": template.Content.Steps,
	}
	for name, value := range template.Content.AdditionalProperties {
		content[name] = value
	}

	return map[string]interface{}{
		"content":              content,
		"id":                   template.ID,
		"name":                 template.Name,
		"requireSignatureAuth": template.RequireSignatureAuth,
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

	return os.WriteFile(resultPath, append(contents, '\n'), 0o644)
}
