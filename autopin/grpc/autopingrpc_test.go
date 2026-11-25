// client_test.go verifies the gRPC autopin client interceptor appends the
// Api-Version metadata for explicit overrides and omits it when no version can be
// resolved.
package autopingrpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// capturedMD records the outgoing metadata seen by a fake invoker.
func invokeUnary(t *testing.T, ic grpc.UnaryClientInterceptor) metadata.MD {
	t.Helper()
	var seen metadata.MD
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		seen = md
		return nil
	}
	if err := ic(context.Background(), "/svc/M", nil, nil, nil, invoker); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return seen
}

func TestUnaryClientSetsOverrideMetadata(t *testing.T) {
	ic := UnaryClientInterceptor(nil, WithServiceToken("greeter"), WithVersion("1.0"))
	md := invokeUnary(t, ic)
	if got := md.Get(metadataKey); len(got) != 1 || got[0] != "greeter 1.0" {
		t.Errorf("api-version metadata = %v, want [greeter 1.0]", got)
	}
}

func TestUnaryClientBareVersion(t *testing.T) {
	ic := UnaryClientInterceptor(nil, WithVersion("2.1"))
	md := invokeUnary(t, ic)
	if got := md.Get(metadataKey); len(got) != 1 || got[0] != "2.1" {
		t.Errorf("api-version metadata = %v, want [2.1]", got)
	}
}

func TestUnaryClientNoVersionOmitsMetadata(t *testing.T) {
	ic := UnaryClientInterceptor(nil)
	md := invokeUnary(t, ic)
	if got := md.Get(metadataKey); len(got) != 0 {
		t.Errorf("api-version metadata = %v, want none", got)
	}
}
