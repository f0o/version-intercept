// Package exampleclient holds helpers shared by the example client binaries:
// a cleartext HTTP/2 (h2c) HTTP client, common connection constants, and
// ConnectRPC v2 client and interceptor builders that auto-pin (or explicitly
// override) the microversion via the autopinconnect client interceptor. It
// exists so the legacy and modern example clients do not duplicate this wiring.
package exampleclient

import (
	"net/http"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.f0o.dev/version-intercept/autopin"
	autopinconnect "go.f0o.dev/version-intercept/autopin/connect"
)

// Connection constants shared by the example clients.
const (
	// BaseURL is the modern example server's address.
	BaseURL = "http://localhost:8080"
	// ServiceToken is the OpenStack-style service token for the greeter service.
	ServiceToken = "greeter"
	// RequestHeader is the microversion request header name.
	RequestHeader = "Api-Version"
)

// HTTPClient returns an http.Client speaking cleartext HTTP/2 (h2c) to the
// example server. Microversion negotiation is handled by ConnectRPC client
// interceptors (see AutoPinInterceptor / OverrideInterceptor), not by the
// transport.
//
// Returns the configured *http.Client.
func HTTPClient() *http.Client {
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Client{
		Transport: &http.Transport{
			Protocols: protocols,
		},
	}
}

// NewConnectClient constructs a ConnectRPC v2 client speaking cleartext h2c to
// the example server with the supplied client interceptors.
//
// Arguments:
//   - interceptors: Client interceptors to attach (e.g. auto-pin or override).
//
// Returns the configured *connect.Client.
func NewConnectClient(interceptors ...connect.ClientInterceptor) *connect.Client {
	return connect.NewClient(
		connecthttp.NewTransport(HTTPClient(), BaseURL),
		interceptors...,
	)
}

// AutoPinInterceptor returns a ConnectRPC v2 client interceptor that stamps
// the Api-Version header from the microversion baked into the supplied service
// descriptor. It also returns the pinned version string for display.
//
// Arguments:
//   - sd: The client's own generated service descriptor.
//
// Returns the client interceptor and the pinned version ("(none)" when the
// descriptor carries no service_versioning.max annotation).
func AutoPinInterceptor(sd protoreflect.ServiceDescriptor) (connect.ClientInterceptor, string) {
	pinned := "(none)"
	if v, ok := autopin.PinnedMaxVersion(sd); ok {
		pinned = v
	}
	return autopinconnect.Interceptor(sd, autopinconnect.WithServiceToken(ServiceToken)), pinned
}

// OverrideInterceptor returns a ConnectRPC v2 client interceptor with an
// explicit microversion override, letting a client deliberately request a
// version other than the one it was compiled for.
//
// Arguments:
//   - apiVersion: The bare microversion to request, e.g. "1.0".
//
// Returns the configured client interceptor.
func OverrideInterceptor(apiVersion string) connect.ClientInterceptor {
	return autopinconnect.Interceptor(nil,
		autopinconnect.WithServiceToken(ServiceToken),
		autopinconnect.WithVersion(apiVersion),
	)
}
