SHELL := /usr/bin/env bash

test:
	go test

build:
	go build -o bin/transloadify transloadify/transloadify.go
	bin/transloadify -h || true

bump:
	$(MAKE) build
	$(MAKE) test
	git status && echo "--> Please first commit your work" && false
	./scripts/bump.sh ./VERSION $(bump)
	git commit ./VERSION -m "Release $$(cat VERSION)"
	git tag $$(cat VERSION)
	git push --tags || true

release:
	cd build && rm *.zip || true
	wget http://gobuild.io/github.com/transloadit/go-sdk/transloadify/$$(cat VERSION)/darwin/amd64 -O ./builds/transloadify-darwin-amd64-$$(cat VERSION).zip
	cd builds && unzip -o *.zip && rm *.zip
	aws s3 cp --acl public-read builds/transloadify s3://transloadify/transloadify-darwin-amd64-$$(cat VERSION)

install:
	go get ./transloadify/

.PHONY: \
	test \
	build \
	bump \
	release \
	install
