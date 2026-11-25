// streaming_test.go verifies the ConnectRPC v2 adapter's enforcement across
// unary and streaming RPCs: method-lifecycle rejection, field gating on
// received and sent messages (including clone-on-send for streams), and panic
// recovery mapping to CodeInternal.
package routerconnect

import (
	"context"
	"io"
	"testing"

	"connectrpc.com/connect/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.f0o.dev/version-intercept/router"
	"go.f0o.dev/version-intercept/router/testfixture"
)

// fakeServerStream is a minimal connect.ServerStream for tests.
type fakeServerStream struct {
	spec     connect.Spec
	incoming []proto.Message
	sent     []proto.Message
	recvIdx  int
}

func (s *fakeServerStream) SendHeaders() error { return nil }

func (s *fakeServerStream) Receive(msg any) error {
	if s.recvIdx >= len(s.incoming) {
		return io.EOF
	}
	src := s.incoming[s.recvIdx]
	s.recvIdx++
	proto.Merge(msg.(proto.Message), src)
	return nil
}

func (s *fakeServerStream) Send(msg any) error {
	s.sent = append(s.sent, proto.Clone(msg.(proto.Message)))
	return nil
}

func methodSpec(t *testing.T, method string, streamType connect.StreamType) connect.Spec {
	t.Helper()
	md, err := testfixture.Method(method)
	if err != nil {
		t.Fatalf("fixture method: %v", err)
	}
	return connect.Spec{
		Procedure:  "/routertest.v1.GreeterService/" + method,
		Schema:     md,
		StreamType: streamType,
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

func TestHandlerRejectsUnavailableMethod(t *testing.T) {
	ic := VersioningInterceptor()
	spec := methodSpec(t, "Farewell", connect.StreamTypeBidi)
	stream := &fakeServerStream{spec: spec}
	ctx := router.WithNegotiatedVersion(context.Background(), router.Version{Major: 1})

	called := false
	next := connect.ServerFunc(func(_ context.Context, _ connect.Spec, _ connect.ServerStream) error {
		called = true
		return nil
	})

	err := ic(next)(ctx, spec, stream)
	if err == nil || connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("Farewell at 1.0 = %v, want CodeUnimplemented", err)
	}
	if called {
		t.Error("handler should not run for an unavailable method")
	}
}

func TestStreamingHandlerGatesReceiveAndSend(t *testing.T) {
	ic := VersioningInterceptor()
	spec := methodSpec(t, "Hello", connect.StreamTypeBidi)
	stream := &fakeServerStream{
		spec:     spec,
		incoming: []proto.Message{fixtureMsg(t, "HelloRequest", map[string]string{"name": "Ada", "title": "Dr.", "language": "de"})},
	}
	ctx := router.WithNegotiatedVersion(context.Background(), router.Version{Major: 1})
	resp := fixtureMsg(t, "HelloResponse", map[string]string{"message": "hi", "served_by": "modern"})

	next := connect.ServerFunc(func(_ context.Context, _ connect.Spec, s connect.ServerStream) error {
		got := fixtureMsg(t, "HelloRequest", nil)
		if err := s.Receive(got); err != nil {
			return err
		}
		gm := got.ProtoReflect()
		if gm.Get(gm.Descriptor().Fields().ByName("title")).String() != "" {
			t.Errorf("received title not gated at 1.0")
		}
		return s.Send(resp)
	})

	if err := ic(next)(ctx, spec, stream); err != nil {
		t.Fatalf("stream handler: %v", err)
	}
	if len(stream.sent) != 1 {
		t.Fatalf("sent count = %d, want 1", len(stream.sent))
	}
	sent := stream.sent[0].ProtoReflect()
	if sent.Get(sent.Descriptor().Fields().ByName(protoreflect.Name("served_by"))).String() != "" {
		t.Errorf("sent served_by not gated at 1.0")
	}
	orig := resp.ProtoReflect()
	if orig.Get(orig.Descriptor().Fields().ByName(protoreflect.Name("served_by"))).String() != "modern" {
		t.Errorf("streaming Send mutated caller message")
	}
}

func TestUnaryHandlerGatesReceiveAndSend(t *testing.T) {
	ic := VersioningInterceptor()
	spec := methodSpec(t, "Hello", connect.StreamTypeUnary)
	stream := &fakeServerStream{
		spec:     spec,
		incoming: []proto.Message{fixtureMsg(t, "HelloRequest", map[string]string{"name": "Ada", "title": "Dr.", "language": "de"})},
	}
	ctx := router.WithNegotiatedVersion(context.Background(), router.Version{Major: 1})

	next := connect.ServerFunc(func(_ context.Context, _ connect.Spec, s connect.ServerStream) error {
		got := fixtureMsg(t, "HelloRequest", nil)
		if err := s.Receive(got); err != nil {
			return err
		}
		gm := got.ProtoReflect()
		if gm.Get(gm.Descriptor().Fields().ByName("title")).String() != "" {
			t.Errorf("received title not gated at 1.0")
		}
		return s.Send(fixtureMsg(t, "HelloResponse", map[string]string{"message": "hi", "served_by": "modern"}))
	})

	if err := ic(next)(ctx, spec, stream); err != nil {
		t.Fatalf("unary handler: %v", err)
	}
	if len(stream.sent) != 1 {
		t.Fatalf("sent count = %d, want 1", len(stream.sent))
	}
	sent := stream.sent[0].ProtoReflect()
	if sent.Get(sent.Descriptor().Fields().ByName(protoreflect.Name("served_by"))).String() != "" {
		t.Errorf("sent served_by not gated at 1.0")
	}
}

func TestStreamingHandlerRecoversPanic(t *testing.T) {
	ic := VersioningInterceptor()
	spec := methodSpec(t, "Hello", connect.StreamTypeBidi)
	stream := &fakeServerStream{spec: spec}
	ctx := router.WithNegotiatedVersion(context.Background(), router.Version{Major: 2})

	next := connect.ServerFunc(func(_ context.Context, _ connect.Spec, _ connect.ServerStream) error {
		panic("stream boom")
	})

	err := ic(next)(ctx, spec, stream)
	if err == nil || connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("recovered stream panic = %v, want CodeInternal", err)
	}
}
