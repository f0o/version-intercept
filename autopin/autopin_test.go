package autopin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.f0o.dev/version-intercept/autopin"
	annotationsv1 "go.f0o.dev/version-intercept/gen/versionintercept/annotations/v1"
)

// serviceDescriptorWithMax builds a self-contained ServiceDescriptor carrying
// the versionintercept service_versioning.max annotation set to the given value.
// It depends only on the library's own annotations package, so the autopin unit
// tests do not need any example-generated code.
//
// Arguments:
//   - max: The service_versioning.max value to bake into the descriptor.
//
// Returns the constructed ServiceDescriptor.
func serviceDescriptorWithMax(t *testing.T, maxVer string) protoreflect.ServiceDescriptor {
	t.Helper()

	svcOpts := &descriptorpb.ServiceOptions{}
	proto.SetExtension(svcOpts, annotationsv1.E_ServiceVersioning, &annotationsv1.ServiceVersioning{
		Min: "1.0",
		Max: maxVer,
	})

	fdProto := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("autopin_test/fixture.proto"),
		Package: proto.String("autopin.test.v1"),
		Syntax:  proto.String("proto3"),
		Dependency: []string{
			"versionintercept/annotations/v1/annotations.proto",
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name:    proto.String("FixtureService"),
				Options: svcOpts,
			},
		},
	}

	deps := new(protoregistry.Files)
	annFile := annotationsv1.File_versionintercept_annotations_v1_annotations_proto
	if err := deps.RegisterFile(annFile); err != nil {
		t.Fatalf("register annotations file: %v", err)
	}

	fd, err := protodesc.NewFile(fdProto, deps)
	if err != nil {
		t.Fatalf("build fixture file descriptor: %v", err)
	}
	return fd.Services().Get(0)

}

func TestPinnedMaxVersionModern(t *testing.T) {
	sd := serviceDescriptorWithMax(t, "2.1")

	got, ok := autopin.PinnedMaxVersion(sd)
	if !ok {
		t.Fatalf("PinnedMaxVersion returned ok=false, want true")
	}
	if got != "2.1" {
		t.Errorf("PinnedMaxVersion = %q, want %q", got, "2.1")
	}
}

func TestPinnedMaxVersionLegacy(t *testing.T) {
	sd := serviceDescriptorWithMax(t, "1.0")

	got, ok := autopin.PinnedMaxVersion(sd)
	if !ok || got != "1.0" {
		t.Errorf("PinnedMaxVersion = (%q, %v), want (\"1.0\", true)", got, ok)
	}
}

// TestHeaderValueModern verifies the OpenStack-style header value formatting.
func TestHeaderValueModern(t *testing.T) {
	sd := serviceDescriptorWithMax(t, "2.1")

	if got, want := autopin.HeaderValue("greeter", sd), "greeter 2.1"; got != want {
		t.Errorf("HeaderValue = %q, want %q", got, want)
	}
	if got, want := autopin.HeaderValue("", sd), "2.1"; got != want {
		t.Errorf("HeaderValue(bare) = %q, want %q", got, want)
	}
}

// TestPinnedMaxVersionNil verifies nil-safety.
func TestPinnedMaxVersionNil(t *testing.T) {
	if _, ok := autopin.PinnedMaxVersion(nil); ok {
		t.Errorf("PinnedMaxVersion(nil) ok=true, want false")
	}
}

// captureTransport records the Api-Version header of the request it sees and
// returns an empty 200 response.
type captureTransport struct{ got string }

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.got = req.Header.Get(autopin.DefaultRequestHeader)
	return httptest.NewRecorder().Result(), nil
}

// TestAutoPinTransportStampsHeader verifies AutoPinTransport stamps the pinned
// Api-Version header when the descriptor carries the annotation.
func TestAutoPinTransportStampsHeader(t *testing.T) {
	sd := serviceDescriptorWithMax(t, "2.1")
	capture := &captureTransport{}
	rt := autopin.AutoPinTransport("greeter", sd, capture)

	req, _ := http.NewRequest(http.MethodGet, "http://example.test/", nil)
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}

	if capture.got != "greeter 2.1" {
		t.Fatalf("stamped header = %q, want %q", capture.got, "greeter 2.1")
	}
	if req.Header.Get(autopin.DefaultRequestHeader) != "" {
		t.Fatal("AutoPinTransport mutated the original request header")
	}
}

// TestAutoPinTransportPassthroughWithoutAnnotation verifies the transport sends
// no version header when the descriptor lacks the annotation.
func TestAutoPinTransportPassthroughWithoutAnnotation(t *testing.T) {
	sd := serviceDescriptorWithMax(t, "")
	capture := &captureTransport{}
	rt := autopin.AutoPinTransport("greeter", sd, capture)

	req, _ := http.NewRequest(http.MethodGet, "http://example.test/", nil)
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	if capture.got != "" {
		t.Fatalf("expected no version header, got %q", capture.got)
	}
}
