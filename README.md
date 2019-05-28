[![Go Report Card](https://goreportcard.com/badge/cisco-ie/pipeline-gnmi)](https://goreportcard.com/report/cisco-ie/pipeline-gnmi)

# Pipeline with GNMI and improved Docker support

This is an improved version of the open-source tool pipeline for telemetry consumption.
The original README can be found here: [README-PIPELINE.md](README-PIPELINE.md)


## GNMI support

This version of pipeline introduces support for [GNMI](https://github.com/openconfig/reference/tree/master/rpc/gnmi).
GNMI is a standardized and cross-platform protocol for network management and telemetry. GNMI does not require prior sensor path configuration on the target device, merely enabling gRPC/gNMI is enough. Sensor paths are requested by the collector (e.g. pipeline). Subscription type (interval, on-change, target-defined) can be specified per path.

Filtering of retrieved sensor values can be done directly at the input stage through selectors in the configuration file,
by defining all the sensor paths that should be stored in a TSDB or forwarded via Kafka. Regular metrics filtering through metrics.json files is ignored and not implemented, due to the lack of user-friendliness of the configuration.

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

## Kafka 2.x Support

This version of Pipeline supports Kafka 2.x by requiring the Kafka version to be specified in the config file. This is a requirement of the underlying Kafka library and ensures that the library is communicating with the Kafka brokers effectively.

## Improved Docker support

This version of Pipeline has proper Docker support. The Dockerfile is now using Alpine (for decreased overhead) and
builds Pipeline from scratch. Additionally the configuration file can now be created from environment variables directly,
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

Additionally existing replays of sensor data can be fed in efficiently using xz-compressed files.


## Updated dependencies and improved builds

Dependencies in the vendor directory have been updated and source-code has been adapted accordingly.
Additionally builds are now statically linked (independent of build environment or userland) and stripped for size.