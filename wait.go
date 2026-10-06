package transloadit

import (
	"context"
	"time"
)

// WaitForAssembly fetches continuously the assembly status until it has
// finished uploading and executing or until an assembly error occurs.
// If you want to end this loop prematurely, you can cancel the supplied context.
func (client *Client) WaitForAssembly(ctx context.Context, assembly *AssemblyInfo) (*AssemblyInfo, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		res, err := client.GetAssembly(ctx, assembly.AssemblySSLURL)
		if err != nil {
			// Keep the caller's cancellation/deadline identifiable through older HTTP error wrappers.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}

		// Abort polling if the assembly has entered an error state
		if res.Error != "" {
			return res, nil
		}

		// Replaying is still active; cancellation, completion and processing errors are terminal.
		if res.Ok != "ASSEMBLY_UPLOADING" && res.Ok != "ASSEMBLY_EXECUTING" && res.Ok != "ASSEMBLY_REPLAYING" {
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
