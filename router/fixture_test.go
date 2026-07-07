// fixture_test.go builds a self-contained protobuf fixture at runtime for the
// router package tests. It constructs a greeter-like service and messages that
// carry the versionintercept microversion annotations (method lifecycle + field
// visibility) using dynamic descriptors, so the library's tests do not depend on
// any generated example code.
package router

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	annotationsv1 "go.f0o.dev/version-intercept/gen/versionintercept/annotations/v1"
)

// fixtureFile is the lazily-built fixture file descriptor shared by tests.
var fixtureFileDescriptor protoreflect.FileDescriptor

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

// buildFixtureFile constructs the fixture FileDescriptor. The service exposes
// Hello (introduced 1.0) and Farewell (introduced 2.0); HelloRequest has
// name (1.0) plus title/language (2.0); HelloResponse has message (1.0) plus
// served_by (2.0).
func buildFixtureFile(t *testing.T) protoreflect.FileDescriptor {
	t.Helper()
	if fixtureFileDescriptor != nil {
		return fixtureFileDescriptor
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
		t.Fatalf("register annotations file: %v", err)
	}

	fd, err := protodesc.NewFile(fdProto, deps)
	if err != nil {
		t.Fatalf("build fixture file descriptor: %v", err)
	}
	fixtureFileDescriptor = fd
	return fd
}

// fixtureService returns the fixture GreeterService descriptor.
func fixtureService(t *testing.T) protoreflect.ServiceDescriptor {
	t.Helper()
	return buildFixtureFile(t).Services().ByName("GreeterService")
}

// fixtureMethod returns the named method descriptor from the fixture service.
func fixtureMethod(t *testing.T, name string) protoreflect.MethodDescriptor {
	t.Helper()
	return fixtureService(t).Methods().ByName(protoreflect.Name(name))
}

// newFixtureMessage creates a dynamic message for the named fixture message type
// and sets the provided string fields by name.
//
// Arguments:
//   - typeName: The fixture message name, e.g. "HelloRequest".
//   - fields: field name -> string value to set.
//
// Returns the populated dynamic message.
func newFixtureMessage(t *testing.T, typeName string, fields map[string]string) *dynamicpb.Message {
	t.Helper()
	md := buildFixtureFile(t).Messages().ByName(protoreflect.Name(typeName))
	if md == nil {
		t.Fatalf("fixture message %q not found", typeName)
	}
	msg := dynamicpb.NewMessage(md)
	for name, value := range fields {
		fd := md.Fields().ByName(protoreflect.Name(name))
		if fd == nil {
			t.Fatalf("fixture field %q not found on %q", name, typeName)
		}
		msg.Set(fd, protoreflect.ValueOfString(value))
	}
	return msg
}

// fixtureString reads a string field from a dynamic fixture message.
func fixtureString(msg *dynamicpb.Message, name string) string {
	fd := msg.Descriptor().Fields().ByName(protoreflect.Name(name))
	if fd == nil {
		return ""
	}
	return msg.Get(fd).String()
}
