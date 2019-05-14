//
// Copyright (c) 2018 Cisco Systems
//
// Author: Nicolas Leiva <nleiva@cisco.com>
//

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
)

// Marshaler is a jsonpb.Marshaler hack
type Marshaler struct {
	jsonpb.Marshaler
	// Whether to render int64/uint64 as strings or not?
	EmitUInt64Unquoted bool
}

const secondInNanos = int64(time.Second / time.Nanosecond)

type int32Slice []int32

// For sorting extensions ids to ensure stable output.
func (s int32Slice) Len() int           { return len(s) }
func (s int32Slice) Less(i, j int) bool { return s[i] < s[j] }
func (s int32Slice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Writer wrapper inspired by https://blog.golang.org/errors-are-values
type errWriter struct {
	writer io.Writer
	err    error
}

type wkt interface {
	XXX_WellKnownType() string
}

// Map fields may have key types of non-float scalars, strings and enums.
// The easiest way to sort them in some deterministic order is to use fmt.
// If this turns out to be inefficient we can always consider other options,
// such as doing a Schwartzian transform.
//
// Numeric keys are sorted in numeric order per
// https://developers.google.com/protocol-buffers/docs/proto#maps.
type mapKeys []reflect.Value

func (s mapKeys) Len() int      { return len(s) }
func (s mapKeys) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s mapKeys) Less(i, j int) bool {
	if k := s[i].Kind(); k == s[j].Kind() {
		switch k {
		case reflect.String:
			return s[i].String() < s[j].String()
		case reflect.Int32, reflect.Int64:
			return s[i].Int() < s[j].Int()
		case reflect.Uint32, reflect.Uint64:
			return s[i].Uint() < s[j].Uint()
		}
	}
	return fmt.Sprint(s[i].Interface()) < fmt.Sprint(s[j].Interface())
}

func (w *errWriter) write(str string) {
	if w.err != nil {
		return
	}
	_, w.err = w.writer.Write([]byte(str))
}

// marshalValue writes the value to the Writer.
func (m *Marshaler) marshalValue(out *errWriter, prop *proto.Properties, v reflect.Value, indent string) error {
	var err error
	v = reflect.Indirect(v)

	// Handle nil pointer
	if v.Kind() == reflect.Invalid {
		out.write("null")
		return out.err
	}

	// Handle repeated elements.
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() != reflect.Uint8 {
		out.write("[")
		comma := ""
		for i := 0; i < v.Len(); i++ {
			sliceVal := v.Index(i)
			out.write(comma)
			if m.Indent != "" {
				out.write("\n")
				out.write(indent)
				out.write(m.Indent)
				out.write(m.Indent)
			}
			if err := m.marshalValue(out, prop, sliceVal, indent+m.Indent); err != nil {
				return err
			}
			comma = ","
		}
		if m.Indent != "" {
			out.write("\n")
			out.write(indent)
			out.write(m.Indent)
		}
		out.write("]")
		return out.err
	}

	// Handle well-known types.
	// Most are handled up in marshalObject (because 99% are messages).
	if wkt, ok := v.Interface().(wkt); ok {
		switch wkt.XXX_WellKnownType() {
		case "NullValue":
			out.write("null")
			return out.err
		}
	}

	// Handle enumerations.
	if !m.EnumsAsInts && prop.Enum != "" {
		// Unknown enum values will are stringified by the proto library as their
		// value. Such values should _not_ be quoted or they will be interpreted
		// as an enum string instead of their value.
		enumStr := v.Interface().(fmt.Stringer).String()
		var valStr string
		if v.Kind() == reflect.Ptr {
			valStr = strconv.Itoa(int(v.Elem().Int()))
		} else {
			valStr = strconv.Itoa(int(v.Int()))
		}
		isKnownEnum := enumStr != valStr
		if isKnownEnum {
			out.write(`"`)
		}
		out.write(enumStr)
		if isKnownEnum {
			out.write(`"`)
		}
		return out.err
	}

	// Handle nested messages.
	if v.Kind() == reflect.Struct {
		return m.marshalObject(out, v.Addr().Interface().(proto.Message), indent+m.Indent, "")
	}

	// Handle maps.
	// Since Go randomizes map iteration, we sort keys for stable output.
	if v.Kind() == reflect.Map {
		out.write(`{`)
		keys := v.MapKeys()
		sort.Sort(mapKeys(keys))
		for i, k := range keys {
			if i > 0 {
				out.write(`,`)
			}
			if m.Indent != "" {
				out.write("\n")
				out.write(indent)
				out.write(m.Indent)
				out.write(m.Indent)
			}

			// TODO handle map key prop properly
			b, err := json.Marshal(k.Interface())
			if err != nil {
				return err
			}
			s := string(b)

			// If the JSON is not a string value, encode it again to make it one.
			if !strings.HasPrefix(s, `"`) {
				b, err := json.Marshal(s)
				if err != nil {
					return err
				}
				s = string(b)
			}

			out.write(s)
			out.write(`:`)
			if m.Indent != "" {
				out.write(` `)
			}

			vprop := prop
			// Need to uncomment this part. The version of proto.Properties in Pipeline
			// is very old, so MapValProp wasn't there. As soon as the dependencies are
			// brought up to speed, this has to be changed.
			// if prop != nil && prop.MapValProp != nil {
			//	vprop = prop.MapValProp
			// }
			if err := m.marshalValue(out, vprop, v.MapIndex(k), indent+m.Indent); err != nil {
				return err
			}
		}
		if m.Indent != "" {
			out.write("\n")
			out.write(indent)
			out.write(m.Indent)
		}
		out.write(`}`)
		return out.err
	}

	// Handle non-finite floats, e.g. NaN, Infinity and -Infinity.
	if v.Kind() == reflect.Float32 || v.Kind() == reflect.Float64 {
		f := v.Float()
		var sval string
		switch {
		case math.IsInf(f, 1):
			sval = `"Infinity"`
		case math.IsInf(f, -1):
			sval = `"-Infinity"`
		case math.IsNaN(f):
			sval = `"NaN"`
		}
		if sval != "" {
			out.write(sval)
			return out.err
		}
	}

	// Default handling defers to the encoding/json library.
	b, err := json.Marshal(v.Interface())
	if err != nil {
		return err
	}
	needToQuote := !m.EmitUInt64Unquoted && string(b[0]) != `"` && (v.Kind() == reflect.Int64 || v.Kind() == reflect.Uint64)
	if needToQuote {
		out.write(`"`)
	}
	out.write(string(b))
	if needToQuote {
		out.write(`"`)
	}
	return out.err
}

// marshalObject writes a struct to the Writer.
func (m *Marshaler) marshalObject(out *errWriter, v proto.Message, indent, typeURL string) error {
	if jsm, ok := v.(JSONPBMarshaler); ok {
		b, err := jsm.MarshalJSONPB(m)
		if err != nil {
			return err
		}
		if typeURL != "" {
			// we are marshaling this object to an Any type
			var js map[string]*json.RawMessage
			if err = json.Unmarshal(b, &js); err != nil {
				return fmt.Errorf("type %T produced invalid JSON: %v", v, err)
			}
			turl, err := json.Marshal(typeURL)
			if err != nil {
				return fmt.Errorf("failed to marshal type URL %q to JSON: %v", typeURL, err)
			}
			js["@type"] = (*json.RawMessage)(&turl)
			if m.Indent != "" {
				b, err = json.MarshalIndent(js, indent, m.Indent)
			} else {
				b, err = json.Marshal(js)
			}
			if err != nil {
				return err
			}
		}

		out.write(string(b))
		return out.err
	}

	s := reflect.ValueOf(v).Elem()

	// Handle well-known types.
	if wkt, ok := v.(wkt); ok {
		switch wkt.XXX_WellKnownType() {
		case "DoubleValue", "FloatValue", "Int64Value", "UInt64Value",
			"Int32Value", "UInt32Value", "BoolValue", "StringValue", "BytesValue":
			// "Wrappers use the same representation in JSON
			//  as the wrapped primitive type, ..."
			sprop := proto.GetProperties(s.Type())
			return m.marshalValue(out, sprop.Prop[0], s.Field(0), indent)
		case "Any":
			// Any is a bit more involved.
			return m.marshalAny(out, v, indent)
		case "Duration":
			// "Generated output always contains 0, 3, 6, or 9 fractional digits,
			//  depending on required precision."
			s, ns := s.Field(0).Int(), s.Field(1).Int()
			if ns <= -secondInNanos || ns >= secondInNanos {
				return fmt.Errorf("ns out of range (%v, %v)", -secondInNanos, secondInNanos)
			}
			if (s > 0 && ns < 0) || (s < 0 && ns > 0) {
				return errors.New("signs of seconds and nanos do not match")
			}
			if s < 0 {
				ns = -ns
			}
			x := fmt.Sprintf("%d.%09d", s, ns)
			x = strings.TrimSuffix(x, "000")
			x = strings.TrimSuffix(x, "000")
			x = strings.TrimSuffix(x, ".000")
			out.write(`"`)
			out.write(x)
			out.write(`s"`)
			return out.err
		case "Struct", "ListValue":
			// Let marshalValue handle the `Struct.fields` map or the `ListValue.values` slice.
			// TODO: pass the correct Properties if needed.
			return m.marshalValue(out, &proto.Properties{}, s.Field(0), indent)
		case "Timestamp":
			// "RFC 3339, where generated output will always be Z-normalized
			//  and uses 0, 3, 6 or 9 fractional digits."
			s, ns := s.Field(0).Int(), s.Field(1).Int()
			if ns < 0 || ns >= secondInNanos {
				return fmt.Errorf("ns out of range [0, %v)", secondInNanos)
			}
			t := time.Unix(s, ns).UTC()
			// time.RFC3339Nano isn't exactly right (we need to get 3/6/9 fractional digits).
			x := t.Format("2006-01-02T15:04:05.000000000")
			x = strings.TrimSuffix(x, "000")
			x = strings.TrimSuffix(x, "000")
			x = strings.TrimSuffix(x, ".000")
			out.write(`"`)
			out.write(x)
			out.write(`Z"`)
			return out.err
		case "Value":
			// Value has a single oneof.
			kind := s.Field(0)
			if kind.IsNil() {
				// "absence of any variant indicates an error"
				return errors.New("nil Value")
			}
			// oneof -> *T -> T -> T.F
			x := kind.Elem().Elem().Field(0)
			// TODO: pass the correct Properties if needed.
			return m.marshalValue(out, &proto.Properties{}, x, indent)
		}
	}

	out.write("{")
	if m.Indent != "" {
		out.write("\n")
	}

	firstField := true

	if typeURL != "" {
		if err := m.marshalTypeURL(out, indent, typeURL); err != nil {
			return err
		}
		firstField = false
	}

	for i := 0; i < s.NumField(); i++ {
		value := s.Field(i)
		valueField := s.Type().Field(i)
		if strings.HasPrefix(valueField.Name, "XXX_") {
			continue
		}

		// IsNil will panic on most value kinds.
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface:
			if value.IsNil() {
				continue
			}
		}

		if !m.EmitDefaults {
			switch value.Kind() {
			case reflect.Bool:
				if !value.Bool() {
					continue
				}
			case reflect.Int32, reflect.Int64:
				if value.Int() == 0 {
					continue
				}
			case reflect.Uint32, reflect.Uint64:
				if value.Uint() == 0 {
					continue
				}
			case reflect.Float32, reflect.Float64:
				if value.Float() == 0 {
					continue
				}
			case reflect.String:
				if value.Len() == 0 {
					continue
				}
			case reflect.Map, reflect.Ptr, reflect.Slice:
				if value.IsNil() {
					continue
				}
			}
		}

		// Oneof fields need special handling.
		if valueField.Tag.Get("protobuf_oneof") != "" {
			// value is an interface containing &T{real_value}.
			sv := value.Elem().Elem() // interface -> *T -> T
			value = sv.Field(0)
			valueField = sv.Type().Field(0)
		}
		prop := jsonProperties(valueField, m.OrigName)
		if !firstField {
			m.writeSep(out)
		}
		if err := m.marshalField(out, prop, value, indent); err != nil {
			return err
		}
		firstField = false
	}

	// Handle proto2 extensions.
	if ep, ok := v.(proto.Message); ok {
		extensions := proto.RegisteredExtensions(v)
		// Sort extensions for stable output.
		ids := make([]int32, 0, len(extensions))
		for id, desc := range extensions {
			if !proto.HasExtension(ep, desc) {
				continue
			}
			ids = append(ids, id)
		}
		sort.Sort(int32Slice(ids))
		for _, id := range ids {
			desc := extensions[id]
			if desc == nil {
				// unknown extension
				continue
			}
			ext, extErr := proto.GetExtension(ep, desc)
			if extErr != nil {
				return extErr
			}
			value := reflect.ValueOf(ext)
			var prop proto.Properties
			prop.Parse(desc.Tag)
			prop.JSONName = fmt.Sprintf("[%s]", desc.Name)
			if !firstField {
				m.writeSep(out)
			}
			if err := m.marshalField(out, &prop, value, indent); err != nil {
				return err
			}
			firstField = false
		}

	}

	if m.Indent != "" {
		out.write("\n")
		out.write(indent)
	}
	out.write("}")
	return out.err
}

// JSONPBMarshaler is implemented by protobuf messages that customize the
// way they are marshaled to JSON. Messages that implement this should
// also implement JSONPBUnmarshaler so that the custom format can be
// parsed.
//
// The JSON marshaling must follow the proto to JSON specification:
//	https://developers.google.com/protocol-buffers/docs/proto3#json
type JSONPBMarshaler interface {
	MarshalJSONPB(*Marshaler) ([]byte, error)
}

// marshalField writes field description and value to the Writer.
func (m *Marshaler) marshalField(out *errWriter, prop *proto.Properties, v reflect.Value, indent string) error {
	if m.Indent != "" {
		out.write(indent)
		out.write(m.Indent)
	}
	out.write(`"`)
	out.write(prop.JSONName)
	out.write(`":`)
	if m.Indent != "" {
		out.write(" ")
	}
	if err := m.marshalValue(out, prop, v, indent); err != nil {
		return err
	}
	return nil
}

func (m *Marshaler) marshalTypeURL(out *errWriter, indent, typeURL string) error {
	if m.Indent != "" {
		out.write(indent)
		out.write(m.Indent)
	}
	out.write(`"@type":`)
	if m.Indent != "" {
		out.write(" ")
	}
	b, err := json.Marshal(typeURL)
	if err != nil {
		return err
	}
	out.write(string(b))
	return out.err
}

func defaultResolveAny(typeUrl string) (proto.Message, error) {
	// Only the part of typeUrl after the last slash is relevant.
	mname := typeUrl
	if slash := strings.LastIndex(mname, "/"); slash >= 0 {
		mname = mname[slash+1:]
	}
	mt := proto.MessageType(mname)
	if mt == nil {
		return nil, fmt.Errorf("unknown message type %q", mname)
	}
	return reflect.New(mt.Elem()).Interface().(proto.Message), nil
}

// AnyResolver takes a type URL, present in an Any message, and resolves it into
// an instance of the associated message.
type AnyResolver interface {
	Resolve(typeUrl string) (proto.Message, error)
}

func (m *Marshaler) writeSep(out *errWriter) {
	if m.Indent != "" {
		out.write(",\n")
	} else {
		out.write(",")
	}
}

// jsonProperties returns parsed proto.Properties for the field and corrects JSONName attribute.
func jsonProperties(f reflect.StructField, origName bool) *proto.Properties {
	var prop proto.Properties
	prop.Init(f.Type, f.Name, f.Tag.Get("protobuf"), &f)
	if origName || prop.JSONName == "" {
		prop.JSONName = prop.OrigName
	}
	return &prop
}

func (m *Marshaler) marshalAny(out *errWriter, any proto.Message, indent string) error {
	// "If the Any contains a value that has a special JSON mapping,
	//  it will be converted as follows: {"@type": xxx, "value": yyy}.
	//  Otherwise, the value will be converted into a JSON object,
	//  and the "@type" field will be inserted to indicate the actual data type."
	v := reflect.ValueOf(any).Elem()
	turl := v.Field(0).String()
	val := v.Field(1).Bytes()

	var msg proto.Message
	var err error
	if m.AnyResolver != nil {
		msg, err = m.AnyResolver.Resolve(turl)
	} else {
		msg, err = defaultResolveAny(turl)
	}
	if err != nil {
		return err
	}

	if err := proto.Unmarshal(val, msg); err != nil {
		return err
	}

	if _, ok := msg.(wkt); ok {
		out.write("{")
		if m.Indent != "" {
			out.write("\n")
		}
		if err := m.marshalTypeURL(out, indent, turl); err != nil {
			return err
		}
		m.writeSep(out)
		if m.Indent != "" {
			out.write(indent)
			out.write(m.Indent)
			out.write(`"value": `)
		} else {
			out.write(`"value":`)
		}
		if err := m.marshalObject(out, msg, indent+m.Indent, ""); err != nil {
			return err
		}
		if m.Indent != "" {
			out.write("\n")
			out.write(indent)
		}
		out.write("}")
		return out.err
	}

	return m.marshalObject(out, msg, indent, turl)
}
