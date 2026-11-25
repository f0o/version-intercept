// Package routerconnect provides the ConnectRPC v2 adapter for version-intercept:
// a connect.ServerInterceptor that enforces the microversion annotations
// declared on a service's protobuf descriptors. It is a thin adapter over the
// framework-neutral enforcement engine in the core router package, translating
// engine outcomes to connect.Code and reading/writing ConnectRPC metadata via
// connect.CallInfo.
package routerconnect

import (
	"context"
	"log/slog"

	"connectrpc.com/connect/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.f0o.dev/version-intercept/router"
)

// DeprecationHeader is set on responses when the invoked method is deprecated at
// the negotiated microversion.
const DeprecationHeader = "Deprecation"

// options configures the versioning server interceptor.
type options struct {
	metrics *router.Metrics
	logger  *slog.Logger
}

// Option configures a VersioningInterceptor at construction time.
type Option func(*options)

// WithMetrics enables Prometheus instrumentation using the supplied Metrics.
// When omitted the interceptor records no metrics, keeping instrumentation fully
// opt-in.
//
// Arguments:
//   - m: The Metrics to record into. A nil value disables recording.
//
// Returns an Option applying the metrics.
func WithMetrics(m *router.Metrics) Option {
	return func(o *options) { o.metrics = m }
}

// WithLogger enables structured logging of version-enforcement events via the
// supplied slog.Logger. When omitted, the interceptor logs nothing.
//
// Arguments:
//   - l: The logger to emit events to. A nil value disables logging.
//
// Returns an Option applying the logger.
func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

// VersioningInterceptor returns a connect.ServerInterceptor that enforces
// microversion annotations across both unary and streaming RPCs. It relies on
// the negotiated version stored in the request context (by the HTTP Router or
// an upstream handler), rejects methods outside their lifecycle, gates request
// and response fields, sets the Deprecation response header when appropriate,
// recovers from handler panics, and records opt-in metrics.
//
// Arguments:
//   - opts: Optional configuration such as WithMetrics and WithLogger.
//
// Returns the configured server interceptor.
func VersioningInterceptor(opts ...Option) connect.ServerInterceptor {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) (err error) {
			version, ok := router.NegotiatedVersion(ctx)
			apiVersion := "unknown"
			if ok {
				apiVersion = version.String()
			}
			defer func() {
				if r := recover(); r != nil {
					logPanic(o.logger, spec.Procedure, r)
					err = connect.NewError(connect.CodeInternal, router.NewInternalError(r).Message)
				}
				o.metrics.Record(spec.Procedure, apiVersion, resultCode(err))
			}()

			if !ok {
				return next(ctx, spec, stream)
			}

			md := methodDescriptor(spec)
			if md == nil {
				return next(ctx, spec, stream)
			}

			lifecycle := router.MethodLifecycleOf(md)
			if !lifecycle.Available(version) {
				logRejection(o.logger, spec.Procedure, version)
				return connect.NewError(connect.CodeUnimplemented, router.NewUnimplementedError(string(md.FullName()), version).Message)
			}
			if lifecycle.IsDeprecated(version) {
				if info, hasInfo := connect.CallInfoForServerContext(ctx); hasInfo {
					info.ResponseHeader().Set(DeprecationHeader, "true")
				}
			}

			wrapped := &gatingServerStream{
				ServerStream: stream,
				version:      version,
				cloneOnSend:  spec.StreamType != connect.StreamTypeUnary,
			}
			return next(ctx, spec, wrapped)
		}
	}
}

// logRejection emits a debug record when a method is rejected as unavailable.
//
// Arguments:
//   - logger: The structured logger, or nil to skip logging.
//   - procedure: The RPC procedure path.
//   - v: The negotiated microversion.
func logRejection(logger *slog.Logger, procedure string, v router.Version) {
	if logger == nil {
		return
	}
	logger.Debug("version-intercept: method not available at microversion",
		"procedure", procedure, "api_version", v.String())
}

// logPanic emits an error record when a handler panic is recovered.
//
// Arguments:
//   - logger: The structured logger, or nil to skip logging.
//   - procedure: The RPC procedure path.
//   - recovered: The recovered panic value.
func logPanic(logger *slog.Logger, procedure string, recovered any) {
	if logger == nil {
		return
	}
	logger.Error("version-intercept: recovered panic in handler",
		"procedure", procedure, "panic", recovered)
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

// gatingServerStream wraps a connect.ServerStream to gate fields on messages
// received from and sent to the client at the negotiated version.
type gatingServerStream struct {
	connect.ServerStream
	version     router.Version
	cloneOnSend bool
}

// Receive gates fields on the received message in place.
//
// Arguments:
//   - msg: The destination message populated by the underlying stream.
//
// Returns any error reported by the underlying Receive call.
func (s *gatingServerStream) Receive(msg any) error {
	if err := s.ServerStream.Receive(msg); err != nil {
		return err
	}
	router.GateMessageAny(msg, s.version)
	return nil
}

// Send gates fields on outgoing messages, cloning first for streaming calls so
// the caller's message value is not mutated.
//
// Arguments:
//   - msg: The outgoing message to send.
//
// Returns any error reported by the underlying Send call.
func (s *gatingServerStream) Send(msg any) error {
	if pm, ok := msg.(proto.Message); ok && pm != nil {
		if s.cloneOnSend {
			return s.ServerStream.Send(router.GateClone(pm, s.version))
		}
		router.GateMessage(pm, s.version)
	}
	return s.ServerStream.Send(msg)
}

// methodDescriptor extracts the protoreflect.MethodDescriptor attached to the
// connect spec via Schema, or nil when unavailable.
//
// Arguments:
//   - spec: The connect spec for the current call.
//
// Returns the method descriptor or nil.
func methodDescriptor(spec connect.Spec) protoreflect.MethodDescriptor {
	md, _ := spec.Schema.(protoreflect.MethodDescriptor)
	return md
}
