// interceptor_test.go verifies the ConnectRPC adapter's enforcement: panic
// recovery mapping to CodeInternal, method-lifecycle rejection, no-version
// passthrough, and opt-in metric recording.
package routerconnect

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/protobuf/proto"

	"go.f0o.dev/version-intercept/router"
	"go.f0o.dev/version-intercept/router/testfixture"
)

// unaryReq is a minimal connect.AnyRequest for exercising WrapUnary.
type unaryReq struct {
	connect.AnyRequest
	procedure string
	msg       proto.Message
}

func (r *unaryReq) Any() any           { return r.msg }
func (r *unaryReq) Spec() connect.Spec { return connect.Spec{Procedure: r.procedure} }

func mustMessage(t *testing.T, typeName string, fields map[string]string) proto.Message {
	t.Helper()
	m, err := testfixture.NewMessage(typeName, fields)
	if err != nil {
		t.Fatalf("fixture message: %v", err)
	}
	return m
}

func TestWrapUnaryRecoversPanic(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := router.NewMetrics(reg)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	i := &versioningInterceptor{metrics: m}

	next := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		panic("boom")
	})

	ctx := router.WithNegotiatedVersion(context.Background(), router.Version{Major: 1})
	req := &unaryReq{procedure: "/svc/Method", msg: mustMessage(t, "HelloRequest", map[string]string{"name": "x"})}
	_, gotErr := i.WrapUnary(next)(ctx, req)

	if gotErr == nil || connect.CodeOf(gotErr) != connect.CodeInternal {
		t.Fatalf("recovered panic = %v, want CodeInternal", gotErr)
	}
	if got := testutil.ToFloat64(m.Collectors()[0].(*prometheus.CounterVec).WithLabelValues("svc", "Method", "1.0", "internal")); got != 1 {
		t.Errorf("panic metric count = %v, want 1", got)
	}
}

func TestWrapUnaryNoVersionPassesThrough(t *testing.T) {
	i := &versioningInterceptor{}
	called := false
	next := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return nil, nil
	})
	if _, err := i.WrapUnary(next)(context.Background(), &unaryReq{procedure: "/svc/Method"}); err != nil {
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
	if got := resultCode(connect.NewError(connect.CodeUnimplemented, context.Canceled)); got != "unimplemented" {
		t.Errorf("resultCode(unimplemented) = %q, want unimplemented", got)
	}
}
