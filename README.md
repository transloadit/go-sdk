# go-sdk

A **Go** Integration for [Transloadit](https://transloadit.com)'s file uploading and encoding service

## Intro

[Transloadit](https://transloadit.com) is a service that helps you handle file uploads, resize, crop and watermark your images, make GIFs, transcode your videos, extract thumbnails, generate audio waveforms, and so much more. In short, [Transloadit](https://transloadit.com) is the Swiss Army Knife for your files.

This is a **Go** SDK to make it easy to talk to the [Transloadit](https://transloadit.com) REST API.

## Install

```bash
go get github.com/transloadit/go-sdk
```

The Go SDK is confirmed to work with Go 1.11 or higher.

## Usage

```go
package main

import (
	"context"
	"fmt"

	"github.com/transloadit/go-sdk"
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

## Contract-generated API methods (experimental)

`client.Contract()` creates a typed low-level client using the existing SDK credentials, endpoint
and HTTP client. Existing methods keep their behavior. You can also construct
`contract.NewClient(contract.Config{AuthKey: key, AuthSecret: secret})` from
`github.com/transloadit/go-sdk/contract`.

```go
api, err := client.Contract()
if err != nil {
    return err
}
templates, err := api.ListTemplates(ctx, contract.ListTemplatesInput{})
```

The generated namespace covers ordinary HTTP operations. It returns the HTTP response, not a
completed Assembly. Keep using existing SDK methods for upload orchestration, tus and polling.
SSE, capability URLs and Webhook receivers are separate work. Go 1.15 remains supported.

Set `BearerToken` instead of Auth Key credentials to use an existing token; the client never mints
one implicitly. Pass raw, unencoded path values. Signed requests authenticate the exact serialized
`params` bytes sent. Multipart files transfer `io.ReadCloser` ownership to the API call, which closes
every supplied stream before returning, including on cancellation or early responses. `Close` must
unblock a concurrent `Read`; wrap in-memory readers with `ioutil.NopCloser`. The default client has
no total upload deadline: use a context deadline or an explicitly configured HTTP client. Optional
fields are pointers so `false` and `0` are not lost. Nullable fields use generated union wrappers so explicit
`null` differs from omission. Set exactly one union choice (or its null choice). These wire types
are not a full JSON Schema validator. `Integer` uses signed 64-bit storage and accepts integral
decimal/exponent JSON representations without rounding. Unknown response fields are tolerated;
fields explicitly modeled as additional properties are retained. Union decoding selects an alternative
that retains the fields represented by the successful alternatives, or fails if none can retain them
all. `ResponseError` retains status and decoded JSON without printing response data. Redirects are
rejected and responses are limited to 128 MiB.

Maintainers: `contract/client_generated.go` and its JSON manifests come from API2. Never edit them
directly. From the matching API2 checkout's `api2/` directory run `./bin/cli.ts contracts sdks
--target go --output <go-sdk>/contract`, then repeat with `--check`. The manifest records the exact
contract digest. Native behavior belongs in `contract/transport.go` and its tests; API2 pins these
sources for regeneration and local-server acceptance. Update that pin when changing them. The
coverage report keeps missing targets and protocols visible; generated does not mean runtime-proven.

## Example

For fully working examples on how to use templates, non-blocking processing and more, take a look at [`examples/`](https://github.com/transloadit/go-sdk/tree/main/examples).

## Documentation

See <a href="https://pkg.go.dev/github.com/transloadit/go-sdk">Godoc</a> for full API documentation.

## License

[MIT Licensed](LICENSE)
