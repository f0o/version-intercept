// Package autopinconnect provides a ConnectRPC client interceptor that stamps
// the OpenStack-style Api-Version header on outgoing requests. It mirrors, on the
// client side, the server-side enforcement interceptor: instead of hand-wiring an
// http.RoundTripper, a Connect client adds this interceptor and the microversion
// it was compiled for (read from its own descriptor via the autopin core) is sent
// automatically. An explicit override is available for deliberately requesting a
// different microversion.
package autopinconnect

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.f0o.dev/version-intercept/autopin"
)

// options configures the client interceptor.
type options struct {
	service string
	version string // explicit override; empty means auto-pin from descriptor
}

// Option configures the Interceptor.
type Option func(*options)

// WithServiceToken sets the OpenStack service token prefixed to the version in
// the Api-Version header (e.g. "greeter" -> "greeter 2.1").
//
// Arguments:
//   - service: The service token. Empty produces a bare version value.
//
// Returns an Option applying the token.
func WithServiceToken(service string) Option {
	return func(o *options) { o.service = service }
}

// WithVersion overrides auto-pinning with an explicit microversion, letting a
// client deliberately request a version other than the one it was compiled for.
//
// Arguments:
//   - version: The bare microversion to request, e.g. "1.0".
//
// Returns an Option applying the override.
func WithVersion(version string) Option {
	return func(o *options) { o.version = version }
}

// Interceptor returns a ConnectRPC client interceptor that sets the Api-Version
// request header. When no explicit version is configured, it auto-pins to the
// service_versioning.max annotation baked into the supplied service descriptor.
// When neither an override nor an annotation is available, no header is set and
// the server serves its latest supported version.
//
// Arguments:
//   - sd: The client's own generated service descriptor (used for auto-pinning).
//   - opts: Optional configuration (service token, explicit version override).
//
// Returns the configured interceptor.
func Interceptor(sd protoreflect.ServiceDescriptor, opts ...Option) connect.Interceptor {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	value := headerValue(o, sd)
	return &clientInterceptor{value: value}
}

// headerValue resolves the Api-Version header value from an explicit override or
// by auto-pinning from the descriptor.
//
// Arguments:
//   - o: The resolved options.
//   - sd: The service descriptor for auto-pinning.
//
// Returns the header value, or "" when neither source yields one.
func headerValue(o options, sd protoreflect.ServiceDescriptor) string {
	if o.version != "" {
		if o.service != "" {
			return o.service + " " + o.version
		}
		return o.version
	}
	return autopin.HeaderValue(o.service, sd)
}

// clientInterceptor stamps a fixed Api-Version header on outgoing requests.
type clientInterceptor struct {
	value string
}

// WrapUnary sets the Api-Version header on unary client requests.
func (c *clientInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if c.value != "" {
			req.Header().Set(autopin.DefaultRequestHeader, c.value)
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient sets the Api-Version header on streaming client requests.
func (c *clientInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		if c.value != "" {
			conn.RequestHeader().Set(autopin.DefaultRequestHeader, c.value)
		}
		return conn
	}
}

// WrapStreamingHandler is a pass-through; this is a client interceptor.
func (c *clientInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
