package main

import (
	"fmt"

	"github.com/transloadit/go-sdk"
)

func main() {

	// Create client
	options := transloadit.DefaultConfig
	options.AuthKey = "TRANSLOADIT_KEY"
	options.AuthSecret = "TRANSLOADIT_SECRET"
	client, err := transloadit.NewClient(options)
	if err != nil {
		panic(err)
	}

	// Initialize new assembly
	assembly := client.NewAssembly()

	// Add a file to upload
	assembly.AddFile("image", "../../fixtures/lol_cat.jpg")

	// Instructions will be read from the template
	// with specified id stored on Transloadit's servers.
	assembly.TemplateId = "TRANSLOADIT_TEMPLATE_ID"

	// Wait until Transloadit is done processing all uploads
	// and is ready to download the results
	assembly.Blocking = true

	// Start the upload
	info, err := assembly.Start()
	if err != nil {
		panic(err)
	}

	fmt.Printf("You can view the result at: %s\n", info.Results["resize"][0].Url)

}
