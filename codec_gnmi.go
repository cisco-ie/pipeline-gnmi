//
// Copyright (c) 2018 Cisco Systems
//
// Author: Steven Barth <stbarth@cisco.com>
//
package main

import (
	"fmt"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/openconfig/gnmi/proto/gnmi"
)

type dataMsgGNMI struct {
	original         []byte
	source           msgproducer
	notification     *gnmi.Notification
	jsonUInt64Compat bool
}

func decodeGNMIPath(path *gnmi.Path) string {
	var builder strings.Builder

	if len(path.Origin) > 0 {
		builder.WriteString(path.Origin)
		builder.WriteString(":")
	} else if len(path.Elem) > 0 {
		builder.WriteString("/")
	}

	for i, elem := range path.Elem {
		builder.WriteString(elem.Name)
		if i < len(path.Elem)-1 {
			builder.WriteString("/")
		}
	}
	return builder.String()
}

func (msg *dataMsgGNMI) getDataMsgDescription() string {
	_, id := msg.getMetaDataIdentifier()
	return fmt.Sprintf("gnmi message [%s msg len %d]", id, len(msg.original))
}

func (msg *dataMsgGNMI) produceByteStream(streamSpec *dataMsgStreamSpec) (error, []byte) {
	switch streamSpec.streamType {
	case dMStreamMsgDefault, dMStreamGPB:
		return nil, msg.original

	case dMStreamJSON:
		marshaler := jsonpb.Marshaler{
			EmitUInt64Unquoted: msg.jsonUInt64Compat,
			OrigName:           true,
		}
		json, err := marshaler.MarshalToString(msg.notification)
		return err, []byte(json)
	}

	//
	// We only support producing stream in JSON for this message
	// for the moment - this is because we have the encoded
	// variant at hand.
	return fmt.Errorf("gnmi codec: reformat msg to [%s] is"+
		" not supported", dataMsgStreamTypeString(streamSpec.streamType)), nil
}

func (msg *dataMsgGNMI) produceMetrics(spec *metricsSpec, handler metricsOutputHandler, context metricsOutputContext) error {
	written := false
	timestamp := uint64(msg.notification.Timestamp / 1000000)
	builder := strings.Builder{}

	if len(msg.notification.Prefix.Origin) > 0 {
		builder.WriteString(msg.notification.Prefix.Origin)
		builder.WriteString(":")
	} else {
		builder.WriteString("/")
	}

	tags := make([]metricsAtom, 3)
	tags[0].key = "EncodingPath"
	tags[1].key = "Producer"
	tags[1].val = msg.source.String()
	tags[2].key = "Target"
	tags[2].val = msg.notification.Prefix.Target

	// Parse generic keys from prefix
	for _, elem := range msg.notification.Prefix.Elem {
		builder.WriteString(elem.Name)
		builder.WriteString("/")
		for key, val := range elem.Key {
			tags = append(tags, metricsAtom{
				key: builder.String() + key,
				val: val,
			})
		}
	}

	path := builder.String()
	tags[0].val = path[:len(path)-1]

	for _, update := range msg.notification.Update {
		metricTags := tags
		builder = strings.Builder{}
		builder.WriteString(path)

		for i, elem := range update.Path.Elem {
			builder.WriteString(elem.Name)

			if i < len(update.Path.Elem)-1 {
				builder.WriteString("/")
			}

			for key, val := range elem.Key {
				metricTags = append(metricTags, metricsAtom{
					key: builder.String() + key,
					val: val,
				})
			}
		}

		var val interface{}
		value := update.Val

		switch value.Value.(type) {
		case *gnmi.TypedValue_AsciiVal:
			val = value.GetAsciiVal()
		case *gnmi.TypedValue_BoolVal:
			val = value.GetBoolVal()
		case *gnmi.TypedValue_BytesVal:
			val = value.GetBytesVal()
		case *gnmi.TypedValue_DecimalVal:
			val = value.GetDecimalVal()
		case *gnmi.TypedValue_FloatVal:
			val = value.GetFloatVal()
		case *gnmi.TypedValue_IntVal:
			val = value.GetIntVal()
		case *gnmi.TypedValue_StringVal:
			val = value.GetStringVal()
		case *gnmi.TypedValue_UintVal:
			val = value.GetUintVal()
		default:
			val = nil
		}

		if val != nil {
			handler.buildMetric(
				metricTags,
				metricsAtom{
					key: builder.String(),
					val: val,
				},
				timestamp,
				context)
			written = true
		}
	}

	if written {
		handler.flushMetric(tags, timestamp, context)
	}

	return nil
}

func (msg *dataMsgGNMI) getDataMsgStreamType() dataMsgStreamType {
	return dMStreamMsgDefault
}

func (msg *dataMsgGNMI) getMetaDataPath() (error, string) {
	return nil, decodeGNMIPath(msg.notification.Prefix)
}

func (msg *dataMsgGNMI) getMetaDataIdentifier() (error, string) {
	return nil, msg.source.String()
}

func (msg *dataMsgGNMI) getMetaData() *dataMsgMetaData {
	_, path := msg.getMetaDataPath()
	return &dataMsgMetaData{
		Path:       path,
		Identifier: msg.source.String(),
	}
}

type codecGNMI struct {
	name string
}

//
// Produce a JSON type codec
func getNewCodecGNMI(name string) (error, codecGNMI) {
	codec := codecGNMI{
		name: name,
	}

	return nil, codec
}

func (codec *codecGNMI) dataMsgToBlock(dM dataMsg) (error, []byte) {
	return fmt.Errorf("gnmi: only decoding is supported currently"),
		nil
}

func (codec *codecGNMI) notificationToDataMsgs(source *gnmiClient, msg *gnmi.Notification) ([]dataMsg, error) {
	dMs := make([]dataMsg, 1)
	block, err := proto.Marshal(msg)

	if err == nil {
		dMs[0] = &dataMsgGNMI{
			original:         block,
			source:           source,
			notification:     msg,
			jsonUInt64Compat: source.jsonUInt64Compat,
		}

		//
		// Count the decoded message against the source, section and
		// type
		codecMetaMonitor.Decoded.WithLabelValues(codec.name, source.String(), "gnmi").Inc()
		codecMetaMonitor.DecodedBytes.WithLabelValues(codec.name, source.String(), "gnmi").Add(float64(len(block)))
	}

	return dMs, err
}
