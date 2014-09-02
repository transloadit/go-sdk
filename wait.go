package transloadit

import (
	"time"
)

type AssemblyWatcher struct {
	Response    chan *AssemblyInfo
	Error       chan error
	assemblyUrl string
	stopped     bool
	client      *Client
}

func (client *Client) WaitForAssembly(assemblyUrl string) *AssemblyWatcher {

	watcher := &AssemblyWatcher{
		Response:    make(chan *AssemblyInfo),
		Error:       make(chan error),
		assemblyUrl: assemblyUrl,
		stopped:     false,
		client:      client,
	}

	watcher.Start()

	return watcher

}

func (watcher *AssemblyWatcher) Start() {

	go func() {

		for {

			if watcher.stopped {
				watcher.closeChannels()
				break
			}

			watcher.poll()

			time.Sleep(time.Second)

		}

	}()

}

func (watcher *AssemblyWatcher) Stop() {
	watcher.stopped = true
}

func (watcher *AssemblyWatcher) closeChannels() {
	close(watcher.Response)
	close(watcher.Error)
}

func (watcher *AssemblyWatcher) poll() {

	res, err := watcher.client.GetAssembly(watcher.assemblyUrl)
	if err != nil {
		watcher.Error <- err
		watcher.Response <- res
		watcher.Stop()
	}

	if res.Ok == "ASSEMBLY_COMPLETED" || res.Ok == "ASSEMBLY_CANCELED" || res.Ok == "REQUEST_ABORTED" {
		watcher.Response <- res
		watcher.Stop()
	}

}
