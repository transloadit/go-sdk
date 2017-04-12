package transloadit

import "context"

// WaitForAssembly fetches continuously the assembly status until is either
// completed (ASSEMBLY_COMPLETED), canceled (ASSEMBLY_CANCELED) or aborted
// (REQUEST_ABORTED). If you want to end this loop prematurely, you can cancel
// the supplied context.
func (client *Client) WaitForAssembly(ctx context.Context, assembly *AssemblyInfo) (*AssemblyInfo, error) {
	for {
		res, err := client.GetAssembly(ctx, assembly.AssemblyUrl)
		if err != nil {
			return nil, err
		}

		if res.Ok == "ASSEMBLY_COMPLETED" || res.Ok == "ASSEMBLY_CANCELED" || res.Ok == "REQUEST_ABORTED" {
			return res, nil
		}
	}
}
