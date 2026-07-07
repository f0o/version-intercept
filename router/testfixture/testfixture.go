// Package testfixture builds a self-contained protobuf fixture at runtime for
// testing version-intercept adapters. It constructs a greeter-like service and
// messages carrying the versionintercept microversion annotations (method
// lifecycle + field visibility) via dynamic descriptors, so tests in any adapter
// module can exercise enforcement without depending on generated example code.
//
// It is a normal (non-test) package so it can be imported across module
// boundaries (the ConnectRPC and gRPC adapter modules both use it).
package testfixture

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	annotationsv1 "go.f0o.dev/version-intercept/gen/versionintercept/annotations/v1"
)

// fileDescriptor is the lazily-built fixture file descriptor.
var fileDescriptor protoreflect.FileDescriptor

// methodVersioningOptions returns MethodOptions carrying method_versioning.
func methodVersioningOptions(introducedIn string) *descriptorpb.MethodOptions {
	opts := &descriptorpb.MethodOptions{}
	proto.SetExtension(opts, annotationsv1.E_MethodVersioning, &annotationsv1.MethodVersioning{
		IntroducedIn: introducedIn,
	})
	return opts
}

// fieldVersioningOptions returns FieldOptions carrying field_versioning.
func fieldVersioningOptions(introducedIn string) *descriptorpb.FieldOptions {
	opts := &descriptorpb.FieldOptions{}
	proto.SetExtension(opts, annotationsv1.E_FieldVersioning, &annotationsv1.FieldVersioning{
		IntroducedIn: introducedIn,
	})
	return opts
}

// File returns the fixture FileDescriptor, building it once. The service exposes
// Hello (introduced 1.0) and Farewell (introduced 2.0); HelloRequest has name
// (1.0) plus title/language (2.0); HelloResponse has message (1.0) plus
// served_by (2.0).
//
// Returns the fixture file descriptor or an error if construction fails.
func File() (protoreflect.FileDescriptor, error) {
	if fileDescriptor != nil {
		return fileDescriptor, nil
	}

	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()
	optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()

	fdProto := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("routertest/fixture.proto"),
		Package: proto.String("routertest.v1"),
		Syntax:  proto.String("proto3"),
		Dependency: []string{
			"versionintercept/annotations/v1/annotations.proto",
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("HelloRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("name"), Number: proto.Int32(1), Type: strType, Label: optional, JsonName: proto.String("name")},
					{Name: proto.String("title"), Number: proto.Int32(2), Type: strType, Label: optional, JsonName: proto.String("title"), Options: fieldVersioningOptions("2.0")},
					{Name: proto.String("language"), Number: proto.Int32(3), Type: strType, Label: optional, JsonName: proto.String("language"), Options: fieldVersioningOptions("2.0")},
				},
			},
			{
				Name: proto.String("HelloResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("message"), Number: proto.Int32(1), Type: strType, Label: optional, JsonName: proto.String("message")},
					{Name: proto.String("served_by"), Number: proto.Int32(2), Type: strType, Label: optional, JsonName: proto.String("servedBy"), Options: fieldVersioningOptions("2.0")},
				},
			},
			{
				Name: proto.String("FarewellRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("name"), Number: proto.Int32(1), Type: strType, Label: optional, JsonName: proto.String("name")},
				},
			},
			{
				Name: proto.String("FarewellResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("message"), Number: proto.Int32(1), Type: strType, Label: optional, JsonName: proto.String("message")},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("GreeterService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("Hello"),
						InputType:  proto.String(".routertest.v1.HelloRequest"),
						OutputType: proto.String(".routertest.v1.HelloResponse"),
						Options:    methodVersioningOptions("1.0"),
					},
					{
						Name:       proto.String("Farewell"),
						InputType:  proto.String(".routertest.v1.FarewellRequest"),
						OutputType: proto.String(".routertest.v1.FarewellResponse"),
						Options:    methodVersioningOptions("2.0"),
					},
				},
			},
		},
	}

	deps := new(protoregistry.Files)
	annFile := annotationsv1.File_versionintercept_annotations_v1_annotations_proto
	if err := deps.RegisterFile(annFile); err != nil {
		return nil, fmt.Errorf("register annotations file: %w", err)
	}

	fd, err := protodesc.NewFile(fdProto, deps)
	if err != nil {
		return nil, fmt.Errorf("build fixture file descriptor: %w", err)
	}
	fileDescriptor = fd
	return fd, nil
}

// Service returns the fixture GreeterService descriptor.
//
// Returns the service descriptor or an error if the fixture cannot be built.
func Service() (protoreflect.ServiceDescriptor, error) {
	fd, err := File()
	if err != nil {
		return nil, err
	}
	return fd.Services().ByName("GreeterService"), nil
}

// Method returns the named method descriptor from the fixture service.
//
// Arguments:
//   - name: The method name, e.g. "Hello".
//
// Returns the method descriptor or an error.
func Method(name string) (protoreflect.MethodDescriptor, error) {
	svc, err := Service()
	if err != nil {
		return nil, err
	}
	md := svc.Methods().ByName(protoreflect.Name(name))
	if md == nil {
		return nil, fmt.Errorf("fixture method %q not found", name)
	}
	return md, nil
}

// NewMessage creates a dynamic message for the named fixture message type and
// sets the provided string fields by name.
//
// Arguments:
//   - typeName: The fixture message name, e.g. "HelloRequest".
//   - fields: field name -> string value to set.
//
// Returns the populated dynamic message or an error.
func NewMessage(typeName string, fields map[string]string) (*dynamicpb.Message, error) {
	fd, err := File()
	if err != nil {
		return nil, err
	}
	md := fd.Messages().ByName(protoreflect.Name(typeName))
	if md == nil {
		return nil, fmt.Errorf("fixture message %q not found", typeName)
	}
	msg := dynamicpb.NewMessage(md)
	for name, value := range fields {
		f := md.Fields().ByName(protoreflect.Name(name))
		if f == nil {
			return nil, fmt.Errorf("fixture field %q not found on %q", name, typeName)
		}
		msg.Set(f, protoreflect.ValueOfString(value))
	}
	return msg, nil
}

// String reads a string field from a dynamic fixture message, returning "" when
// the field is absent.
//
// Arguments:
//   - msg: The dynamic message to read from.
//   - name: The field name.
//
// Returns the field's string value.
func String(msg *dynamicpb.Message, name string) string {
	f := msg.Descriptor().Fields().ByName(protoreflect.Name(name))
	if f == nil {
		return ""
	}
	return msg.Get(f).String()
}
