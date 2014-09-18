package main

import (
	"fmt"
	"github.com/transloadit/go-sdk"
	"os"
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

	file, err := os.Open("../../fixtures/lol_cat.jpg")
	if err != nil {
		panic(err)
	}

	// Add an io.Reader to upload
	assembly.AddReader("image", "lol_cat.jpg", file)

	// Add instructions, e.g. resize image to 75x75px
	assembly.AddStep("resize", map[string]interface{}{
		"robot":           "/image/resize",
		"width":           75,
		"height":          75,
		"resize_strategy": "pad",
		"background":      "#000000",
	})

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
