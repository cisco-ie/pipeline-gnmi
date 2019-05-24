#
#
# March 2017
# Copyright (c) 2017 by cisco Systems, Inc.
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
# Cumbersome way of excluding vendor directory
GO_BAR_VENDOR := $(shell go list ./... | egrep -v vendor/)

.DEFAULT_GOAL: bin/$(BINARY)

# Build binary in bin directory
bin/$(BINARY): $(SOURCES)
	@mkdir -p bin
	go vet $(GO_BAR_VENDOR)
	go fmt $(GO_BAR_VENDOR)
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY)

.PHONY: generated-source
generated-source:
	go generate -x

.PHONY: integration-test
integration-test:
	@echo Starting Integration tests
	$(GOTEST) -v -coverpkg=./... -tags=integration $(COVER_PROFILE) ./...

## Unit tests
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





