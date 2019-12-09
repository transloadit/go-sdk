[![Build Status](https://travis-ci.org/transloadit/go-sdk.svg)](https://travis-ci.org/transloadit/go-sdk)
[![Coverage Status](https://coveralls.io/repos/transloadit/go-sdk/badge.png)](https://coveralls.io/r/transloadit/go-sdk)

# go-sdk

A **Go** Integration for [Transloadit](https://transloadit.com)'s file uploading and encoding service

## Intro

[Transloadit](https://transloadit.com) is a service that helps you handle file uploads, resize, crop and watermark your images, make GIFs, transcode your videos, extract thumbnails, generate audio waveforms, and so much more. In short, [Transloadit](https://transloadit.com) is the Swiss Army Knife for your files.

This is a **Go** SDK to make it easy to talk to the [Transloadit](https://transloadit.com) REST API.

## Install

```bash
go get gopkg.in/transloadit/go-sdk.v1
```

The Go SDK requires Go 1.7 or higher.

## Usage

```go
package main

import (
	"context"
	"fmt"

	"gopkg.in/transloadit/go-sdk.v1"
)

func main() {
	// Create client
	options := transloadit.DefaultConfig
	options.AuthKey = "YOUR_TRANSLOADIT_KEY"
	options.AuthSecret = "YOUR_TRANSLOADIT_SECRET"
	client := transloadit.NewClient(options)

	// Initialize new assembly
	assembly := transloadit.NewAssembly()

	// Add a file to upload
	assembly.AddFile("image", "/PATH/TO/FILE.jpg")

	// Add Instructions, e.g. resize image to 75x75px
	assembly.AddStep("resize", map[string]interface{}{
		"robot":           "/image/resize",
		"width":           75,
		"height":          75,
		"resize_strategy": "pad",
		"background":      "#000000",
	})

	// Start the upload
	info, err := client.StartAssembly(context.Background(), assembly)
	if err != nil {
		panic(err)
	}

	// All files have now been uploaded and the assembly has started but no
	// results are available yet since the conversion has not finished.
	// WaitForAssembly provides functionality for polling until the assembly
	// has ended.
	info, err = client.WaitForAssembly(context.Background(), info)
	if err != nil {
		panic(err)
	}

	fmt.Printf("You can view the result at: %s\n", info.Results["resize"][0].SSLURL)
}
```

## Example

For fully working examples on how to use templates, non-blocking processing and more, take a look at [`examples/`](https://github.com/transloadit/go-sdk/tree/master/examples).

## Documentation

See <a href="https://godoc.org/gopkg.in/transloadit/go-sdk.v1">Godoc</a> for full API documentation.

## License

[MIT Licensed](LICENSE)
