SHELL := /usr/bin/env bash

test-examples:
	tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	while IFS= read -r -d '' file; do \
		go build -o "$$tmp/$$(basename "$$(dirname "$$file")")" "$$file"; \
	done < <(find ./examples -name '*.go' -print0)

test-package:
	go test -v -coverprofile=coverage.out -covermode=atomic .

test: test-package test-examples

release:
	#$(MAKE) build
	$(MAKE) test
	git diff --quiet HEAD || (echo "--> Please first commit your work" && false)
	./scripts/bump.sh ./version.go $(bump)
	git commit ./version.go -m "Release $$(./scripts/bump.sh ./version.go)"
	git tag $$(./scripts/bump.sh ./version.go)
	git push --tags || true

.PHONY: \
	release \
	test \
	test-package \
	test-examples
