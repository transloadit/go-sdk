package transloadit

import "context"

// WaitForAssembly fetches continuously the assembly status until is either
// completed (ASSEMBLY_COMPLETED), canceled (ASSEMBLY_CANCELED) or aborted
// (REQUEST_ABORTED). If you want to end this loop prematurely, you can cancel
// the supplied context.
// The assembly URL must be absolute, for example:
// https://api2-amberly.transloadit.com/assemblies/15a6b3701d3811e78d7bfba4db1b053e
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
