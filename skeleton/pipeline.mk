#
#
# March 2017
# Copyright (c) 2017-2019 by cisco Systems, Inc.
# All rights reserved.
#
# Rudimentary build and test support
#
#

VERSION = $(shell git describe --always --long --dirty)
COVER_PROFILE = -coverprofile=coverage.out

PKG = $(shell go list)

# If infra-utils package is not vendored in your workspace, (e.g. you
# are making changes to it, you can simply comment out the VENDOR
# line, and variable update on packages will assume they are under
# source.
VENDOR = $(PKG)/vendor/

LDFLAGS = -ldflags "-X  main.appVersion=v${VERSION}(bigmuddy)"

SOURCEDIR = .
SOURCES := $(shell find $(SOURCEDIR) -name '*.go' -o -name "*.proto" )

# Derived from https://vic.demuzere.be/articles/golang-makefile-crosscompile/
PLATFORMS := linux/amd64 windows/amd64 darwin/amd64
GOPLATFORMTEMP = $(subst /, ,$@)
GOOS = $(word 1, $(GOPLATFORMTEMP))
GOARCH = $(word 2, $(GOPLATFORMTEMP))

.PHONY: $(PLATFORMS)
$(PLATFORMS):
	@echo "  >  Building for ${GOOS}/${GOARCH}"
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOBUILD) $(LDFLAGS) -o $(BINDIR)/$(BINARY)_$(GOOS)_$(GOARCH)

## Build binaries
.PHONY: build
build: hygiene $(PLATFORMS)

## Run Go hygiene tooling like vet and fmt
hygiene:
	@echo "  >  Running Go hygiene tooling"
	go vet -composites=false ./...
	go fmt ./...

.PHONY: generated-source
generated-source:
	go generate -x

## Run unit tests
.PHONY: test
test:
	$(GOTEST) -v $(COVER_PROFILE) ./...

## Displays unit test coverage
.PHONY: coverage
coverage: test
	$(GOTOOL) cover -html=coverage.out

## Displays integration test coverage
.PHONY: integration-coverage
integration-coverage: integration-test
	$(GOTOOL) cover -html=coverage.out





