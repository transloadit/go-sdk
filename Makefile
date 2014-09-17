SHELL := /usr/bin/env bash

test:
	go test

build:
	go build transloadify/transloadify.go

install:
	go get ./transloadify/

.PHONY: \
	test build install
