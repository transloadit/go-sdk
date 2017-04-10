package transloadit

import "context"

// Wait until the status of an assembly is either completed, canceled or aborted.
func (client *Client) WaitForAssembly(ctx context.Context, assemblyUrl string) (*AssemblyInfo, error) {
	for {
		res, err := client.GetAssembly(ctx, assemblyUrl)
		if err != nil {
			return nil, err
		}

		if res.Ok == "ASSEMBLY_COMPLETED" || res.Ok == "ASSEMBLY_CANCELED" || res.Ok == "REQUEST_ABORTED" {
			return res, nil
		}
	}
}
