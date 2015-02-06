package main

import (
	"fmt"
	"os"

	"github.com/transloadit/go-sdk"
)

func main() {

	// Create client
	options := transloadit.DefaultConfig
	options.AuthKey = "TRANSLOADIT_KEY"
	options.AuthSecret = "TRANSLOADIT_SECRET"
	client, err := transloadit.NewClient(&options)
	if err != nil {
		panic(err)
	}

	// Initialize new assembly
	assembly := client.CreateAssembly()

	// Add an file to upload
	assembly.AddReader("image", "../../fixtures/lol_cat.jpg")

	// Instructions will be read from the template
	// `02a8693053cd11e49b9ba916b58830db` stored on Transloadit's servers.
	assembly.TemplateId = "02a8693053cd11e49b9ba916b58830db"

	// Wait until transloadit is done processing all uploads
	// and is ready to download the results
	assembly.Blocking = true

	// Start the upload
	info, err := assembly.Upload()
	if err != nil {
		panic(err)
	}

	fmt.Printf("You can view the result at: %s\n", info.Results["resize"][0].Url)

}
