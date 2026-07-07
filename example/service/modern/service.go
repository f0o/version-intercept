// Package modern implements the 2.x behavior of the example Greeter service.
// It honors the title and language fields introduced at microversion 2.0,
// populates the served_by response field, and implements the Farewell method
// that only exists from 2.0 onward.
package modern

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	greeterv1 "go.f0o.dev/version-intercept/example/gen-modern/greeter/v1"
	"go.f0o.dev/version-intercept/router"
)

// Service is the 2.x Greeter implementation.
type Service struct{}

// New returns a new modern Greeter Service.
func New() *Service { return &Service{} }

// greetings maps supported BCP-47 language tags to their greeting word.
var greetings = map[string]string{
	"en": "Hello",
	"de": "Hallo",
	"fr": "Bonjour",
	"es": "Hola",
}

// Hello returns a localized, optionally titled greeting and reports the serving
// microversion via served_by.
//
// Arguments:
//   - ctx: The request context carrying the negotiated microversion.
//   - req: The Connect request wrapping HelloRequest.
//
// Returns the Connect response wrapping HelloResponse.
func (s *Service) Hello(ctx context.Context, req *connect.Request[greeterv1.HelloRequest]) (*connect.Response[greeterv1.HelloResponse], error) {
	name := req.Msg.GetName()
	if name == "" {
		name = "World"
	}
	if title := strings.TrimSpace(req.Msg.GetTitle()); title != "" {
		name = title + " " + name
	}

	greeting, ok := greetings[strings.ToLower(req.Msg.GetLanguage())]
	if !ok {
		greeting = greetings["en"]
	}

	servedBy := "greeter (modern)"
	if v, ok := router.NegotiatedVersion(ctx); ok {
		servedBy = fmt.Sprintf("greeter (modern, negotiated %s)", v)
	}

	return connect.NewResponse(&greeterv1.HelloResponse{
		Message:  fmt.Sprintf("%s, %s!", greeting, name),
		ServedBy: servedBy,
	}), nil
}

// Farewell returns a parting message. It is only reachable at microversion 2.0
// and above; the versioning interceptor rejects earlier callers.
//
// Arguments:
//   - ctx: The request context.
//   - req: The Connect request wrapping FarewellRequest.
//
// Returns the Connect response wrapping FarewellResponse.
func (s *Service) Farewell(ctx context.Context, req *connect.Request[greeterv1.FarewellRequest]) (*connect.Response[greeterv1.FarewellResponse], error) {
	name := req.Msg.GetName()
	if name == "" {
		name = "World"
	}
	return connect.NewResponse(&greeterv1.FarewellResponse{
		Message: fmt.Sprintf("Goodbye, %s!", name),
	}), nil
}

// HelloStream streams repeated greetings, one per requested repeat (default 3).
// It is only reachable at microversion 2.1 and above; the versioning interceptor
// rejects earlier callers and gates the served_by field on each streamed message
// for microversions below 2.0.
//
// Arguments:
//   - ctx: The request context carrying the negotiated microversion.
//   - req: The Connect request wrapping HelloStreamRequest.
//   - stream: The server stream to which HelloStreamResponse messages are sent.
//
// Returns an error if sending a streamed message fails.
func (s *Service) HelloStream(ctx context.Context, req *connect.Request[greeterv1.HelloStreamRequest], stream *connect.ServerStream[greeterv1.HelloStreamResponse]) error {
	name := req.Msg.GetName()
	if name == "" {
		name = "World"
	}
	if title := strings.TrimSpace(req.Msg.GetTitle()); title != "" {
		name = title + " " + name
	}

	greeting, ok := greetings[strings.ToLower(req.Msg.GetLanguage())]
	if !ok {
		greeting = greetings["en"]
	}

	servedBy := "greeter (modern)"
	if v, ok := router.NegotiatedVersion(ctx); ok {
		servedBy = fmt.Sprintf("greeter (modern, negotiated %s)", v)
	}

	repeat := req.Msg.GetRepeat()
	if repeat <= 0 {
		repeat = 3
	}

	for i := int32(1); i <= repeat; i++ {
		msg := &greeterv1.HelloStreamResponse{
			Message:  fmt.Sprintf("%s, %s! (%d/%d)", greeting, name, i, repeat),
			ServedBy: servedBy,
			Sequence: i,
		}
		if err := stream.Send(msg); err != nil {
			return err
		}
	}
	return nil
}
