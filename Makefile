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

## Removes containers, images, binaries and cache
clean: clean-containers
	@echo "  >  Cleaning binaries and cache"
	@-rm -f $(GOBIN)/$(PROJECTNAME)/$(BINARY)
	@$(GOCLEAN)

clean-containers:
	@echo "  >  Cleaning containers"
	@cd $(DOCKER) && docker-compose down --rmi all --volumes --remove-orphans 2>/dev/null

start-containers: clean-containers
	@echo "  >  Starting containers"
	@cd $(DOCKER) && docker-compose up -d

# Alias for integration-test
testall: integration-test


## Integration test with Kafka and Zookeper
integration-test: pretestinfra

pretestinfra:
	@echo Setting up Zookeeper and Kafka. Docker required.
	@$(MAKE) start-containers

.DEFAULT:
	@$(MAKE) help

## This help message
.PHONY: help
help:
	@printf "\nUsage\n";

	@awk '{ \
			if ($$0 ~ /^.PHONY: [a-zA-Z\-\_0-9]+$$/) { \
				helpCommand = substr($$0, index($$0, ":") + 2); \
				if (helpMessage) { \
					printf "\033[36m%-20s\033[0m %s\n", \
						helpCommand, helpMessage; \
					helpMessage = ""; \
				} \
			} else if ($$0 ~ /^[a-zA-Z\-\_0-9.]+:/) { \
				helpCommand = substr($$0, 0, index($$0, ":")); \
				if (helpMessage) { \
					printf "\033[36m%-20s\033[0m %s\n", \
						helpCommand, helpMessage; \
					helpMessage = ""; \
				} \
			} else if ($$0 ~ /^##/) { \
				if (helpMessage) { \
					helpMessage = helpMessage"\n                     "substr($$0, 3); \
				} else { \
					helpMessage = substr($$0, 3); \
				} \
			} else { \
				if (helpMessage) { \
					print "\n                     "helpMessage"\n" \
				} \
				helpMessage = ""; \
			} \
		}' \
		$(MAKEFILE_LIST)