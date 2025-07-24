package main

import (
	"fmt"
	"net/url"

	transloadit "github.com/transloadit/go-sdk"
)

func main() {
	url := GetSmartCDNUrl()
	fmt.Println(url)
}

func GetSmartCDNUrl() string {
	options := transloadit.DefaultConfig
	options.AuthKey = "YOUR_TRANSLOADIT_KEY"
	options.AuthSecret = "YOUR_TRANSLOADIT_SECRET"
	client := transloadit.NewClient(options)

	params := url.Values{}
	params.Add("height", "100")
	params.Add("width", "100")

	url := client.CreateSignedSmartCDNUrl(transloadit.SignedSmartCDNUrlOptions{
		Workspace: "YOUR_WORKSPACE",
		Template:  "YOUR_TEMPLATE",
		Input:     "image.png",
		URLParams: params,
	})

	return url
}
