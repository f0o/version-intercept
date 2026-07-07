// Package grpcimpl provides a raw-gRPC (google.golang.org/grpc) implementation
// of the example Greeter service. It mirrors the modern (2.x) behavior; the
// routergrpc server interceptors enforce microversion method lifecycles and gate
// request/response fields based on the negotiated Api-Version metadata, so a
// single implementation serves every microversion.
package grpcimpl

import (
	"context"
	"fmt"
	"strings"

	greeterv1 "go.f0o.dev/version-intercept/example/gen-modern/greeter/v1"
	"go.f0o.dev/version-intercept/router"
)

// Service is the raw-gRPC Greeter implementation.
type Service struct {
	greeterv1.UnimplementedGreeterServiceServer
}

// New returns a new gRPC Greeter Service.
func New() *Service { return &Service{} }

// greetings maps supported language tags to their greeting word.
var greetings = map[string]string{
	"en": "Hello",
	"de": "Hallo",
	"fr": "Bonjour",
	"es": "Hola",
}

// servedByLabel renders the serving description, including the negotiated
// microversion when the routergrpc interceptor stored it in the context.
//
// Arguments:
//   - ctx: The request context.
//
// Returns the served_by label.
func servedByLabel(ctx context.Context) string {
	if v, ok := router.NegotiatedVersion(ctx); ok {
		return fmt.Sprintf("greeter (grpc, negotiated %s)", v)
	}
	return "greeter (grpc)"
}

// greetingFor resolves the localized greeting word for a language tag, falling
// back to English for unknown tags.
//
// Arguments:
//   - language: The requested language tag.
//
// Returns the greeting word.
func greetingFor(language string) string {
	if g, ok := greetings[strings.ToLower(language)]; ok {
		return g
	}
	return greetings["en"]
}

// nameWithTitle prefixes a non-empty title to the name.
//
// Arguments:
//   - name: The base name (defaults to "World" when empty).
//   - title: The optional title (gated away below microversion 2.0).
//
// Returns the composed display name.
func nameWithTitle(name, title string) string {
	if name == "" {
		name = "World"
	}
	if t := strings.TrimSpace(title); t != "" {
		return t + " " + name
	}
	return name
}

// Hello returns a localized, optionally titled greeting and reports the serving
// microversion via served_by. Fields not visible at the negotiated microversion
// are gated by the interceptor before this method runs.
//
// Arguments:
//   - ctx: The request context carrying the negotiated microversion.
//   - req: The HelloRequest.
//
// Returns the HelloResponse.
func (s *Service) Hello(ctx context.Context, req *greeterv1.HelloRequest) (*greeterv1.HelloResponse, error) {
	name := nameWithTitle(req.GetName(), req.GetTitle())
	return &greeterv1.HelloResponse{
		Message:  fmt.Sprintf("%s, %s!", greetingFor(req.GetLanguage()), name),
		ServedBy: servedByLabel(ctx),
	}, nil
}

// Farewell returns a parting message. It is only reachable at microversion 2.0
// and above; the interceptor rejects earlier callers with Unimplemented.
//
// Arguments:
//   - ctx: The request context.
//   - req: The FarewellRequest.
//
// Returns the FarewellResponse.
func (s *Service) Farewell(ctx context.Context, req *greeterv1.FarewellRequest) (*greeterv1.FarewellResponse, error) {
	name := req.GetName()
	if name == "" {
		name = "World"
	}
	return &greeterv1.FarewellResponse{Message: fmt.Sprintf("Goodbye, %s!", name)}, nil
}

// HelloStream streams repeated greetings, one per requested repeat (default 3).
// It is only reachable at microversion 2.1 and above; the interceptor rejects
// earlier callers and gates the served_by field on each streamed message below
// microversion 2.0.
//
// Arguments:
//   - req: The HelloStreamRequest.
//   - stream: The server stream to send HelloStreamResponse messages on.
//
// Returns an error if sending fails.
func (s *Service) HelloStream(req *greeterv1.HelloStreamRequest, stream greeterv1.GreeterService_HelloStreamServer) error {
	name := nameWithTitle(req.GetName(), req.GetTitle())
	label := servedByLabel(stream.Context())
	greeting := greetingFor(req.GetLanguage())

	repeat := req.GetRepeat()
	if repeat <= 0 {
		repeat = 3
	}
	for i := int32(1); i <= repeat; i++ {
		msg := &greeterv1.HelloStreamResponse{
			Message:  fmt.Sprintf("%s, %s! (%d/%d)", greeting, name, i, repeat),
			ServedBy: label,
			Sequence: i,
		}
		if err := stream.Send(msg); err != nil {
			return err
		}
	}
	return nil
}
