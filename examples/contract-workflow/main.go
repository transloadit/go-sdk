// Run with TRANSLOADIT_KEY and TRANSLOADIT_SECRET set in a trusted server-side shell:
// go run ./examples/contract-workflow ./image.jpg
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/transloadit/go-sdk/contract"
)

func assemblyFailed(status *contract.CreateAssemblyResult) bool {
	return status.WithError != nil || status.GetOk() == "ASSEMBLY_CANCELED" || status.GetOk() == "REQUEST_ABORTED"
}

func run() (err error) {
	if len(os.Args) != 2 {
		return errors.New("pass the path to an image")
	}
	client, err := contract.NewClient(contract.Config{
		AuthKey: os.Getenv("TRANSLOADIT_KEY"), AuthSecret: os.Getenv("TRANSLOADIT_SECRET"),
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	width, original, yes := contract.Integer(120), ":original", true
	step := contract.CreateTemplateParams_Template_Object_Steps_AdditionalProperty_ImageResize{
		Robot: "/image/resize",
		Width: &contract.CreateTemplateParams_Template_Object_Steps_AdditionalProperty_ImageResize_Width{
			Variant: &contract.ValueIntegerOrString{Integer: &width},
		},
		Use: &contract.CreateTemplateParams_Template_Object_Steps_AdditionalProperty_ImageResize_Use{
			Variant: &contract.CreateAssemblyParams_Object2_Steps_AdditionalProperty_AudioArtwork_Use_Variant{String: &original},
		},
		Result: &contract.CreateTemplateParams_Template_Object_Steps_AdditionalProperty_ImageResize_Result{
			Variant: &contract.CreateAssemblyParams_Object2_Steps_AdditionalProperty_TransloaditImport1_ForceAccept_Variant{Boolean: &yes},
		},
	}
	created, err := client.CreateTemplate(ctx, contract.CreateTemplateInput{Params: contract.CreateTemplateParams{
		Name: fmt.Sprintf("sdk-example-%d", time.Now().UnixNano()),
		Template: contract.CreateTemplateParams_Template{Object: &contract.CreateTemplateParams_Template_Object{
			Steps: &contract.CreateTemplateParams_Template_Object_Steps{
				AdditionalProperties: map[string]contract.CreateTemplateParams_Template_Object_Steps_AdditionalProperty{
					"resize": {ImageResize: &step},
				},
			},
		}},
	}})
	if err != nil {
		return err
	}
	id := created.Id.GetString()
	if id == "" {
		return errors.New("the API did not return a Template ID")
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, cleanupErr := client.DeleteTemplate(cleanup, contract.DeleteTemplateInput{TemplateIdOrName: id}); cleanupErr != nil {
			fmt.Fprintln(os.Stderr, "Template cleanup failed:", cleanupErr)
			if err == nil {
				err = cleanupErr
			}
		}
	}()
	if _, err := client.GetTemplate(ctx, contract.GetTemplateInput{TemplateIdOrName: id}); err != nil {
		return err
	}
	file, err := os.Open(os.Args[1])
	if err != nil {
		return err
	}
	// The API call owns the file stream and closes it before returning.
	status, err := client.CreateAssembly(ctx, contract.CreateAssemblyInput{
		Params: contract.CreateAssemblyParams{Object2: &contract.CreateAssemblyParams_Object2{TemplateId: &id}},
		Files:  map[string]contract.UploadFile{"file": {Reader: file, Filename: filepath.Base(os.Args[1])}},
	})
	if err != nil {
		return err
	}
	assemblyID := status.GetAssemblyId()
	if assemblyID == "" {
		return errors.New("the API did not return an Assembly ID")
	}
	finished := false
	defer func() {
		if finished {
			return
		}
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, cleanupErr := client.CancelAssembly(cleanup, contract.CancelAssemblyInput{AssemblyId: assemblyID}); cleanupErr != nil {
			fmt.Fprintln(os.Stderr, "Assembly cleanup failed:", cleanupErr)
			if err == nil {
				err = cleanupErr
			}
		}
	}()
	// Polling is application logic, not an implicit retry or lifecycle engine in the HTTP client.
	for status.GetOk() != "ASSEMBLY_COMPLETED" {
		if assemblyFailed(status) {
			finished = true
			return errors.New("Assembly processing did not complete successfully")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		status, err = client.GetAssembly(ctx, contract.GetAssemblyInput{AssemblyId: assemblyID})
		if err != nil {
			return err
		}
	}
	finished = true
	results := status.GetResults()
	if results == nil || len(results.AdditionalProperties["resize"]) == 0 {
		return errors.New("the completed Assembly has no resized image")
	}
	result := results.AdditionalProperties["resize"][0]
	url := result.SslUrl.GetString()
	if url == "" {
		return errors.New("the resized image has no download URL")
	}
	fmt.Println("Resized image:", url)
	fmt.Println("Download this temporary result before it expires, or add a storage Step.")
	return nil
}

func main() {
	if err := run(); err != nil {
		var response *contract.ResponseError
		if errors.As(err, &response) {
			// Code() is empty for unknown or malformed bodies. TEMPLATE_NOT_FOUND uses HTTP 400.
			fmt.Fprintln(os.Stderr, response.Error(), response.Code())
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
