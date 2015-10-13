package transloadit

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mitchellh/go-homedir"
	"gopkg.in/fsnotify.v1"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9\-\_\.]`)

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
	// Read template from JSON file. See exmaples for information about the format.
	TemplateFile string
	// If true file which are already in the directory won't be compiled.
	DontProcessDir bool
}

type Watcher struct {
	client *Client
	// Original options passed to Client.Watch.
	// If Input or Output are relative to the home directory
	// they will be extended (~/input -> /home/user/input)
	Options *WatchOptions
	stopped bool
	// Listen for errors
	Error chan error
	// Listen for completed assemblies
	Done chan *AssemblyInfo
	// Listen for file changes (only if Watch == true)
	Change       chan string
	stop         chan bool
	recentWrites map[string]time.Time
	blacklist    map[string]bool
}

// Watch a directory for changes and convert all changes files and download the result.
// It will create a new assembly for each file.
// If the directory already contains some they are all converted.
// See WatchOptions for possible configuration.
func (client *Client) Watch(options *WatchOptions) *Watcher {
	options.Input = expandPath(options.Input)
	options.Output = expandPath(options.Output)

	if options.TemplateFile != "" {
		options.TemplateFile = expandPath(options.TemplateFile)
		options.Steps = readJson(options.TemplateFile)
	}

	if _, err := os.Stat(options.Input); os.IsNotExist(err) {
		panic(fmt.Errorf("Input directory does not exist: %s", options.Input))
	}

	if _, err := os.Stat(options.Output); os.IsNotExist(err) {
		panic(fmt.Errorf("Output directory does not exist: %s", options.Output))
	}

	if options.Input == options.Output {
		panic(fmt.Errorf("Input and output directory are both: %s", options.Output))
	}

	watcher := &Watcher{
		client:       client,
		Options:      options,
		Error:        make(chan error, 1),
		Done:         make(chan *AssemblyInfo),
		Change:       make(chan string),
		stop:         make(chan bool, 1),
		recentWrites: make(map[string]time.Time),
		blacklist:    make(map[string]bool),
	}

	watcher.start()

	return watcher
}

func (watcher *Watcher) start() {
	if watcher.Options.Watch {
		go watcher.startWatcher()
	}

	if !watcher.Options.DontProcessDir {
		go watcher.processDir()
	}

	// If we have nothing to do (neither processing the input directory nor
	// watching it for changes) stop everything again
	if !watcher.Options.Watch && watcher.Options.DontProcessDir {
		watcher.Stop()
	}
}

// Stop the watcher.
func (watcher *Watcher) Stop() {
	if watcher.stopped {
		return
	}

	watcher.stopped = true

	close(watcher.Done)
	close(watcher.Error)
	close(watcher.Change)
	close(watcher.stop)
}

func (watcher *Watcher) processDir() {
	files, err := ioutil.ReadDir(watcher.Options.Input)
	if err != nil {
		watcher.error(err)
		return
	}

	input := watcher.Options.Input
	var wg sync.WaitGroup

	for _, file := range files {
		if !file.IsDir() {
			wg.Add(1)
			go func(file os.FileInfo) {
				watcher.processFile(path.Join(input, file.Name()))
				wg.Done()
			}(file)
		}
	}

	wg.Wait()

	// If watching is not enabled, stop and close all channels to avoid a
	// deadlock since we are done with everything.
	if !watcher.Options.Watch {
		watcher.Stop()
	}
}

func (watcher *Watcher) processFile(name string) {
	file, err := os.Open(name)
	if err != nil {
		watcher.error(err)
		return
	}

	// Add file to blacklist
	watcher.blacklist[name] = true

	assembly := watcher.client.CreateAssembly()

	if watcher.Options.TemplateId != "" {
		assembly.TemplateId = watcher.Options.TemplateId
	}

	if watcher.Options.NotifyUrl != "" {
		assembly.NotifyUrl = watcher.Options.NotifyUrl
	}

	for name, step := range watcher.Options.Steps {
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

	var wg sync.WaitGroup

	for stepName, results := range info.Results {
		for index, result := range results {
			wg.Add(1)
			go func() {
				watcher.downloadResult(stepName, index, result)
				watcher.handleOriginalFile(name)
				delete(watcher.blacklist, name)
				watcher.Done <- info
				wg.Done()
			}()
		}
	}

	wg.Wait()

	if !watcher.Options.Watch {
		watcher.Stop()
	}
}

func (watcher *Watcher) downloadResult(stepName string, index int, result *FileInfo) {
	fileName := sanitizeRe.ReplaceAllString(fmt.Sprintf("%s_%d_%s", stepName, index, result.Name), "-")

	resp, err := http.Get(result.Url)
	if err != nil {
		watcher.error(err)
		return
	}

	defer resp.Body.Close()

	out, err := os.Create(path.Join(watcher.Options.Output, fileName))
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
		return
	}

	defer fsWatcher.Close()

	if err = fsWatcher.Add(watcher.Options.Input); err != nil {
		watcher.error(err)
		return
	}

	for {
		select {
		case _, more := <-watcher.stop:
			if !more {
				return
			}
		case err := <-fsWatcher.Errors:
			watcher.error(err)
		case evt := <-fsWatcher.Events:
			// Ignore the event if the file is currently processed
			if _, ok := watcher.blacklist[evt.Name]; ok == true {
				continue
			}
			if evt.Op&fsnotify.Create == fsnotify.Create || evt.Op&fsnotify.Write == fsnotify.Write {
				watcher.recentWrites[evt.Name] = time.Now()
			}
		case <-time.Tick(1 * time.Second):
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
	}
}

func (watcher *Watcher) handleOriginalFile(name string) {
	var err error
	if watcher.Options.Preserve {
		_, file := path.Split(name)
		err = os.Rename(name, watcher.Options.Output+"/-original_0_"+basename(file))
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

func expandPath(str string) string {
	expanded, err := homedir.Expand(str)
	if err != nil {
		panic(err)
	}

	expanded, err = filepath.Abs(expanded)
	if err != nil {
		panic(err)
	}

	return expanded
}

func readJson(file string) map[string]map[string]interface{} {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		panic(fmt.Errorf("Error reading template file: %s", err))
	}

	steps := make(map[string]map[string]interface{})

	err = json.Unmarshal(content, &steps)
	if err != nil {
		panic(fmt.Errorf("Error parsing template file: %s", err))
	}

	return steps
}
