// server_test.go verifies the gRPC server interceptor: method-lifecycle
// rejection (Unimplemented), field gating, no-version passthrough, and panic
// recovery (Internal). It registers the shared test fixture in the global proto
// registry so LookupMethodDescriptor can resolve it.
package routergrpc

import (
	"context"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	"go.f0o.dev/version-intercept/router"
	"go.f0o.dev/version-intercept/router/testfixture"
)

var registerOnce sync.Once

// registerFixture registers the fixture file in the global registry once, so the
// interceptor's LookupMethodDescriptor(info.FullMethod) can find it.
func registerFixture(t *testing.T) {
	t.Helper()
	registerOnce.Do(func() {
		fd, err := testfixture.File()
		if err != nil {
			t.Fatalf("fixture file: %v", err)
		}
		if err := protoregistry.GlobalFiles.RegisterFile(fd); err != nil {
			t.Fatalf("register fixture globally: %v", err)
		}
	})
}

// ctxWithVersion builds an incoming context carrying the Api-Version metadata.
func ctxWithVersion(v string) context.Context {
	md := metadata.Pairs(metadataKey, "greeter "+v)
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestUnaryRejectsUnavailableMethod(t *testing.T) {
	registerFixture(t)
	ic := UnaryServerInterceptor(WithHeaderConfig(router.DefaultHeaderConfig("greeter")))

	called := false
	handler := func(ctx context.Context, req any) (any, error) {
		called = true
		return nil, nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/routertest.v1.GreeterService/Farewell"}

	_, err := ic(ctxWithVersion("1.0"), nil, info, handler)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("Farewell at 1.0 code = %v, want Unimplemented", status.Code(err))
	}
	if called {
		t.Error("handler should not run for an unavailable method")
	}
}

func TestUnaryGatesRequestAndResponse(t *testing.T) {
	registerFixture(t)
	ic := UnaryServerInterceptor(WithHeaderConfig(router.DefaultHeaderConfig("greeter")))

	reqMsg, _ := testfixture.NewMessage("HelloRequest", map[string]string{"name": "Ada", "title": "Dr."})
	info := &grpc.UnaryServerInfo{FullMethod: "/routertest.v1.GreeterService/Hello"}

	handler := func(ctx context.Context, req any) (any, error) {
		// title (2.0) must be gated at 1.0 before the handler sees it.
		if testfixture.String(reqMsg, "title") != "" {
			t.Errorf("request title not gated at 1.0: %q", testfixture.String(reqMsg, "title"))
		}
		resp, _ := testfixture.NewMessage("HelloResponse", map[string]string{"message": "hi", "served_by": "modern"})
		return resp, nil
	}

	resp, err := ic(ctxWithVersion("1.0"), reqMsg, info, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := testfixture.String(respAsDynamic(t, resp), "served_by"); got != "" {
		t.Errorf("response served_by not gated at 1.0: %q", got)
	}
}

func TestUnaryRecoversPanic(t *testing.T) {
	registerFixture(t)
	ic := UnaryServerInterceptor(WithHeaderConfig(router.DefaultHeaderConfig("greeter")))
	info := &grpc.UnaryServerInfo{FullMethod: "/routertest.v1.GreeterService/Hello"}
	reqMsg, _ := testfixture.NewMessage("HelloRequest", map[string]string{"name": "x"})

	handler := func(ctx context.Context, req any) (any, error) { panic("boom") }

	_, err := ic(ctxWithVersion("2.0"), reqMsg, info, handler)
	if status.Code(err) != codes.Internal {
		t.Fatalf("recovered panic code = %v, want Internal", status.Code(err))
	}
}

func TestUnaryNoMetadataPassesThrough(t *testing.T) {
	registerFixture(t)
	ic := UnaryServerInterceptor(WithHeaderConfig(router.DefaultHeaderConfig("greeter")))
	info := &grpc.UnaryServerInfo{FullMethod: "/routertest.v1.GreeterService/Hello"}

	called := false
	handler := func(ctx context.Context, req any) (any, error) {
		called = true
		return nil, nil
	}
	if _, err := ic(context.Background(), nil, info, handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler should run when no version metadata is present")
	}
}

// respAsDynamic converts the handler's returned message to a dynamic message for
// field inspection.
func respAsDynamic(t *testing.T, resp any) *dynamicpb.Message {
	t.Helper()
	dm, ok := resp.(*dynamicpb.Message)
	if !ok {
		t.Fatalf("response is %T, want *dynamicpb.Message", resp)
	}
	return dm
}
