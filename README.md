# pipeline-gnmi [![Go Report Card](https://goreportcard.com/badge/cisco-ie/pipeline-gnmi)](https://goreportcard.com/report/cisco-ie/pipeline-gnmi) [![Build Status](https://travis-ci.org/cisco-ie/pipeline-gnmi.svg?branch=master)](https://travis-ci.org/cisco-ie/pipeline-gnmi)

> A streamlined Model-Driven Telemetry collector based on the open-source tool [`pipeline`](https://github.com/cisco/bigmuddy-network-telemetry-pipeline) including enhancements and bug fixes. 

`pipeline-gnmi` is a Model-Driven Telemetry (MDT) collector based on the open-source tool [`pipeline`](https://github.com/cisco/bigmuddy-network-telemetry-pipeline) which has a refreshed codebase improving maintainability, performance, and modern compatibility. It supports MDT from IOS XE, IOS XR, and NX-OS, enabling DIY operators with cross-platform Cisco MDT collection.

The original `pipeline` README is included [here](README-PIPELINE.md) for reference.

## Installation
1) `pipeline-gnmi` can be downloaded from [Releases](https://github.com/cisco-ie/pipeline-gnmi/releases)
2) Built from source:
```bash
git clone https://github.com/cisco-ie/pipeline-gnmi
cd pipeline-gnmi
make build
```
3) Acquired via `go get github.com/cisco-ie/pipeline-gnmi` to be located in `$GOPATH/bin`

## Configuration
`pipeline` configuration support is maintained and detailed in the [original README](README-PIPELINE.md). Sample configuration is supplied as [pipeline.conf](pipeline.conf).

### gNMI Support
This version of pipeline introduces support for [gNMI](https://github.com/openconfig/reference/tree/master/rpc/gnmi).
gNMI is a standardized and cross-platform protocol for network management and telemetry. gNMI does not require prior sensor path configuration on the target device, merely enabling gRPC/gNMI is enough. Sensor paths are requested by the collector (e.g. pipeline). Subscription type (interval, on-change, target-defined) can be specified per path.

Filtering of retrieved sensor values can be done directly at the input stage through selectors in the configuration file,
by defining all the sensor paths that should be stored in a TSDB or forwarded via Kafka. **Regular metrics filtering through metrics.json files is ignored and not implemented**, due to the lack of user-friendliness of the configuration.

```
[mygnmirouter]
stage = xport_input
type = gnmi
server = 10.49.234.114:57777

# Sensor Path to subscribe to. No configuration on the device necessary
# Appending an @ with a parameter specifies subscription type:
#   @x where x is a positive number indicates a fixed interval, e.g. @10 -> every 10 seconds
#   @change indicates only changes should be reported
#   omitting @ and parameter will do a target-specific subscriptions (not universally supported)
#
path1 = Cisco-IOS-XR-infra-statsd-oper:infra-statistics/interfaces/interface/latest/generic-counters@10
#path2 = /interfaces/interface/state@change

# Whitelist the actual sensor values we are interested in (1 per line) and drop the rest.
# This replaces metrics-based filtering for gNMI input - which is not implemented.
# Note: Specifying one or more selectors will drop all other sensor values and is applied for all paths.
#select1 = Cisco-IOS-XR-infra-statsd-oper:infra-statistics/interfaces/interface/latest/generic-counters/packets-sent
#select2 = Cisco-IOS-XR-infra-statsd-oper:infra-statistics/interfaces/interface/latest/generic-counters/packets-received

# Suppress redundant messages (minimum hearbeat interval)
# If set and 0 or positive, redundant messages should be suppressed by the server
# If greater than 0, the number of seconds after which a measurement should be sent, even if no change has occured
#heartbeat_interval = 0

tls = false
username = cisco
password = ...
```

### Kafka 2.x Support
This version of Pipeline supports Kafka 2.x by requiring the Kafka version to be specified in the config file. This is a requirement of the underlying Kafka library and ensures that the library is communicating with the Kafka brokers effectively.

```
[kafkaconsumer]
topic=mdt
consumergroup=pipeline-gnmi
type=kafka
stage=xport_input
brokers=kafka-host:9092
encoding=gpb
datachanneldepth=1000
kafkaversion=2.1.0
```

### Docker Environment Variables

This version of `pipeline` has improved Docker support. The Dockerfile uses multi-stage builds and
builds Pipeline from scratch. The configuration file can now be created from environment variables directly,
e.g.

```
PIPELINE_default_id=pipeline
PIPELINE_mygnmirouter_stage=xport_input
PIPELINE_mygnmirouter_type=gnmi
```

is translated into a pipeline.conf with following contents:
```
[default]
id = pipeline

[mygnmirouter]
stage = xport_input
type = gnmi
```

If the special variable *_password* is used, the value is encrypted using the pipeline RSA key before being written to
the *password* option. Similarly *_secret* can be used, then the value is read from the file whose name is given as
value, encrypted using the pipeline RSA key and then written as *password* option. If the Pipeline RSA key is not
given or does not exist it is created upon creation of the container.

Additionally, existing replays of sensor data can be fed in efficiently using xz-compressed files.

## Licensing
`pipeline-gnmi` is licensed with [Apache License, Version 2.0](LICENSE), per `pipeline`.

## Special Thanks
Chris Cassar for implementing `pipeline` used by anyone interested in MDT, and Steven Barth for gNMI plugin development.