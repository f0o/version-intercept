// Package routergrpc provides raw gRPC (google.golang.org/grpc) server
// interceptors that enforce version-intercept microversion annotations, mirroring
// the ConnectRPC adapter. It negotiates the microversion from the incoming
// Api-Version metadata, rejects methods outside their lifecycle, gates request
// and response fields, surfaces deprecation via response metadata, records opt-in
// metrics, and recovers from handler panics.
//
// It lives in its own module so google.golang.org/grpc is never pulled into the
// framework-agnostic core.
package routergrpc

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.f0o.dev/version-intercept/router"
)

// metadataKey is the lowercase metadata key carrying the microversion request
// value (gRPC metadata keys are canonicalized to lowercase).
const metadataKey = "api-version"

// options configures the server interceptors.
type options struct {
	header  router.HeaderConfig
	metrics *router.Metrics
}

// Option configures the server interceptors.
type Option func(*options)

// WithHeaderConfig sets the header configuration used to parse the microversion
// metadata value (service token, header name semantics).
//
// Arguments:
//   - cfg: The header configuration.
//
// Returns an Option applying it.
func WithHeaderConfig(cfg router.HeaderConfig) Option {
	return func(o *options) { o.header = cfg }
}

// WithMetrics enables opt-in Prometheus instrumentation.
//
// Arguments:
//   - m: The Metrics to record into. Nil disables recording.
//
// Returns an Option applying it.
func WithMetrics(m *router.Metrics) Option {
	return func(o *options) { o.metrics = m }
}

// newOptions resolves options with defaults.
func newOptions(opts ...Option) options {
	o := options{header: router.DefaultHeaderConfig("")}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// negotiate reads and parses the microversion from incoming metadata.
//
// Arguments:
//   - ctx: The incoming request context.
//   - h: The header configuration used to parse the value.
//
// Returns the negotiated version and true, or false when no version metadata is
// present or it cannot be parsed.
func negotiate(ctx context.Context, h router.HeaderConfig) (router.Version, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return router.Version{}, false
	}
	values := md.Get(metadataKey)
	if len(values) == 0 {
		return router.Version{}, false
	}
	v, latest, err := h.ParseRequestValue(values[0])
	if err != nil || latest {
		return router.Version{}, false
	}
	return v, true
}

// UnaryServerInterceptor returns a grpc.UnaryServerInterceptor that enforces
// microversion annotations for unary calls.
//
// Arguments:
//   - opts: Optional configuration (header config, metrics).
//
// Returns the interceptor.
func UnaryServerInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	o := newOptions(opts...)
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		version, ok := negotiate(ctx, o.header)
		apiVersion := "unknown"
		if ok {
			apiVersion = version.String()
			ctx = router.WithNegotiatedVersion(ctx, version)
		}
		defer func() {
			if r := recover(); r != nil {
				err = status.Errorf(codes.Internal, "version-intercept: recovered panic in handler: %v", r)
				resp = nil
			}
			o.metrics.Record(info.FullMethod, apiVersion, codeLabel(err))
		}()

		if !ok {
			return handler(ctx, req)
		}

		md, found := router.LookupMethodDescriptor(info.FullMethod)
		if !found {
			return handler(ctx, req)
		}

		lifecycle := router.MethodLifecycleOf(md)
		if !lifecycle.Available(version) {
			return nil, status.Error(codes.Unimplemented, "version-intercept: method "+info.FullMethod+" is not available at microversion "+version.String())
		}

		router.GateMessageAny(req, version)

		resp, err = handler(ctx, req)
		if err != nil {
			return nil, err
		}
		router.GateMessageAny(resp, version)
		if lifecycle.IsDeprecated(version) {
			_ = grpc.SetHeader(ctx, metadata.Pairs(router.DeprecationMetadataKey, "true"))
		}
		return resp, nil
	}
}

// StreamServerInterceptor returns a grpc.StreamServerInterceptor that enforces
// microversion annotations for streaming calls: it rejects unavailable methods
// and gates fields on every received and sent message.
//
// Arguments:
//   - opts: Optional configuration (header config, metrics).
//
// Returns the interceptor.
func StreamServerInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	o := newOptions(opts...)
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		ctx := ss.Context()
		version, ok := negotiate(ctx, o.header)
		apiVersion := "unknown"
		if ok {
			apiVersion = version.String()
		}
		defer func() {
			if r := recover(); r != nil {
				err = status.Errorf(codes.Internal, "version-intercept: recovered panic in streaming handler: %v", r)
			}
			o.metrics.Record(info.FullMethod, apiVersion, codeLabel(err))
		}()

		if !ok {
			return handler(srv, ss)
		}

		md, found := router.LookupMethodDescriptor(info.FullMethod)
		if !found {
			return handler(srv, ss)
		}

		lifecycle := router.MethodLifecycleOf(md)
		if !lifecycle.Available(version) {
			return status.Error(codes.Unimplemented, "version-intercept: method "+info.FullMethod+" is not available at microversion "+version.String())
		}
		if lifecycle.IsDeprecated(version) {
			_ = ss.SetHeader(metadata.Pairs(router.DeprecationMetadataKey, "true"))
		}

		return handler(srv, &gatingServerStream{ServerStream: ss, version: version})
	}
}

// gatingServerStream wraps a grpc.ServerStream to gate fields on received and
// sent messages at the negotiated microversion.
type gatingServerStream struct {
	grpc.ServerStream
	version router.Version
}

// RecvMsg gates fields on the received message after the underlying stream
// populates it.
func (s *gatingServerStream) RecvMsg(m any) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}
	router.GateMessageAny(m, s.version)
	return nil
}

// SendMsg gates fields on a clone so the handler's message is not mutated.
func (s *gatingServerStream) SendMsg(m any) error {
	if pm, ok := m.(proto.Message); ok && pm != nil {
		return s.ServerStream.SendMsg(router.GateClone(pm, s.version))
	}
	return s.ServerStream.SendMsg(m)
}

// codeLabel maps a gRPC error to its status code label, "ok" on success.
//
// Arguments:
//   - err: The handler error, or nil on success.
//
// Returns the lowercase status code string used as a metric label.
func codeLabel(err error) string {
	if err == nil {
		return "ok"
	}
	return fmt.Sprintf("%v", status.Code(err))
}
