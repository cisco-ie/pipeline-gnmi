GOCMD=go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST  = $(GOCMD) test
GOGET   = $(GOCMD) get
GOTOOL  = $(GOCMD) tool
GOBASE := $(shell pwd)
GOBIN  := $(GOBASE)/bin
DOCKER := $(GOBASE)/tools/test

# name of executable.
BINARY = pipeline

include skeleton/pipeline.mk

clean: clean-containers
	@echo "  >  Cleaning binaries and cache"
	@-rm -f $(GOBIN)/$(PROJECTNAME)/$(BINARY)
	@$(GOCLEAN)

clean-containers:
	@echo "  >  Cleaning containers"
	@cd $(DOCKER) && docker-compose kill && docker-compose rm -f

start-containers: clean-containers
	@echo "  >  Starting containers"
	@cd $(DOCKER) && docker-compose up -d

# Backward compatibility
testall: integration-test

# Setup pretest as a prerequisite of tests.
integration-test: pretestinfra

pretestinfra:
	@echo Setting up Zookeeper and Kafka. Docker required.
	@$(MAKE) start-containers
