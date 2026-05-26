package transloadit

import (
	"context"
	"time"
)

// <api2-generated-feature waitForAssembly>

// This block is generated from Transloadit API2 contracts. If it looks wrong,
// please report the issue instead of editing this block by hand; the source fix
// belongs in the contract generator so all SDKs stay in sync.

// WaitForAssembly fetches continuously the assembly status until it has
// finished uploading and executing or until an assembly error occurs.
// If you want to end this loop prematurely, you can cancel the supplied context.
func (client *Client) WaitForAssembly(ctx context.Context, assembly *AssemblyInfo) (*AssemblyInfo, error) {
	for {
		res, err := client.GetAssembly(ctx, assembly.AssemblySSLURL)
		if err != nil {
			return nil, err
		}

		// Abort polling if the assembly has entered an error state
		if res.Error != "" {
			return res, nil
		}

		// The polling is done if the assembly is not uploading or executing anymore.
		if res.Ok != "ASSEMBLY_UPLOADING" && res.Ok != "ASSEMBLY_EXECUTING" {
			return res, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
			continue
		}
	}
}

// </api2-generated-feature waitForAssembly>
