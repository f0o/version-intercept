// enforce.go provides the framework-agnostic microversion enforcement engine
// shared by the ConnectRPC and gRPC adapter modules. It contains no ConnectRPC
// or gRPC imports: it operates purely on protobuf descriptors and messages plus
// the negotiated Version, and reports outcomes via a neutral EnforcementError so
// each adapter can map them to its own status codes.
package router

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// DeprecationMetadataKey is the response header/metadata key adapters set to
// "true" when the invoked method is deprecated at the negotiated microversion.
// It matches the HTTP "Deprecation" header (lowercased for gRPC metadata).
const DeprecationMetadataKey = "deprecation"

// EnforcementCode is a framework-neutral result code produced by the enforcement
// engine. Adapters translate it into connect.Code / gRPC codes.Code.
type EnforcementCode int

const (
	// CodeOK indicates the call is permitted.
	CodeOK EnforcementCode = iota
	// CodeUnimplemented indicates the method is outside its lifecycle at the
	// negotiated microversion.
	CodeUnimplemented
	// CodeInternal indicates an internal failure (e.g. a recovered panic).
	CodeInternal
)

// EnforcementError is a neutral error carrying an EnforcementCode. Adapters
// convert it into their framework's error type.
type EnforcementError struct {
	Code    EnforcementCode
	Message string
}

// Error implements error.
func (e *EnforcementError) Error() string { return e.Message }

// NewUnimplementedError builds an EnforcementError for a method that is not
// available at the negotiated microversion.
//
// Arguments:
//   - fullMethod: The fully-qualified method name.
//   - v: The negotiated microversion.
//
// Returns the constructed error.
func NewUnimplementedError(fullMethod string, v Version) *EnforcementError {
	return &EnforcementError{
		Code:    CodeUnimplemented,
		Message: "version-intercept: method " + fullMethod + " is not available at microversion " + v.String(),
	}
}

// NewInternalError builds an EnforcementError describing a recovered panic.
//
// Arguments:
//   - recovered: The recovered panic value.
//
// Returns the constructed error.
func NewInternalError(recovered any) *EnforcementError {
	return &EnforcementError{
		Code:    CodeInternal,
		Message: fmt.Sprintf("version-intercept: recovered panic in handler: %v", recovered),
	}
}

// GateMessage clears fields of msg that are not visible at the negotiated
// microversion, mutating msg in place. Non-protobuf or nil values are ignored.
// Use this for messages the caller owns for the duration of the call (unary
// request/response and received streaming messages).
//
// Arguments:
//   - msg: The protobuf message to gate in place.
//   - v: The negotiated microversion.
func GateMessage(msg proto.Message, v Version) {
	if msg == nil {
		return
	}
	gateFields(msg.ProtoReflect(), v)
}

// GateMessageAny gates a value in place when it is a proto.Message; other values
// are ignored. It is a convenience for adapters that receive `any`.
//
// Arguments:
//   - msg: The candidate protobuf message value.
//   - v: The negotiated microversion.
func GateMessageAny(msg any, v Version) {
	pm, ok := msg.(proto.Message)
	if !ok || pm == nil {
		return
	}
	gateFields(pm.ProtoReflect(), v)
}

// GateClone returns a deep copy of msg with fields not visible at v cleared,
// leaving the original untouched. Use this on send paths where the caller may
// retain ownership of the message.
//
// Arguments:
//   - msg: The protobuf message to copy and gate.
//   - v: The negotiated microversion.
//
// Returns the gated clone.
func GateClone(msg proto.Message, v Version) proto.Message {
	clone := proto.Clone(msg)
	gateFields(clone.ProtoReflect(), v)
	return clone
}

// LookupMethodDescriptor resolves a protoreflect.MethodDescriptor from the
// global protobuf registry by fully-qualified method path. It accepts either the
// "/pkg.Service/Method" form (used by gRPC's grpc.UnaryServerInfo.FullMethod) or
// the bare "pkg.Service.Method" form. It is intended for adapters (such as raw
// gRPC) where the method descriptor is not otherwise attached to the call.
//
// Arguments:
//   - fullMethod: The method path, e.g. "/greeter.v1.GreeterService/Hello".
//
// Returns the method descriptor and true when found; false otherwise.
func LookupMethodDescriptor(fullMethod string) (protoreflect.MethodDescriptor, bool) {
	name, ok := methodFullName(fullMethod)
	if !ok {
		return nil, false
	}
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(name)
	if err != nil {
		return nil, false
	}
	md, ok := desc.(protoreflect.MethodDescriptor)
	if !ok {
		return nil, false
	}
	return md, true
}

// methodFullName normalizes a method path into a protoreflect.FullName. It
// accepts the slashed gRPC form "/pkg.Service/Method" and the bare dotted form
// "pkg.Service.Method".
//
// Arguments:
//   - fullMethod: The method path in either supported form.
//
// Returns the fully-qualified name and true when it can be parsed.
func methodFullName(fullMethod string) (protoreflect.FullName, bool) {
	if service, method := splitProcedure(fullMethod); service != "unknown" && method != "unknown" {
		return protoreflect.FullName(service + "." + method), true
	}
	name := protoreflect.FullName(fullMethod)
	if name.IsValid() && name.Parent() != "" {
		return name, true
	}
	return "", false
}

// WithNegotiatedVersion returns a context carrying the negotiated microversion.
// Adapters use it after negotiating the version from request headers/metadata so
// downstream handlers (and NegotiatedVersion) can read it.
//
// Arguments:
//   - ctx: The parent context.
//   - v: The negotiated microversion.
//
// Returns the derived context.
func WithNegotiatedVersion(ctx context.Context, v Version) context.Context {
	return withNegotiatedVersion(ctx, v)
}
