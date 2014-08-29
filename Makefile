SHELL := /usr/bin/env bash

test:
	go test -cover

.PHONY: \
	test
