SHELL := /usr/bin/env bash

test:
	go test

build:
	go build -o bin/transloadify transloadify/transloadify.go
	bin/transloadify -h || true

release:
	$(MAKE) build
	$(MAKE) test
	git status && echo "--> Please first commit your work" && false
	./scripts/bump.sh ./VERSION $(bump)
	git commit ./VERSION -m "Release $$(cat VERSION)"
	git tag $$(cat VERSION)
	git push --tags || true
	curl -L http://gobuild.io/github.com/transloadit/go-sdk/transloadify/$$(git describe --tags)/darwin/amd64 -o ./builds/transloadify-darwin-amd64-$$(git describe --tags).zip

install:
	go get ./transloadify/

.PHONY: \
	test \
	build \
	release \
	install
