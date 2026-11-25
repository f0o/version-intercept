// autopinconnect_test.go verifies the ConnectRPC v2 autopin client interceptor
// sets the Api-Version header on CallInfo for explicit overrides, preserves
// caller-supplied CallInfo, and omits the header when no version can be
// resolved.
package autopinconnect

import (
	"context"
	"testing"

	"connectrpc.com/connect/v2"

	"go.f0o.dev/version-intercept/autopin"
)

func invoke(t *testing.T, ctx context.Context, ic connect.ClientInterceptor) string {
	t.Helper()
	var seen string
	next := connect.ClientFunc(func(callCtx context.Context, spec connect.Spec) (connect.ClientStream, error) {
		if info, ok := connect.CallInfoForClientContext(callCtx); ok {
			seen = info.RequestHeader().Get(autopin.DefaultRequestHeader)
		}
		return nil, nil
	})
	spec := connect.Spec{Procedure: "/svc/Method", StreamType: connect.StreamTypeUnary}
	if _, err := ic(next)(ctx, spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return seen
}

func TestInterceptorSetsOverrideHeader(t *testing.T) {
	ic := Interceptor(nil, WithServiceToken("greeter"), WithVersion("1.0"))
	if got := invoke(t, context.Background(), ic); got != "greeter 1.0" {
		t.Errorf("Api-Version = %q, want %q", got, "greeter 1.0")
	}
}

func TestInterceptorPreservesExistingClientContext(t *testing.T) {
	ic := Interceptor(nil, WithServiceToken("greeter"), WithVersion("1.0"))
	ctx, info := connect.NewClientContext(context.Background())
	info.RequestHeader().Set("X-Trace", "abc")

	if got := invoke(t, ctx, ic); got != "greeter 1.0" {
		t.Errorf("Api-Version = %q, want %q", got, "greeter 1.0")
	}
	if got := info.RequestHeader().Get("X-Trace"); got != "abc" {
		t.Errorf("X-Trace = %q, want %q", got, "abc")
	}
	if got := info.RequestHeader().Get(autopin.DefaultRequestHeader); got != "greeter 1.0" {
		t.Errorf("caller CallInfo Api-Version = %q, want %q", got, "greeter 1.0")
	}
}

func TestInterceptorBareVersionOverride(t *testing.T) {
	ic := Interceptor(nil, WithVersion("2.1"))
	if got := invoke(t, context.Background(), ic); got != "2.1" {
		t.Errorf("Api-Version = %q, want %q", got, "2.1")
	}
}

func TestInterceptorNoVersionOmitsHeader(t *testing.T) {
	ic := Interceptor(nil)
	if got := invoke(t, context.Background(), ic); got != "" {
		t.Errorf("Api-Version = %q, want empty", got)
	}
}
