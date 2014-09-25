package main

import (
	"encoding/json"
	"flag"
	"github.com/transloadit/go-sdk"
	"io/ioutil"
	"log"
	"os"
	"text/template"
)

var AuthKey string
var AuthSecret string
var Input string
var Output string
var TemplateId string
var TemplateFile string
var Watch bool
var Preserve bool
var Upstart bool

func init() {
	flag.StringVar(&AuthKey, "key", "", "Auth key")
	flag.StringVar(&AuthSecret, "secret", "", "Auth secret")
	flag.StringVar(&Input, "input", ".", "Input directory")
	flag.StringVar(&Output, "output", "", "Output directory")
	flag.StringVar(&TemplateId, "template", "", "Template's id to create assemblies with")
	flag.StringVar(&TemplateFile, "template-file", "", "Path to local file containing template JSON")
	flag.BoolVar(&Watch, "watch", false, "Watch input directory for changes")
	flag.BoolVar(&Preserve, "preserve", true, "Move input file as original into output directory")
	flag.BoolVar(&Upstart, "upstart", false, "Show an Upstart script for the specified config and exit")
	flag.Parse()

	if env := os.Getenv("TRANSLOADIT_KEY"); AuthKey == "" {
		AuthKey = env
	}

	if env := os.Getenv("TRANSLOADIT_SECRET"); AuthSecret == "" {
		AuthSecret = env
	}
}

func main() {
	if AuthKey == "" {
		log.Fatal("No TRANSLOADIT_KEY defined. Visit https://transloadit.com/accounts/credentials")
	}

	if AuthSecret == "" {
		log.Fatal("No TRANSLOADIT_SECRET defined. Visit https://transloadit.com/accounts/credentials")
	}

	if Upstart {
		upstartFile()
		return
	}

	if Output == "" {
		log.Fatal("No output directory defined")
	}

	if TemplateId == "" && TemplateFile == "" {
		log.Fatal("No template id or template file defined")
	}

	log.Printf("Converting all files in '%s' and putting the result into '%s'.", Input, Output)

	var steps map[string]map[string]interface{}
	if TemplateId != "" {
		log.Printf("Using template with id '%s'.", TemplateId)
	} else if TemplateFile != "" {
		steps = readJson(TemplateFile)
		log.Printf("Using template file '%s' (read %d steps).", TemplateFile, len(steps))
	}

	if Watch {
		log.Printf("Watching directory '%s' for changes...", Input)
	}

	config := transloadit.DefaultConfig
	config.AuthKey = AuthKey
	config.AuthSecret = AuthSecret

	client, err := transloadit.NewClient(&config)
	if err != nil {
		log.Fatal(err)
	}

	options := &transloadit.WatchOptions{
		Input:      Input,
		Output:     Output,
		Watch:      Watch,
		TemplateId: TemplateId,
		Preserve:   Preserve,
		Steps:      steps,
	}
	watcher := client.Watch(options)

	for {
		select {
		case err := <-watcher.Error:
			log.Printf("error: %s", err)
		case file := <-watcher.Change:
			log.Printf("Detected change for '%s'. Starting conversion...", file)
		case info := <-watcher.Done:
			log.Printf("Successfully converted '%s'.", info.Uploads[0].Name)
		}
	}
}

func readJson(file string) map[string]map[string]interface{} {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatalf("Error reading template file: %s", err)
	}

	steps := make(map[string]map[string]interface{})

	err = json.Unmarshal(content, &steps)
	if err != nil {
		log.Fatalf("Error parsing template file: %s", err)
	}

	return steps
}

type DaemonVars struct {
	Unixname string
	Username string
	Cmd      string
	Path     string
	Key      string
	Secret   string
}

func upstartFile() {
	var buf string

	buf = `description {{ .Unixname }}
author      "kvz.io"

start on (local-filesystems and net-device-up IFACE!=lo)
stop on shutdown
respawn
respawn limit 20 5

// Max open files are @ 1024 by default. Bit few.
limit nofile 32768 32768

script
  set -e
  mkfifo /tmp/{{ .Unixname }}-log-fifo
  ( logger -t {{ .Unixname }} </tmp/{{ .Unixname }}-log-fifo & )
  exec >/tmp/{{ .Unixname }}-log-fifo
  rm /tmp/{{ .Unixname }}-log-fifo
  exec bash -c "exec sudo -HEu{{ .Username }} env \
    PATH={{ .Path }} \
    TRANSLOADIT_KEY={{ .Key }} \
    TRANSLOADIT_SECRET={{ .Secret }} \
  {{ .Cmd }} 2>&1"
end script`

	// log.Printf("%s", buf)

	t := template.New("upstart")
	t, _ = t.Parse(buf)
	daemonVars := DaemonVars{
		Unixname: "transloadify",
		Username: os.Getenv("USER"),
		Cmd:      os.Args[0],
		Path:     os.Getenv("PATH"),
		Key:      os.Getenv("TRANSLOADIT_KEY"),
		Secret:   os.Getenv("TRANSLOADIT_SECRET"),
	}

	t.Execute(os.Stdout, daemonVars)
}
