// interceptor_test.go verifies the ConnectRPC autopin client interceptor sets
// the Api-Version header for explicit overrides and omits it when no version can
// be resolved.
package autopinconnect

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	"go.f0o.dev/version-intercept/autopin"
)

// invoke runs the interceptor's WrapUnary against a real client request and
// returns the Api-Version header the interceptor set (if any).
func invoke(t *testing.T, ic connect.Interceptor) string {
	t.Helper()
	var seen string
	next := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		seen = req.Header().Get(autopin.DefaultRequestHeader)
		return nil, nil
	})
	req := connect.NewRequest(structpb.NewStringValue("x"))
	if _, err := ic.WrapUnary(next)(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return seen
}

func TestInterceptorSetsOverrideHeader(t *testing.T) {
	ic := Interceptor(nil, WithServiceToken("greeter"), WithVersion("1.0"))
	if got := invoke(t, ic); got != "greeter 1.0" {
		t.Errorf("Api-Version = %q, want %q", got, "greeter 1.0")
	}
}

func TestInterceptorBareVersionOverride(t *testing.T) {
	ic := Interceptor(nil, WithVersion("2.1"))
	if got := invoke(t, ic); got != "2.1" {
		t.Errorf("Api-Version = %q, want %q", got, "2.1")
	}
}

func TestInterceptorNoVersionOmitsHeader(t *testing.T) {
	ic := Interceptor(nil) // nil descriptor, no override -> no value
	if got := invoke(t, ic); got != "" {
		t.Errorf("Api-Version = %q, want empty", got)
	}
}
