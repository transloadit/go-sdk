package transloadit

import (
	"time"
)

type AssemblyWaiter struct {
	Response    chan *AssemblyInfo
	Error       chan error
	stop        chan bool
	assemblyUrl string
	stopped     bool
	client      *Client
}

// Wait until the status of an assembly is either completed, canceled or aborted.
func (client *Client) WaitForAssembly(assemblyUrl string) *AssemblyWaiter {
	waiter := &AssemblyWaiter{
		Response:    make(chan *AssemblyInfo),
		Error:       make(chan error),
		stop:        make(chan bool, 1),
		assemblyUrl: assemblyUrl,
		stopped:     false,
		client:      client,
	}

	waiter.start()

	return waiter
}

func (waiter *AssemblyWaiter) start() {
	go func() {
		for {
			select {
			case <-waiter.stop:
				waiter.closeChannels()
				return
			default:
				waiter.poll()
				time.Sleep(time.Second)
			}
		}
	}()
}

// Stop the waiter and close all channels.
func (waiter *AssemblyWaiter) Stop() {
	waiter.stop <- true
}

func (waiter *AssemblyWaiter) closeChannels() {
	close(waiter.Response)
	close(waiter.Error)
}

func (waiter *AssemblyWaiter) poll() {
	res, err := waiter.client.GetAssembly(waiter.assemblyUrl)
	if err != nil {
		waiter.Error <- err
		waiter.Response <- res
		waiter.Stop()
	}

	if res.Ok == "ASSEMBLY_COMPLETED" || res.Ok == "ASSEMBLY_CANCELED" || res.Ok == "REQUEST_ABORTED" {
		waiter.Response <- res
		waiter.Stop()
	}
}
