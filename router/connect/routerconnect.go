// Package routerconnect provides the ConnectRPC adapter for version-intercept:
// a connect.Interceptor that enforces the microversion annotations declared on a
// service's protobuf descriptors. It is a thin adapter over the framework-neutral
// enforcement engine in the core router package, translating engine outcomes to
// connect.Code and reading/writing ConnectRPC headers.
package routerconnect

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.f0o.dev/version-intercept/router"
)

// DeprecationHeader is set on responses when the invoked method is deprecated at
// the negotiated microversion.
const DeprecationHeader = "Deprecation"

// Option configures a VersioningInterceptor at construction time.
type Option func(*versioningInterceptor)

// WithMetrics enables Prometheus instrumentation using the supplied Metrics.
// When omitted the interceptor records no metrics, keeping instrumentation fully
// opt-in.
//
// Arguments:
//   - m: The Metrics to record into. A nil value disables recording.
//
// Returns an Option applying the metrics.
func WithMetrics(m *router.Metrics) Option {
	return func(i *versioningInterceptor) { i.metrics = m }
}

// WithLogger enables structured logging of version-enforcement events via the
// supplied slog.Logger. When omitted, the interceptor logs nothing.
//
// Arguments:
//   - l: The logger to emit events to. A nil value disables logging.
//
// Returns an Option applying the logger.
func WithLogger(l *slog.Logger) Option {
	return func(i *versioningInterceptor) { i.logger = l }
}

// VersioningInterceptor returns a connect.Interceptor that enforces microversion
// annotations. It relies on the negotiated version stored in the request context
// (by the HTTP Router or an upstream handler).
//
// Arguments:
//   - opts: Optional configuration such as WithMetrics and WithLogger.
//
// Returns the configured interceptor.
func VersioningInterceptor(opts ...Option) connect.Interceptor {
	i := &versioningInterceptor{}
	for _, opt := range opts {
		opt(i)
	}
	return i
}

// versioningInterceptor implements connect.Interceptor.
type versioningInterceptor struct {
	metrics *router.Metrics
	logger  *slog.Logger
}

// logRejection emits a debug record when a method is rejected as unavailable.
func (i *versioningInterceptor) logRejection(procedure string, v router.Version) {
	if i.logger == nil {
		return
	}
	i.logger.Debug("version-intercept: method not available at microversion",
		"procedure", procedure, "api_version", v.String())
}

// logPanic emits an error record when a handler panic is recovered.
func (i *versioningInterceptor) logPanic(procedure string, recovered any) {
	if i.logger == nil {
		return
	}
	i.logger.Error("version-intercept: recovered panic in handler",
		"procedure", procedure, "panic", recovered)
}

// WrapUnary enforces microversion annotations around a unary call, recovers from
// handler panics, and records a metric for every request.
func (i *versioningInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (resp connect.AnyResponse, err error) {
		version, ok := router.NegotiatedVersion(ctx)
		apiVersion := "unknown"
		if ok {
			apiVersion = version.String()
		}
		defer func() {
			if r := recover(); r != nil {
				i.logPanic(req.Spec().Procedure, r)
				err = connect.NewError(connect.CodeInternal, errors.New(router.NewInternalError(r).Message))
				resp = nil
			}
			i.metrics.Record(req.Spec().Procedure, apiVersion, resultCode(err))
		}()

		if !ok {
			return next(ctx, req)
		}

		md := methodDescriptor(req.Spec())
		if md == nil {
			return next(ctx, req)
		}

		lifecycle := router.MethodLifecycleOf(md)
		if !lifecycle.Available(version) {
			i.logRejection(req.Spec().Procedure, version)
			return nil, connect.NewError(connect.CodeUnimplemented, errors.New(router.NewUnimplementedError(string(md.FullName()), version).Message))
		}

		router.GateMessageAny(req.Any(), version)

		resp, err = next(ctx, req)
		if err != nil {
			return nil, err
		}
		router.GateMessageAny(resp.Any(), version)
		if lifecycle.IsDeprecated(version) {
			resp.Header().Set(DeprecationHeader, "true")
		}
		return resp, nil
	}
}

// resultCode maps a call error to its Connect result code label, "ok" on nil.
//
// Arguments:
//   - err: The error returned by the handler, or nil on success.
//
// Returns the lowercase Connect code string used as a metric label.
func resultCode(err error) string {
	if err == nil {
		return "ok"
	}
	return connect.CodeOf(err).String()
}

// WrapStreamingClient passes streaming client calls through unchanged. Client
// version selection is handled by the autopin/connect interceptor.
func (i *versioningInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler enforces microversion annotations around a streaming
// handler call: it rejects methods outside their lifecycle, gates fields on
// received and sent messages, sets the deprecation header, and records a metric.
func (i *versioningInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) (err error) {
		version, ok := router.NegotiatedVersion(ctx)
		apiVersion := "unknown"
		if ok {
			apiVersion = version.String()
		}
		defer func() {
			if r := recover(); r != nil {
				i.logPanic(conn.Spec().Procedure, r)
				err = connect.NewError(connect.CodeInternal, errors.New(router.NewInternalError(r).Message))
			}
			i.metrics.Record(conn.Spec().Procedure, apiVersion, resultCode(err))
		}()

		if !ok {
			return next(ctx, conn)
		}

		md := methodDescriptor(conn.Spec())
		if md == nil {
			return next(ctx, conn)
		}

		lifecycle := router.MethodLifecycleOf(md)
		if !lifecycle.Available(version) {
			i.logRejection(conn.Spec().Procedure, version)
			return connect.NewError(connect.CodeUnimplemented, errors.New(router.NewUnimplementedError(string(md.FullName()), version).Message))
		}
		if lifecycle.IsDeprecated(version) {
			conn.ResponseHeader().Set(DeprecationHeader, "true")
		}

		return next(ctx, &gatingHandlerConn{StreamingHandlerConn: conn, version: version})
	}
}

// gatingHandlerConn wraps a StreamingHandlerConn to gate fields on messages
// received from and sent to the client at the negotiated version.
type gatingHandlerConn struct {
	connect.StreamingHandlerConn
	version router.Version
}

// Receive gates fields on the received message in place.
func (c *gatingHandlerConn) Receive(msg any) error {
	if err := c.StreamingHandlerConn.Receive(msg); err != nil {
		return err
	}
	router.GateMessageAny(msg, c.version)
	return nil
}

// Send gates fields on a clone so the caller's value is not mutated.
func (c *gatingHandlerConn) Send(msg any) error {
	if pm, ok := msg.(proto.Message); ok && pm != nil {
		return c.StreamingHandlerConn.Send(router.GateClone(pm, c.version))
	}
	return c.StreamingHandlerConn.Send(msg)
}

// methodDescriptor extracts the protoreflect.MethodDescriptor attached to the
// connect spec via WithSchema, or nil when unavailable.
//
// Arguments:
//   - spec: The connect spec for the current call.
//
// Returns the method descriptor or nil.
func methodDescriptor(spec connect.Spec) protoreflect.MethodDescriptor {
	md, _ := spec.Schema.(protoreflect.MethodDescriptor)
	return md
}
