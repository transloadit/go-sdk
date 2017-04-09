[![Build Status](https://travis-ci.org/transloadit/go-sdk.svg)](https://travis-ci.org/transloadit/go-sdk)
[![Coverage Status](https://coveralls.io/repos/transloadit/go-sdk/badge.png)](https://coveralls.io/r/transloadit/go-sdk)

## go-sdk

A **Go** Integration for [Transloadit](https://transloadit.com)'s file uploading and encoding service

## Intro

[Transloadit](https://transloadit.com) is a service that helps you handle file uploads, resize, crop and watermark your images, make GIFs, transcode your videos, extract thumbnails, generate audio waveforms, and so much more. In short, [Transloadit](https://transloadit.com) is the Swiss Army Knife for your files.

This is a **Go** SDK to make it easy to talk to the [Transloadit](https://transloadit.com) REST API.

## Install

```bash
go get gopkg.in/transloadit/go-sdk.v1
```

The Go SDK requires Go 1.1 or higher.

## Usage

```go
package main

import (
    "fmt"
    "go get gopkg.in/transloadit/go-sdk.v1"
)

func main () {
    // Create client
    options := transloadit.DefaultConfig
    options.AuthKey = "TRANSLOADIT_KEY"
    options.AuthSecret = "TRANSLOADIT_SECRET"
    client, err := transloadit.NewClient(options)
    if err != nil {
        panic(err)
    }

    // Initialize new assembly
    assembly := client.CreateAssembly()

    // Add input file to upload
    assembly.AddFile("myimage", "/PATH/TO/FILE.jpg")

    // Add instructions, e.g. resize image to 75x75px
    assembly.AddStep("resize", map[string]interface{}{
        "robot":           "/image/resize",
        "width":           75,
        "height":          75,
        "resize_strategy": "pad",
        "background":      "#000000",
    })

    // Wait until Transloadit is done processing all uploads
    // and is ready to download the results
    assembly.Blocking = true

    // Start the upload
    info, err := assembly.Upload()
    if err != nil {
        panic(err)
    }

    fmt.Printf("You can view the result at: %s\n", info.Results["resize"][0].Url)
}
```

## Example

For fully working examples on how to use templates, non-blocking processing and more, take a look at [`examples/`](https://github.com/transloadit/go-sdk/tree/master/examples).

## Documentation

See [Godoc](http://godoc.org/gopkg.in/transloadit/go-sdk.v1).

## License

[MIT Licensed](LICENSE)
