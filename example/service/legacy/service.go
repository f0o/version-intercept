// Package legacy implements the 1.x behavior of the example Greeter service.
// It honors only the fields that existed at microversion 1.0 and returns the
// original greeting format. Newer request fields are already stripped by the
// versioning interceptor before this implementation runs, but it also ignores
// them by construction, illustrating stable legacy behavior.
package legacy

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	greeterv1 "go.f0o.dev/version-intercept/example/gen-modern/greeter/v1"
	"go.f0o.dev/version-intercept/example/gen-modern/greeter/v1/greeterv1connect"
)

// Service is the 1.x Greeter implementation.
type Service struct {
	greeterv1connect.UnimplementedGreeterServiceHandler
}

// New returns a new legacy Greeter Service.
func New() *Service { return &Service{} }

// Hello returns the original plain greeting, ignoring the 2.0 title and language
// fields entirely.
//
// Arguments:
//   - ctx: The request context.
//   - req: The Connect request wrapping HelloRequest.
//
// Returns the Connect response wrapping HelloResponse.
func (s *Service) Hello(ctx context.Context, req *connect.Request[greeterv1.HelloRequest]) (*connect.Response[greeterv1.HelloResponse], error) {
	name := req.Msg.GetName()
	if name == "" {
		name = "World"
	}
	return connect.NewResponse(&greeterv1.HelloResponse{
		Message: fmt.Sprintf("Hello, %s", name),
	}), nil
}
