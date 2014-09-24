package transloadit

import (
	"errors"
	"fmt"
	"gopkg.in/fsnotify.v1"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

type WatchOptions struct {
	// Directory to watch files in (only if Watch is true)
	Input string
	// Directoy to put result files in
	Output string
	// Watch the input directory or just compile files in input directory
	Watch bool
	// Template id to convert files with
	TemplateId string
	// Optional notify url for each assembly.
	NotifyUrl string
	// Instead of using templates you can define steps
	Steps map[string]map[string]interface{}
	// If true the original files will be copied in the output directoy with `-original_0_` prefix.
	// If false input files will be deleted.
	Preserve bool
}

type Watcher struct {
	client  *Client
	options *WatchOptions
	stopped bool
	// Listen for errors
	Error chan error
	// Listen for completed assemblies
	Done chan *AssemblyInfo
	// Listen for file changes (only if Watch == true)
	Change       chan string
	end          chan bool
	recentWrites map[string]time.Time
	blacklist    map[string]bool
}

// Watch a directory for changes and convert all changes files and download the result.
// It will create a new assembly for each file.
// If the directory already contains some they are all converted.
// See WatchOptions for possible configuration.
func (client *Client) Watch(options *WatchOptions) *Watcher {

	watcher := &Watcher{
		client:       client,
		options:      options,
		Error:        make(chan error),
		Done:         make(chan *AssemblyInfo),
		Change:       make(chan string),
		end:          make(chan bool),
		recentWrites: make(map[string]time.Time),
		blacklist:    make(map[string]bool),
	}

	watcher.start()

	return watcher

}

func (watcher *Watcher) start() {

	watcher.processDir()

	if watcher.options.Watch {
		go watcher.startWatcher()
	}

}

// Stop the watcher.
func (watcher *Watcher) Stop() {

	if watcher.stopped {
		return
	}

	watcher.stopped = true

	watcher.end <- true
	close(watcher.Done)
	close(watcher.Error)
	close(watcher.Change)
	close(watcher.end)
}

func (watcher *Watcher) processDir() {

	files, err := ioutil.ReadDir(watcher.options.Input)
	if err != nil {
		watcher.error(err)
		return
	}

	input := watcher.options.Input

	for _, file := range files {
		if !file.IsDir() {
			go watcher.processFile(path.Join(input, file.Name()))
		}
	}

}

func (watcher *Watcher) processFile(name string) {

	file, err := os.Open(name)
	if err != nil {
		watcher.error(err)
		return
	}

	// Add file to blacklist
	log.Printf("Adding to blacklist: '%s'", name)
	watcher.blacklist[name] = true

	assembly := watcher.client.CreateAssembly()

	if watcher.options.TemplateId != "" {
		assembly.TemplateId = watcher.options.TemplateId
	}

	if watcher.options.NotifyUrl != "" {
		assembly.NotifyUrl = watcher.options.NotifyUrl
	}

	for name, step := range watcher.options.Steps {
		assembly.AddStep(name, step)
	}

	assembly.Blocking = true

	assembly.AddReader("file", path.Base(name), file)

	info, err := assembly.Upload()
	if err != nil {
		watcher.error(err)
		return
	}

	if info.Error != "" {
		watcher.error(errors.New(info.Error))
		return
	}

	for stepName, results := range info.Results {
		for index, result := range results {
			go func() {
				watcher.downloadResult(stepName, index, result)
				watcher.handleOriginalFile(name)
				log.Printf("Removing from blacklist: '%s'", name)
				delete(watcher.blacklist, name)
				watcher.Done <- info
			}()
		}
	}
}

func (watcher *Watcher) downloadResult(stepName string, index int, result *FileInfo) {

	fileName := fmt.Sprintf("%s_%d_%s", stepName, index, result.Name)

	resp, err := http.Get(result.Url)
	if err != nil {
		watcher.error(err)
		return
	}

	defer resp.Body.Close()

	out, err := os.Create(path.Join(watcher.options.Output, fileName))
	if err != nil {
		watcher.error(err)
		return
	}

	defer out.Close()

	io.Copy(out, resp.Body)

}

func (watcher *Watcher) startWatcher() {

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		watcher.error(err)
	}

	defer fsWatcher.Close()

	if err = fsWatcher.Add(watcher.options.Input); err != nil {
		watcher.error(err)
	}

	go func() {
		for {

			if watcher.stopped {
				return
			}

			time.Sleep(time.Second)
			now := time.Now()

			for name, lastEvent := range watcher.recentWrites {
				diff := now.Sub(lastEvent)
				if diff > (time.Millisecond * 500) {
					delete(watcher.recentWrites, name)
					watcher.Change <- name
					go watcher.processFile(name)
				}
			}

		}
	}()

	for {
		select {
		case <-watcher.end:
			return
		case err := <-fsWatcher.Errors:
			watcher.error(err)
		case evt := <-fsWatcher.Events:
			// Ignore the event if the file is currently processed
			log.Printf("Checking blacklist: '%s'", evt.Name)
			if _, ok := watcher.blacklist[evt.Name]; ok == true {
				continue
			}
			if evt.Op&fsnotify.Create == fsnotify.Create || evt.Op&fsnotify.Write == fsnotify.Write {
				watcher.recentWrites[evt.Name] = time.Now()
			}
		}
	}

}

func (watcher *Watcher) handleOriginalFile(name string) {

	var err error
	if watcher.options.Preserve {
		_, file := path.Split(name)
		err = os.Rename(name, watcher.options.Output+"/-original_0_"+basename(file))
	} else {
		err = os.Remove(name)
	}

	if err != nil {
		watcher.error(err)
	}

}

func (watcher *Watcher) error(err error) {
	watcher.Error <- err
}

func basename(name string) string {
	i := strings.LastIndex(name, string(os.PathSeparator))
	return name[i+1:]
}
