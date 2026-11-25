// Package autopin lets a client automatically pin the microversion it was
// compiled for by reading the service_versioning.max annotation baked into its
// own generated protobuf descriptor. A client generated from an old proto
// snapshot therefore auto-negotiates the old microversion, while a client
// generated from a newer snapshot auto-negotiates the newer one — with no
// hardcoded version string.
//
// This package deliberately depends only on protoreflect and the standard
// library. It reads the annotation by field number rather than importing any
// generated annotations package, so a client can link its OWN copy of the
// annotations without colliding with the server's copy in the proto registry.
package autopin

import (
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// serviceVersioningFieldNumber is the extension field number of
// versionintercept.annotations.v1.service_versioning on ServiceOptions.
const serviceVersioningFieldNumber protoreflect.FieldNumber = 50001

// maxFieldNumber is the field number of ServiceVersioning.max.
const maxFieldNumber protoreflect.FieldNumber = 2

// DefaultRequestHeader is the OpenStack-style microversion request header the
// AutoPinTransport stamps by default.
const DefaultRequestHeader = "Api-Version"

// PinnedMaxVersion reads the service_versioning.max annotation from a service
// descriptor and returns it as the version string the client should pin to. The
// value is baked into the descriptor at codegen time, so it reflects the newest
// microversion the client's frozen proto snapshot knew about.
//
// Arguments:
//   - sd: The service descriptor from the client's generated code.
//
// Returns the pinned max version string (e.g. "1.0") and true when the
// annotation is present; false otherwise.
func PinnedMaxVersion(sd protoreflect.ServiceDescriptor) (string, bool) {
	if sd == nil {
		return "", false
	}
	opts := sd.Options()
	if opts == nil {
		return "", false
	}

	var result string
	var found bool
	opts.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, val protoreflect.Value) bool {
		if fd.Number() != serviceVersioningFieldNumber {
			return true
		}
		sv := val.Message()
		maxFD := sv.Descriptor().Fields().ByNumber(maxFieldNumber)
		if maxFD == nil {
			return false
		}
		result = strings.TrimSpace(sv.Get(maxFD).String())
		found = result != ""
		return false
	})
	return result, found
}

// HeaderValue renders the Api-Version header value for a service token and the
// pinned version read from the descriptor. It returns "" when the descriptor
// carries no service_versioning.max annotation.
//
// Arguments:
//   - service: The service token to prefix the version with (OpenStack style).
//     May be empty to produce a bare version value.
//   - sd: The service descriptor from the client's generated code.
//
// Returns the header value (e.g. "greeter 1.0") or "" when unavailable.
func HeaderValue(service string, sd protoreflect.ServiceDescriptor) string {
	maxVer, ok := PinnedMaxVersion(sd)
	if !ok {
		return ""
	}
	if !validVersion(maxVer) {
		return ""
	}
	if service != "" {
		return service + " " + maxVer
	}
	return maxVer
}

// AutoPinTransport wraps an http.RoundTripper so every request carries the
// Api-Version header pinned to the client's compiled-in service_versioning.max.
// When the annotation is absent, requests are sent without a version header (the
// server then serves its latest supported version).
//
// Arguments:
//   - service: The service token to prefix the version with (OpenStack style).
//   - sd: The service descriptor from the client's generated code.
//   - base: The underlying RoundTripper; when nil, http.DefaultTransport is used.
//
// Returns a RoundTripper that stamps the auto-pinned version header.
func AutoPinTransport(service string, sd protoreflect.ServiceDescriptor, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return autoPinRoundTripper{
		base:   base,
		header: DefaultRequestHeader,
		value:  HeaderValue(service, sd),
	}
}

// autoPinRoundTripper stamps a fixed Api-Version header on every request.
type autoPinRoundTripper struct {
	base   http.RoundTripper
	header string
	value  string
}

// RoundTrip implements http.RoundTripper.
func (t autoPinRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.value == "" {
		return t.base.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	clone.Header.Set(t.header, t.value)
	return t.base.RoundTrip(clone)
}

// validVersion reports whether s parses as a major[.minor] version.
//
// Arguments:
//   - s: The candidate version string.
//
// Returns true when s is a valid version.
func validVersion(s string) bool {
	parts := strings.SplitN(strings.TrimSpace(s), ".", 2)
	if major, err := strconv.Atoi(strings.TrimSpace(parts[0])); err != nil || major < 0 {
		return false
	}
	if len(parts) == 2 {
		if minor, err := strconv.Atoi(strings.TrimSpace(parts[1])); err != nil || minor < 0 {
			return false
		}
	}
	return true
}
