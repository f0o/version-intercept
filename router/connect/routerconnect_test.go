// routerconnect_test.go verifies the ConnectRPC v2 adapter's enforcement: panic
// recovery mapping to CodeInternal, no-version passthrough, and metric/result
// code labeling.
package routerconnect

import (
	"context"
	"testing"

	"connectrpc.com/connect/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/protobuf/proto"

	"go.f0o.dev/version-intercept/router"
	"go.f0o.dev/version-intercept/router/testfixture"
)

func mustMessage(t *testing.T, typeName string, fields map[string]string) proto.Message {
	t.Helper()
	m, err := testfixture.NewMessage(typeName, fields)
	if err != nil {
		t.Fatalf("fixture message: %v", err)
	}
	return m
}

func TestInterceptorRecoversPanic(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := router.NewMetrics(reg)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	ic := VersioningInterceptor(WithMetrics(m))

	next := connect.ServerFunc(func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
		panic("boom")
	})

	ctx := router.WithNegotiatedVersion(context.Background(), router.Version{Major: 1})
	spec := connect.Spec{Procedure: "/svc/Method", StreamType: connect.StreamTypeUnary}
	gotErr := ic(next)(ctx, spec, &fakeServerStream{
		spec:     spec,
		incoming: []proto.Message{mustMessage(t, "HelloRequest", map[string]string{"name": "x"})},
	})

	if gotErr == nil || connect.CodeOf(gotErr) != connect.CodeInternal {
		t.Fatalf("recovered panic = %v, want CodeInternal", gotErr)
	}
	if got := testutil.ToFloat64(m.Collectors()[0].(*prometheus.CounterVec).WithLabelValues("svc", "Method", "1.0", "internal")); got != 1 {
		t.Errorf("panic metric count = %v, want 1", got)
	}
}

func TestInterceptorNoVersionPassesThrough(t *testing.T) {
	ic := VersioningInterceptor()
	called := false
	next := connect.ServerFunc(func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
		called = true
		return nil
	})
	spec := connect.Spec{Procedure: "/svc/Method", StreamType: connect.StreamTypeUnary}
	if err := ic(next)(context.Background(), spec, &fakeServerStream{spec: spec}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("next should be called when no version is negotiated")
	}
}

func TestResultCode(t *testing.T) {
	if got := resultCode(nil); got != "ok" {
		t.Errorf("resultCode(nil) = %q, want ok", got)
	}
	if got := resultCode(connect.NewError(connect.CodeUnimplemented, "not implemented")); got != "unimplemented" {
		t.Errorf("resultCode(unimplemented) = %q, want unimplemented", got)
	}
}
