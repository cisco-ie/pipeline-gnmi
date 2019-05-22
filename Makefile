# name of executable.
BINARY=pipeline

include skeleton/pipeline.mk

# Setup pretest as a prerequisite of tests.
testall: pretestinfra
pretestinfra:
	@echo Setting up Zookeeper and Kafka. Docker required.
	tools/test/run.sh
