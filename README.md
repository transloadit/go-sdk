[![Build Status](https://travis-ci.org/transloadit/go-sdk.svg)](https://travis-ci.org/transloadit/go-sdk)
[![Coverage Status](https://coveralls.io/repos/transloadit/go-sdk/badge.png)](https://coveralls.io/r/transloadit/go-sdk)

## go-sdk

A Go SDK to make it easy to talk to the [Transloadit](https://transloadit.com) REST API.

## Example

```go
// Create client
options := transloadit.DefaultConfig
options.AuthKey = "AUTH_KEY"
options.AuthSecret = "AUTH_SECRET"
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
```

For more example, take a look at [`examples/`](https://github.com/transloadit/go-sdk/tree/master/examples).

## Installation

```sh
go get github.com/transloadit/go-sdk
```

The Go SDK requires Go 1.1 or higher.

## Documentation

See [Godoc](http://godoc.org/github.com/transloadit/go-sdk).

## Command line interface

The `transloadify` command provides the functionality of [`Client.Watch`](http://godoc.org/github.com/transloadit/go-sdk#Client.Watch) in the command line for simple watching and automated uploading and processing of files:

```sh
# Use -h for more help
transloadify -h

# Upload all files from ./input and process them using the steps defined in the template with the id 'tpl123id'.
# Download the results and put them into ./output.
# Watch the input directory to automatically upload all new files.
transloadify -key=$AUTH_KEY -secret=$AUTH_SECRET \
  -input="./input" -output="./output" -template="tpl123id" -watch
```

### Installation

There are multiple way to obtain the `transloadify` binary:

**Gobuild**

Use [gobuild.io](http://gobuild.io/download/github.com/transloadit/go-sdk/transloadify) to select your OS and download a zipped version of the ready-to-use binary.

**go get**

```sh
go get github.com/transloadit/go-sdk/transloadify

# Use the binary
$GOPATH/bin/transloadify -h
```

**Github**

```sh
git clone https://github.com/transloadit/go-sdk.git
cd go-sdk
make build
./transloadify -h
```

## License

[MIT Licensed](LICENSE)
