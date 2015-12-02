SHELL := /usr/bin/env bash

test:
	go test

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
	test
