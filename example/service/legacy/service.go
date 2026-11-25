// Package legacy implements the 1.x behavior of the example Greeter service.
// It honors only the fields that existed at microversion 1.0 and returns the
// original greeting format. Newer request fields are already stripped by the
// versioning interceptor before this implementation runs, but it also ignores
// them by construction, illustrating stable legacy behavior.
package legacy

import (
	"context"
	"fmt"

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
//   - req: The HelloRequest payload.
//
// Returns the HelloResponse payload.
func (s *Service) Hello(_ context.Context, req *greeterv1.HelloRequest) (*greeterv1.HelloResponse, error) {
	name := req.GetName()
	if name == "" {
		name = "World"
	}
	return &greeterv1.HelloResponse{
		Message: fmt.Sprintf("Hello, %s", name),
	}, nil
}
