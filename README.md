[![Build Status](https://travis-ci.org/transloadit/go-sdk.svg)](https://travis-ci.org/transloadit/go-sdk)
[![Coverage Status](https://coveralls.io/repos/transloadit/go-sdk/badge.png)](https://coveralls.io/r/transloadit/go-sdk)

## go-sdk

A Go SDK to make it easy to talk to the [Transloadit](https://transloadit.com) REST API.

We also offer [transloadify](https://github.com/transloadit/transloadify) that bundles much of the technology inside this SDK as a commandline utility.

## Example

```go
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
assembly.AddFile("myimage", "./lol_cat.jpg")

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
```

For examples on how to use templates, non-blocking processing and more, take a look at [`examples/`](https://github.com/transloadit/go-sdk/tree/master/examples).

## Installation

```bash
go get github.com/transloadit/go-sdk
```

The Go SDK requires Go 1.1 or higher.

## Documentation

See [Godoc](http://godoc.org/github.com/transloadit/go-sdk).

## License

[MIT Licensed](LICENSE)
