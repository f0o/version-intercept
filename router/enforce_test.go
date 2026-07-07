// enforce_test.go exercises the framework-agnostic enforcement engine
// (enforce.go) directly, without going through the ConnectRPC or gRPC adapters,
// so the core module's own test suite covers the public engine API: message
// gating, descriptor lookup, context propagation, and neutral errors.
package router

import (
	"context"
	"testing"

	"google.golang.org/protobuf/reflect/protoregistry"
)

// TestGateMessageInPlace verifies GateMessage clears fields not visible at the
// negotiated microversion while leaving visible fields intact.
func TestGateMessageInPlace(t *testing.T) {
	msg := newFixtureMessage(t, "HelloRequest", map[string]string{
		"name":     "Ada",
		"title":    "Dr",
		"language": "en",
	})

	GateMessage(msg, MustParseVersion("1.0"))

	if got := fixtureString(msg, "name"); got != "Ada" {
		t.Fatalf("name gated away: got %q, want %q", got, "Ada")
	}
	if got := fixtureString(msg, "title"); got != "" {
		t.Fatalf("title (2.0) not gated at 1.0: got %q", got)
	}
	if got := fixtureString(msg, "language"); got != "" {
		t.Fatalf("language (2.0) not gated at 1.0: got %q", got)
	}
}

// TestGateMessageKeepsNewFieldsAtNewVersion verifies 2.0 fields survive gating
// at the 2.0 microversion.
func TestGateMessageKeepsNewFieldsAtNewVersion(t *testing.T) {
	msg := newFixtureMessage(t, "HelloRequest", map[string]string{
		"name":  "Ada",
		"title": "Dr",
	})

	GateMessage(msg, MustParseVersion("2.0"))

	if got := fixtureString(msg, "title"); got != "Dr" {
		t.Fatalf("title (2.0) gated at 2.0: got %q, want %q", got, "Dr")
	}
}

// TestGateMessageNil verifies GateMessage tolerates a nil message.
func TestGateMessageNil(t *testing.T) {
	GateMessage(nil, MustParseVersion("1.0"))
}

// TestGateMessageAny verifies GateMessageAny gates proto messages and ignores
// non-proto values.
func TestGateMessageAny(t *testing.T) {
	msg := newFixtureMessage(t, "HelloRequest", map[string]string{
		"name":  "Ada",
		"title": "Dr",
	})

	GateMessageAny(msg, MustParseVersion("1.0"))
	if got := fixtureString(msg, "title"); got != "" {
		t.Fatalf("title (2.0) not gated at 1.0 via GateMessageAny: got %q", got)
	}

	GateMessageAny("not a proto message", MustParseVersion("1.0"))
	GateMessageAny(nil, MustParseVersion("1.0"))
}

// TestGateClone verifies GateClone returns a gated copy and leaves the original
// message untouched.
func TestGateClone(t *testing.T) {
	original := newFixtureMessage(t, "HelloResponse", map[string]string{
		"message":   "hi",
		"served_by": "modern",
	})

	clone := GateClone(original, MustParseVersion("1.0"))

	if got := fixtureString(original, "served_by"); got != "modern" {
		t.Fatalf("GateClone mutated the original: served_by=%q", got)
	}

	cloneRefl := clone.ProtoReflect()
	servedBy := cloneRefl.Descriptor().Fields().ByName("served_by")
	if got := cloneRefl.Get(servedBy).String(); got != "" {
		t.Fatalf("served_by (2.0) not gated on clone at 1.0: got %q", got)
	}
}

// TestLookupMethodDescriptor verifies descriptor lookup by both the slashed gRPC
// FullMethod form and the bare dotted form, plus failure cases.
func TestLookupMethodDescriptor(t *testing.T) {
	fd := buildFixtureFile(t)
	if err := protoregistry.GlobalFiles.RegisterFile(fd); err != nil {
		t.Fatalf("register fixture file globally: %v", err)
	}

	if _, ok := LookupMethodDescriptor("/routertest.v1.GreeterService/Hello"); !ok {
		t.Fatal("slashed form lookup failed")
	}
	if _, ok := LookupMethodDescriptor("routertest.v1.GreeterService.Hello"); !ok {
		t.Fatal("dotted form lookup failed")
	}
	if _, ok := LookupMethodDescriptor("routertest.v1.GreeterService.DoesNotExist"); ok {
		t.Fatal("expected lookup miss for unknown method")
	}
	if _, ok := LookupMethodDescriptor(""); ok {
		t.Fatal("expected lookup miss for empty method")
	}
	if _, ok := LookupMethodDescriptor("routertest.v1.HelloRequest"); ok {
		t.Fatal("expected lookup miss when name resolves to a non-method descriptor")
	}
}

// TestWithNegotiatedVersion verifies the negotiated version round-trips through
// the context.
func TestWithNegotiatedVersion(t *testing.T) {
	want := MustParseVersion("2.1")
	ctx := WithNegotiatedVersion(context.Background(), want)

	got, ok := NegotiatedVersion(ctx)
	if !ok {
		t.Fatal("negotiated version not present in context")
	}
	if got != want {
		t.Fatalf("negotiated version mismatch: got %v, want %v", got, want)
	}
}

// TestEnforcementErrors verifies the neutral error constructors carry the
// expected codes and non-empty messages.
func TestEnforcementErrors(t *testing.T) {
	unimpl := NewUnimplementedError("/routertest.v1.GreeterService/Farewell", MustParseVersion("1.0"))
	if unimpl.Code != CodeUnimplemented {
		t.Fatalf("unimplemented code: got %v, want %v", unimpl.Code, CodeUnimplemented)
	}
	if unimpl.Error() == "" {
		t.Fatal("unimplemented error message is empty")
	}

	internal := NewInternalError("boom")
	if internal.Code != CodeInternal {
		t.Fatalf("internal code: got %v, want %v", internal.Code, CodeInternal)
	}
	if internal.Error() == "" {
		t.Fatal("internal error message is empty")
	}
}
