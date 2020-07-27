SHELL := /usr/bin/env bash

test-examples:
	cd ./examples && find . -type f | xargs -i sh -c "go build {} && go clean" \;

test-package:
	go test -v .

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
