//
// Copyright (c) 2018 Cisco Systems
//
// Author: Steven Barth <stbarth@cisco.com>
//
//
package main

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	gnmiWaitToRedial = 10
	gnmiTimeout      = 10
)

type gnmiClient struct {
	name  string
	codec codecGNMI

	// Log data into debug log
	logData bool

	// Credentials
	tls         bool
	tlsPEM      string
	tlsKey      string
	tlsHostname string

	// Control channel used to control the gNMI client
	ctrlChan <-chan *ctrlMsg

	// Data channels fed by the server
	dataChans []chan<- dataMsg

	// Use to signal that client has shutdown.
	clientDone chan struct{}

	// Setup a consistent log entry to build logging from
	logctx *logrus.Entry

	// Server name
	server string

	paths             []string
	selectors         map[string]struct{}
	heartbeatInterval float64

	// Authentication
	auth userPasswordCollector

	// Compatibility mode for uint64 marshaling
	jsonUInt64Compat bool
}

func (client *gnmiClient) String() string {
	return client.server
}

func (client *gnmiClient) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	username, password, err := client.auth.getUP()
	return map[string]string{
		"username": username,
		"password": password,
	}, err
}

func (client *gnmiClient) RequireTransportSecurity() bool {
	return false
}

func parseGnmiSensorPath(sensorPath string) (string, []*gnmi.PathElem, float64) {
	var interval float64 = -1
	var err error

	sensorParts := strings.SplitN(sensorPath, ":", 2)
	origin := ""
	path := sensorParts[0]
	if len(sensorParts) > 1 {
		origin = sensorParts[0]
		path = sensorParts[1]
	}

	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	pathInterval := strings.SplitN(path, "@", 2)
	path = pathInterval[0]

	if len(pathInterval) > 1 {
		if pathInterval[1] == "change" {
			interval = 0
		} else {
			interval, err = strconv.ParseFloat(pathInterval[1], 64)
			if err != nil {
				interval = -1
			}
		}
	}

	pathElems := make([]*gnmi.PathElem, strings.Count(path, "/")+1)
	filterRegex := regexp.MustCompile(`^([^=]+)='([^']+)'$`)

	for i, elem := range strings.Split(path, "/") {
		pathElems[i] = &gnmi.PathElem{}
		for j, entry := range strings.Split(elem, "[") {
			if j == 0 {
				pathElems[i].Name = entry
			} else if j >= 1 {
				filter := filterRegex.FindStringSubmatch(entry)
				pathElems[i].Key[filter[1]] = filter[2]
			}
		}
	}
	return origin, pathElems, interval
}

//
// Sticky loop when we are handling a remote server, involves trying
// to stay connected and pulling streams for all the subscriptions.
func (client *gnmiClient) loop(ctx context.Context) {
	var err error
	logctx := client.logctx
	defer close(client.clientDone)

	//
	// Prepare dial options (TLS, user/password, timeout...)
	dialOptions := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(client),
		grpc.WithTimeout(time.Second * time.Duration(gnmiTimeout)),
	}

	if client.tls {
		var creds credentials.TransportCredentials
		if client.tlsPEM != "" {
			creds, err = credentials.NewClientTLSFromFile(client.tlsPEM, client.tlsHostname)
			if err != nil {
				logctx.WithError(err).Error("gnmi: failed to load TLS credentials!")
				return
			}
		} else {
			creds = credentials.NewClientTLSFromCert(nil, client.tlsHostname)
		}
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))
	} else {
		dialOptions = append(dialOptions, grpc.WithInsecure())
	}

	conn, err := grpc.DialContext(ctx, client.server, dialOptions...)
	if err != nil {
		logctx.WithError(err).Error("gnmi: dial failed, retrying")
		return
	}
	defer conn.Close()
	logctx.Info("gnmi: Connected")

	// Setup client on connection
	gnmiClient := gnmi.NewGNMIClient(conn)
	subscribeClient, err := gnmiClient.Subscribe(ctx)
	if err != nil {
		logctx.WithError(err).Error("gnmi: subscription setup failed")
		return
	}
	defer subscribeClient.CloseSend()

	var heartbeatInterval uint64
	var suppressRedundant bool

	if client.heartbeatInterval >= 0 {
		suppressRedundant = true
		heartbeatInterval = uint64(1000000000 * client.heartbeatInterval)
	}

	// Create subscription objects
	subscriptions := make([]*gnmi.Subscription, len(client.paths))
	for i, path := range client.paths {
		origin, elems, interval := parseGnmiSensorPath(path)

		subscriptions[i] = &gnmi.Subscription{
			Path: &gnmi.Path{
				Origin: origin,
				Elem:   elems,
			},
			SuppressRedundant: suppressRedundant,
			HeartbeatInterval: heartbeatInterval,
		}

		if interval == 0 {
			subscriptions[i].Mode = gnmi.SubscriptionMode_ON_CHANGE
		} else if interval > 0 {
			subscriptions[i].Mode = gnmi.SubscriptionMode_SAMPLE
			subscriptions[i].SampleInterval = uint64(1000000000 * interval)
		} else {
			subscriptions[i].Mode = gnmi.SubscriptionMode_TARGET_DEFINED
		}
	}

	// Construct subscribe request
	request := &gnmi.SubscribeRequest{
		Request: &gnmi.SubscribeRequest_Subscribe{
			Subscribe: &gnmi.SubscriptionList{
				Prefix:       &gnmi.Path{Target: client.name},
				Mode:         gnmi.SubscriptionList_STREAM,
				Encoding:     gnmi.Encoding_PROTO,
				Subscription: subscriptions,
			},
		},
	}

	err = subscribeClient.Send(request)
	if err != nil {
		logctx.WithError(err).Error("gnmi: subscription failed")
		return
	}

	// Subscription setup, kick off go routine to handle
	// the stream. Add child to wait group such that this
	// routine can wait for all its children on the way
	// out.

	marshaler := jsonpb.Marshaler{Indent: "  "}
	logctx.Info("gnmi: SubscribeClient running")

	for {
		reply, err := subscribeClient.Recv()
		select {
		case <-ctx.Done():
			count := 0
			for {
				if _, err := subscribeClient.Recv(); err != nil {
					logctx.WithFields(logrus.Fields{"count": count}).Info("gnmi: drained on cancellation")
					break
				}
				count++
			}
			return
		default:
			if err != nil {
				logctx.WithError(err).Error("gnmi: server terminated sub")
				return
			}

			if client.logData {
				json, err := marshaler.MarshalToString(reply)
				if err == nil {
					logctx.WithFields(logrus.Fields{"msg": json}).Debug("gnmi: server logdata")
				}
			}

			updateResponse, ok := reply.Response.(*gnmi.SubscribeResponse_Update)
			if !ok {
				continue
			}

			notification := updateResponse.Update

			// Work around XR not honoring target field
			notification.Prefix.Target = client.name

			// Prefilter using selectors
			if len(client.selectors) > 0 {
				prefix := decodeGNMIPath(notification.Prefix)

				k := 0
				for _, update := range notification.Update {
					_, present := client.selectors[prefix + decodeGNMIPath(update.Path)]
					if present {
						notification.Update[k] = update
						k++
					}
				}
				notification.Update = notification.Update[:k]
			}

			dMs, err := client.codec.notificationToDataMsgs(client, notification)
			if err != nil {
				logctx.WithError(err).Error("gnmi: extracting msg from stream failed")
				return
			}

			for _, dM := range dMs {
				//
				// Push data onto channel.
				for _, dataChan := range client.dataChans {
					//
					// Make sure that if
					// we are blocked on
					// consumer, we still
					// handle cancel.
					select {
					case <-ctx.Done():
						return
					case dataChan <- dM:
						continue
					}
				}
			}
		}
	}

	logctx.Info("gnmi: all subscriptions closed")
	return
}

///////////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////
///////                        C O M M O N                      ///////
///////////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////

//
// Run the client/server until shutdown. The role of this outer
// handler is to receive control plane messages and convey them to
// workers, as well as to retry sticky loops if we bail.
// Note: this loop handle dialout and dialin roles.
func (client *gnmiClient) run() {

	var stats msgStats
	var cancel context.CancelFunc
	var ctx context.Context

	username, _, _ := client.auth.getUP()
	logctx := logger.WithFields(logrus.Fields{
		"name":     client.name,
		"type":     "gnmi",
		"server":   client.server,
		"username": username,
	})
	client.logctx = logctx

	go func() {
		// Prime the pump, and kick off the first retry.
		close(client.clientDone)
	}()

	logctx.Info("gNMI starting block")

	for {
		select {

		case <-client.clientDone:

			//
			// If we receive clientDone signal here, we need to retry.
			// Start by making a new channel.
			ctx, cancel = context.WithCancel(context.Background())
			client.clientDone = make(chan struct{})
			go client.loop(ctx)
			//
			// Ensure that if we bailed out right away we
			// do not reschedule immediately.
			time.Sleep(gnmiWaitToRedial * time.Second)

		case msg := <-client.ctrlChan:
			switch msg.id {
			case REPORT:
				content, _ := json.Marshal(stats)
				resp := &ctrlMsg{
					id:       ACK,
					content:  content,
					respChan: nil,
				}
				msg.respChan <- resp

			case SHUTDOWN:
				logctx.WithFields(logrus.Fields{"ctrl_msg_id": msg.id}).
					Info("gnmi: client loop, rxed SHUTDOWN, closing connections")

				//
				// Flag cancellation of binding and
				// its connections and wait for
				// cancellation to complete
				// synchronously.
				if cancel != nil {
					logctx.Debug("gnmi: waiting for children")
					cancel()
				} else {
					logctx.Debug("gnmi: NOT waiting for children")
				}

				logctx.Debug("gnmi: server notify conductor binding is closed")
				resp := &ctrlMsg{
					id:       ACK,
					respChan: nil,
				}
				msg.respChan <- resp
				return

			default:
				logctx.Error("gnmi: block loop, unknown ctrl message")
			}
		}
	}
}

// Module implement inputNodeModule interface
type gnmiInputModule struct {
}

func gnmiInputModuleNew() inputNodeModule {
	return &gnmiInputModule{}
}

//
// Read the module configuration, and set us up to handle control
// requests, receive telemetry and feed the data channels passed in.
func (module *gnmiInputModule) configure(
	name string,
	nc nodeConfig,
	dataChans []chan<- dataMsg) (error, chan<- *ctrlMsg) {

	var err error

	//
	// If not set, will default to false
	logData, _ := nc.config.GetBool(name, "logdata")

	//
	// If not set, will default to false
	tls, _ := nc.config.GetBool(name, "tls")
	tlsPEM, _ := nc.config.GetString(name, "tls_pem")
	tlsHostname, _ := nc.config.GetString(name, "tls_hostname")
	tlsKey, _ := nc.config.GetString(name, "tls_key")

	server, _ := nc.config.GetString(name, "server")
	jsonUInt64Compat, _ := nc.config.GetBool(name, "json_uint64_compat")

	heartbeatInterval, err := nc.config.GetFloat64(name, "heartbeat_interval")
	if err != nil {
		heartbeatInterval = -1
	}

	paths := make([]string, 0)
	selectors := make(map[string]struct{}, 0)

	options, _ := nc.config.GetOptions(name)
	for _, key := range options {
		if strings.HasPrefix(key, "path") {
			path, _ := nc.config.GetString(name, key)
			paths = append(paths, path)
		} else if strings.HasPrefix(key, "select") {
			selector, _ := nc.config.GetString(name, key)
			selectors[selector] = struct{}{}
		}
	}

	//
	// Handle user/password
	authCollect := gnmiUPCollectorFactory()
	err = authCollect.handleConfig(nc, name, server)
	if err != nil {
		return err, nil
	}

	//
	// Create a control channel which will be used to control us,
	// and kick off the server which will accept connections and
	// listen for control requests.
	ctrlChan := make(chan *ctrlMsg)
	_, codec := getNewCodecGNMI(name)

	client := &gnmiClient{
		name:              name,
		codec:             codec,
		auth:              authCollect,
		server:            server,
		logData:           logData,
		ctrlChan:          ctrlChan,
		dataChans:         dataChans,
		clientDone:        make(chan struct{}),
		tls:               tls,
		tlsPEM:            tlsPEM,
		tlsHostname:       tlsHostname,
		tlsKey:            tlsKey,
		heartbeatInterval: heartbeatInterval,
		paths:             paths,
		selectors:         selectors,
		jsonUInt64Compat:  jsonUInt64Compat,
	}
	go client.run()

	return nil, ctrlChan
}

var gnmiUPCollectorFactory userPasswordCollectorFactory

//
// We use init to setup the default user/password collection
// factory. This can be overwritten by test.
func init() {
	gnmiUPCollectorFactory = func() userPasswordCollector {
		return &cryptUserPasswordCollector{}
	}
}
