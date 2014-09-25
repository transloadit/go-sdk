[![Build Status](https://travis-ci.org/transloadit/go-sdk.svg)](https://travis-ci.org/transloadit/go-sdk)
[![Coverage Status](https://coveralls.io/repos/transloadit/go-sdk/badge.png)](https://coveralls.io/r/transloadit/go-sdk)

## go-sdk

A Go SDK to make it easy to talk to the [Transloadit](https://transloadit.com) REST API.

## Example

```go
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
```

For more example, take a look at [`examples/`](https://github.com/transloadit/go-sdk/tree/master/examples).

## Installation

```bash
go get github.com/transloadit/go-sdk
```

The Go SDK requires Go 1.1 or higher.

## Documentation

See [Godoc](http://godoc.org/github.com/transloadit/go-sdk).

## Command line interface

As a **bonus** we ship a command line tool: `transloadify` which provides the functionality of [`Client.Watch`](http://godoc.org/github.com/transloadit/go-sdk#Client.Watch) for simple watching and automated uploading and processing of files. This way you don't have to write a single line of code to get an existing folder converted, even when new files get added to it

```bash
# Use -h for more help
transloadify -h

# Upload all files from ./input and process them using the steps defined in the template with the id 'tpl123id'.
# Download the results and put them into ./output.
# Watch the input directory to automatically upload all new files.
TRANSLOADIT_KEY=abc123 \
TRANSLOADIT_SECRET=abc123efg \
./transloadify \
  -input="./input" \
  -output="./output" \
  -template="tpl123id" \
  -watch
```

Instead of using a template id you can also load the steps from a local template file using the `template-file` option (see [`examples/imgresize.json`](examples/imgresize.json), for example):
```bash
TRANSLOADIT_KEY=abc123 \
TRANSLOADIT_SECRET=abc123efg \
./transloadify \
  -input="./input" \
  -output="./output" \
  -template-file="./examples/imgresize.json" \
  -watch
```

### Installation

There are multiple way to obtain the `transloadify` binary:

**Gobuild**

Use [gobuild.io](http://gobuild.io/download/github.com/transloadit/go-sdk/transloadify) to select your OS and download a zipped version of the ready-to-use binary.

**go get**

```bash
go get github.com/transloadit/go-sdk/transloadify

# Use the binary
$GOPATH/bin/transloadify -h
```

## Development

If you want to get into Transloadit Go SDK or Transloadify development, here are the steps:

### Set up Go

If you haven't already, [download Go](http://golang.org/dl/) for your platform.

### Paths

You [don't need GOROOT](http://dave.cheney.net/2013/06/14/you-dont-need-to-set-goroot-)

```bash
unset GOROOT
```

Set `GOPATH` to your projects directory, e.g.:

```bash
export GOPATH=~/go
```

### Get the SDK & Dependencies

```bash
mkdir -p $GOPATH/src/github.com/transloadit && \
cd $_ && \
git clone https://github.com/transloadit/go-sdk.git && \
cd go-sdk && \
go get github.com/transloadit/go-sdk/transloadify
```

### Run transloadify in debug mode

```bash
go run transloadify/transloadify.go -h
```

### Build

```bash
make build
```

### Release

Releasing requires the [AWS Command Line Interface
](http://aws.amazon.com/cli/) and write access to the `transloadify` S3 bucket, hence this can only be done by Transloadit's staff.

Depending on [SemVer](http://semver.org/) impact, any of the following will release a new version

```bash
make release bump=major
make release bump=minor
make release bump=patch
```

This means:

 - Aborts unless working tree is clean
 - Build to `./bin`
 - Test
 - Bumps specified SemVer part in `./VERSION`
 - Commits the file with msg "Release v<version>"
 - Creates a Git tag with this version
 - Pushes commit & tag to GitHub
 - Runs gobuild.io on this tag for *most* platforms, saving to `./builds`
 - Saves them to S3 as `s3://transloadify/transloadify-<platform>-<arch>-<version>` with `public-read` access, making the file accessible as e.g. http://transloadify.s3.amazonaws.com/transloadify-darwin-amd64-v0.1.0
 - Clears the `./builds` directory

## License

[MIT Licensed](LICENSE)
