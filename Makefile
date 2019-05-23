# name of executable.
BINARY=pipeline

include skeleton/pipeline.mk


# Backward compatibility
testall: integration-test

# Setup pretest as a prerequisite of tests.
integration-test: pretestinfra

pretestinfra:
	@echo Setting up Zookeeper and Kafka. Docker required.
	cd tools/test && ./run.sh
