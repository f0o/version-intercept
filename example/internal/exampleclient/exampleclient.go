// Package exampleclient holds helpers shared by the example client binaries:
// an h2c dialer, the common connection constants, and ConnectRPC client-option
// builders that auto-pin (or explicitly override) the microversion via the
// autopinconnect client interceptor. It exists so the legacy and modern example
// clients do not duplicate this wiring.
package exampleclient

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
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

// h2cDialer establishes a plain TCP connection for cleartext HTTP/2 (h2c),
// bypassing TLS. It lets an http2.Transport talk to the h2c example server
// without certificates.
//
// Arguments:
//   - ctx: The dial context.
//   - network: The network to dial (e.g. "tcp").
//   - addr: The target address.
//   - _: Unused TLS configuration required by the DialTLSContext signature.
//
// Returns the established connection or a dial error.
func h2cDialer(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
}

// h2cBase returns an http2.Transport configured for cleartext h2c.
//
// Returns the base RoundTripper.
func h2cBase() *http2.Transport {
	return &http2.Transport{AllowHTTP: true, DialTLSContext: h2cDialer}
}

// HTTPClient returns an http.Client speaking cleartext h2c to the example
// server. Microversion negotiation is handled by ConnectRPC client interceptors
// (see AutoPinOption / OverrideOption), not by the transport.
//
// Returns the configured *http.Client.
func HTTPClient() *http.Client {
	return &http.Client{Transport: h2cBase()}
}

// AutoPinOption returns a ConnectRPC client option that installs the
// autopinconnect interceptor, auto-pinning the Api-Version header to the
// microversion baked into the supplied service descriptor. It also returns the
// pinned version string for display.
//
// Arguments:
//   - sd: The client's own generated service descriptor.
//
// Returns the client option and the pinned version ("(none)" when the descriptor
// carries no service_versioning.max annotation).
func AutoPinOption(sd protoreflect.ServiceDescriptor) (connect.ClientOption, string) {
	pinned := "(none)"
	if v, ok := autopin.PinnedMaxVersion(sd); ok {
		pinned = v
	}
	interceptor := autopinconnect.Interceptor(sd, autopinconnect.WithServiceToken(ServiceToken))
	return connect.WithInterceptors(interceptor), pinned
}

// OverrideOption returns a ConnectRPC client option that installs the
// autopinconnect interceptor with an explicit microversion override, letting a
// client deliberately request a version other than the one it was compiled for.
//
// Arguments:
//   - apiVersion: The bare microversion to request, e.g. "1.0".
//
// Returns the client option.
func OverrideOption(apiVersion string) connect.ClientOption {
	interceptor := autopinconnect.Interceptor(nil,
		autopinconnect.WithServiceToken(ServiceToken),
		autopinconnect.WithVersion(apiVersion),
	)
	return connect.WithInterceptors(interceptor)
}
