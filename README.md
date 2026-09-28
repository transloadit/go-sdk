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

Import `github.com/transloadit/go-sdk/contract` explicitly to use the typed low-level client.
Existing SDK consumers do not compile the generated package unless they import it. Existing
APIs remain available alongside the opt-in package.

```go
api, err := contract.NewClient(contract.Config{AuthKey: key, AuthSecret: secret})
if err != nil {
    return err
}
templates, err := api.ListTemplates(ctx, contract.ListTemplatesInput{})
```

The generated namespace covers ordinary HTTP operations. It returns the HTTP response, not a
completed Assembly. `CreateAssembly.Files` supports multipart uploads. Polling and upload
orchestration remain caller-owned, as shown in the example below, or use the existing high-level
SDK methods. Resumable tus upload is not implemented in either Go API.
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
all. Generated union members use schema discriminants such as `ImageResize` where available.
Public type aliases can be followed with `go doc`; comments come from the contract. Assembly unions
provide `GetAssemblyId()`, `GetOk()` and `GetResults()` so callers need not guess which success
variant was decoded. Scalar wrappers offer methods such as `GetString()`. These accessors return
zero values for absent/null fields or an inactive scalar variant; inspect the variant pointers when
the distinction matters. Both wire spellings of the legacy Assembly ID remain represented:
`AssemblyId` is `assembly_id`, and `AssemblyIdCamelCase` is `assemblyId`.

`ResponseError` retains status and decoded JSON without printing response data. Use
`errors.As(err, &response)` with `var response *contract.ResponseError`, then `response.Code()`
to classify recognized public codes. Unknown or malformed codes return an empty string. For example,
`TEMPLATE_NOT_FOUND` uses HTTP 400, not 404; the SDK preserves that API behavior. Redirects are
rejected and responses are limited to 128 MiB. Union decoding rejects values deeper than 64 nested
objects/arrays to bound native decoding work. This is a client resource limit, not an API schema rule.

The [complete generated-client example](examples/contract-workflow/main.go) builds typed Template
Steps, uploads an image, polls to completion and reads a typed result. With server-side
`TRANSLOADIT_KEY` and `TRANSLOADIT_SECRET` set, run `go run ./examples/contract-workflow ./image.jpg`
from this checkout. It creates one billable Assembly and removes its temporary Template; completed
results expire normally. Its deadline, polling and cleanup are application logic, not automatic
SDK retries or a replacement for the existing high-level client.

Maintainers: `contract/client_generated.go` and its JSON manifests come from API2. Never edit them
directly. From the matching API2 checkout's `api2/` directory run `./bin/cli.ts contracts sdks
--target go --output <go-sdk>/contract`, then repeat with `--check`. The manifest records the exact
contract digest. Native behavior belongs in `contract/transport.go` and its tests; API2 pins these
sources for regeneration and local-server acceptance. Update that pin when changing them. The
coverage report keeps missing targets and protocols visible; generated does not mean runtime-proven.

The experimental types use API2-owned domains such as `AssemblySteps`, `ApiError` and
`JsonDocument`. Their names do not depend on which endpoint happens to be generated first. This
unreleased draft intentionally replaces earlier operation-prefixed type names; existing SDK APIs
are unchanged. Model naming belongs in API2's `api2/lib/contract/schemaModels.ts`, not local aliases.

API2 also owns `contract/workflow-vectors.json`. `go test -race . -run '^TestSharedWorkflow' -v`
exercises the existing public upload, wait, cancellation and Smart CDN methods with the same
observations used by the Node SDK. Tests use synthetic credentials and loopback HTTP servers;
adapters may not implement missing SDK polling, retries or signing. CI runs these cases as part of
the root tests. **Resumable tus upload is explicitly unsupported in this Go SDK**, so that named
case reports `SKIP`, not a pass. New unclassified cases fail. Passing fixture tests is not proof of
live-server behavior or of workflows in the generated `contract` package. Change shared scenarios
in API2's `api2/lib/contract/sdk/workflowVectors.ts` and regenerate rather than editing the JSON.

The workflow tests also correct existing SDK behavior: Smart CDN signing now uses the server's
path/query encoding and UTF-16 key order, preserves an explicit expiry's milliseconds, and replaces
stale authentication fields. Default expiry also retains millisecond precision, so generated URLs
can differ from earlier SDK versions. Waiting continues through `ASSEMBLY_REPLAYING` and preserves
identifiable context cancellation/deadline errors. A canceled or failed Assembly remains a terminal
result, not a successful completion.

## Example

For fully working examples on how to use templates, non-blocking processing and more, take a look at [`examples/`](https://github.com/transloadit/go-sdk/tree/main/examples).

## Documentation

See <a href="https://pkg.go.dev/github.com/transloadit/go-sdk">Godoc</a> for full API documentation.

## License

[MIT Licensed](LICENSE)
