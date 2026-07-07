// streaming_test.go verifies the ConnectRPC adapter's streaming enforcement:
// method-lifecycle rejection, field gating on received and sent messages, and
// panic recovery mapping to CodeInternal.
package routerconnect

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.f0o.dev/version-intercept/router"
	"go.f0o.dev/version-intercept/router/testfixture"
)

// fakeHandlerConn is a minimal connect.StreamingHandlerConn for tests.
type fakeHandlerConn struct {
	connect.StreamingHandlerConn
	spec     connect.Spec
	incoming []proto.Message
	sent     []proto.Message
	recvIdx  int
}

func (c *fakeHandlerConn) Spec() connect.Spec { return c.spec }

func (c *fakeHandlerConn) Receive(msg any) error {
	if c.recvIdx >= len(c.incoming) {
		return connect.NewError(connect.CodeUnknown, context.Canceled)
	}
	src := c.incoming[c.recvIdx]
	c.recvIdx++
	proto.Merge(msg.(proto.Message), src)
	return nil
}

func (c *fakeHandlerConn) Send(msg any) error {
	c.sent = append(c.sent, proto.Clone(msg.(proto.Message)))
	return nil
}

func methodSpec(t *testing.T, method string) connect.Spec {
	t.Helper()
	md, err := testfixture.Method(method)
	if err != nil {
		t.Fatalf("fixture method: %v", err)
	}
	return connect.Spec{
		Procedure:  "/routertest.v1.GreeterService/" + method,
		Schema:     md,
		StreamType: connect.StreamTypeBidi,
	}
}

func fixtureMsg(t *testing.T, typeName string, fields map[string]string) proto.Message {
	t.Helper()
	m, err := testfixture.NewMessage(typeName, fields)
	if err != nil {
		t.Fatalf("fixture message: %v", err)
	}
	return m
}

func TestStreamingHandlerRejectsUnavailableMethod(t *testing.T) {
	i := &versioningInterceptor{}
	conn := &fakeHandlerConn{spec: methodSpec(t, "Farewell")}
	ctx := router.WithNegotiatedVersion(context.Background(), router.Version{Major: 1})

	called := false
	next := connect.StreamingHandlerFunc(func(ctx context.Context, c connect.StreamingHandlerConn) error {
		called = true
		return nil
	})

	err := i.WrapStreamingHandler(next)(ctx, conn)
	if err == nil || connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("Farewell at 1.0 = %v, want CodeUnimplemented", err)
	}
	if called {
		t.Error("handler should not run for an unavailable method")
	}
}

func TestStreamingHandlerGatesReceiveAndSend(t *testing.T) {
	i := &versioningInterceptor{}
	conn := &fakeHandlerConn{
		spec:     methodSpec(t, "Hello"),
		incoming: []proto.Message{fixtureMsg(t, "HelloRequest", map[string]string{"name": "Ada", "title": "Dr.", "language": "de"})},
	}
	ctx := router.WithNegotiatedVersion(context.Background(), router.Version{Major: 1})

	next := connect.StreamingHandlerFunc(func(ctx context.Context, c connect.StreamingHandlerConn) error {
		got := fixtureMsg(t, "HelloRequest", nil)
		if err := c.Receive(got); err != nil {
			return err
		}
		gm := got.ProtoReflect()
		if gm.Get(gm.Descriptor().Fields().ByName("title")).String() != "" {
			t.Errorf("received title not gated at 1.0")
		}
		return c.Send(fixtureMsg(t, "HelloResponse", map[string]string{"message": "hi", "served_by": "modern"}))
	})

	if err := i.WrapStreamingHandler(next)(ctx, conn); err != nil {
		t.Fatalf("stream handler: %v", err)
	}
	if len(conn.sent) != 1 {
		t.Fatalf("sent count = %d, want 1", len(conn.sent))
	}
	sent := conn.sent[0].ProtoReflect()
	if sent.Get(sent.Descriptor().Fields().ByName(protoreflect.Name("served_by"))).String() != "" {
		t.Errorf("sent served_by not gated at 1.0")
	}
}

func TestStreamingHandlerRecoversPanic(t *testing.T) {
	i := &versioningInterceptor{}
	conn := &fakeHandlerConn{spec: methodSpec(t, "Hello")}
	ctx := router.WithNegotiatedVersion(context.Background(), router.Version{Major: 2})

	next := connect.StreamingHandlerFunc(func(ctx context.Context, c connect.StreamingHandlerConn) error {
		panic("stream boom")
	})

	err := i.WrapStreamingHandler(next)(ctx, conn)
	if err == nil || connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("recovered stream panic = %v, want CodeInternal", err)
	}
}
