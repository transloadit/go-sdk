SHELL := /usr/bin/env bash

test:
	go test

build:
	go build -o bin/transloadify transloadify/transloadify.go
	bin/transloadify -h || true

release: build test
	#@todo Needs logic for semver tagging. Will now just use current tag
	git status && echo "--> Please first commit your work" && false
	git push || true
	curl -L http://gobuild.io/github.com/transloadit/go-sdk/transloadify/$$(git describe --tags)/darwin/amd64 -o transloadify-darwin-amd64-$$(git describe --tags).zip

install:
	go get ./transloadify/

.PHONY: \
	test \
	build \
	release \
	install
