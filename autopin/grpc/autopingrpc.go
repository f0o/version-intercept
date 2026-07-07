// Package autopingrpc provides raw gRPC (google.golang.org/grpc) client
// interceptors that stamp the OpenStack-style Api-Version metadata on outgoing
// requests, mirroring the ConnectRPC autopin interceptor. The microversion is
// auto-pinned from the client's own service descriptor (via the autopin core) or
// set explicitly for a deliberate override.
//
// It lives in its own module so google.golang.org/grpc is never pulled into the
// framework-agnostic core.
package autopingrpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.f0o.dev/version-intercept/autopin"
)

// metadataKey is the lowercase metadata key carrying the microversion value.
const metadataKey = "api-version"

// options configures the client interceptors.
type options struct {
	service string
	version string // explicit override; empty means auto-pin from descriptor
}

// Option configures the interceptors.
type Option func(*options)

// WithServiceToken sets the service token prefixed to the version value.
//
// Arguments:
//   - service: The service token. Empty produces a bare version value.
//
// Returns an Option applying it.
func WithServiceToken(service string) Option {
	return func(o *options) { o.service = service }
}

// WithVersion overrides auto-pinning with an explicit microversion.
//
// Arguments:
//   - version: The bare microversion to request, e.g. "1.0".
//
// Returns an Option applying it.
func WithVersion(version string) Option {
	return func(o *options) { o.version = version }
}

// resolveValue computes the metadata value from options + descriptor.
func resolveValue(sd protoreflect.ServiceDescriptor, opts ...Option) string {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if o.version != "" {
		if o.service != "" {
			return o.service + " " + o.version
		}
		return o.version
	}
	return autopin.HeaderValue(o.service, sd)
}

// UnaryClientInterceptor returns a grpc.UnaryClientInterceptor that adds the
// Api-Version metadata to outgoing unary calls. When no version can be resolved,
// requests are sent without the metadata (the server serves its latest version).
//
// Arguments:
//   - sd: The client's own generated service descriptor (for auto-pinning).
//   - opts: Optional configuration (service token, explicit version override).
//
// Returns the interceptor.
func UnaryClientInterceptor(sd protoreflect.ServiceDescriptor, opts ...Option) grpc.UnaryClientInterceptor {
	value := resolveValue(sd, opts...)
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, callOpts ...grpc.CallOption) error {
		if value != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, metadataKey, value)
		}
		return invoker(ctx, method, req, reply, cc, callOpts...)
	}
}

// StreamClientInterceptor returns a grpc.StreamClientInterceptor that adds the
// Api-Version metadata to outgoing streaming calls.
//
// Arguments:
//   - sd: The client's own generated service descriptor (for auto-pinning).
//   - opts: Optional configuration (service token, explicit version override).
//
// Returns the interceptor.
func StreamClientInterceptor(sd protoreflect.ServiceDescriptor, opts ...Option) grpc.StreamClientInterceptor {
	value := resolveValue(sd, opts...)
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, callOpts ...grpc.CallOption) (grpc.ClientStream, error) {
		if value != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, metadataKey, value)
		}
		return streamer(ctx, desc, cc, method, callOpts...)
	}
}
