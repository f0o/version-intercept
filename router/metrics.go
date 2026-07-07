// metrics.go provides opt-in Prometheus instrumentation for the versioning
// interceptor. Rather than registering on the global default registry (a library
// anti-pattern that risks duplicate-registration panics and denies the host
// application control), a Metrics value owns its collectors and is registered on
// a caller-supplied prometheus.Registerer. When no Metrics is configured on the
// interceptor, no metrics are recorded.
package router

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the Prometheus collectors used by the versioning interceptor.
// Construct it with NewMetrics and pass it to the interceptor via WithMetrics.
// A nil *Metrics is valid and records nothing, making metrics fully opt-in.
type Metrics struct {
	requestsTotal *prometheus.CounterVec
}

// NewMetrics creates the versioning interceptor's collectors and registers them
// on the provided registerer. Passing a dedicated registry keeps the library
// free of global state and lets the host expose the metrics however it wishes.
//
// Arguments:
//   - reg: The registry to register the collectors on. When nil, the collectors
//     are created but left unregistered so the caller may register them later.
//
// Returns the initialized Metrics or an error if registration fails (for
// example, a collector with the same name is already registered).
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "version_intercept_requests_total",
			Help: "Total number of requests processed by the versioning interceptor, labeled by service, method, negotiated API version, and result code.",
		},
		[]string{"service", "method", "api_version", "code"},
	)
	if reg != nil {
		if err := reg.Register(requestsTotal); err != nil {
			return nil, err
		}
	}
	return &Metrics{requestsTotal: requestsTotal}, nil
}

// Collectors returns the Prometheus collectors owned by this Metrics so callers
// that constructed it with a nil registerer can register them on their own
// registry.
//
// Returns the slice of collectors to register.
func (m *Metrics) Collectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return []prometheus.Collector{m.requestsTotal}
}

// Record increments the request counter for a single call. It is a no-op when
// the receiver is nil, so metrics remain fully opt-in. Adapter modules
// (ConnectRPC, gRPC) call this after resolving a framework-neutral result code.
//
// Arguments:
//   - procedure: The method path, e.g. "/pkg.Service/Method".
//   - apiVersion: The negotiated microversion label, e.g. "2.1" or "unknown".
//   - code: The result code label, e.g. "ok" or "unimplemented".
func (m *Metrics) Record(procedure, apiVersion, code string) {
	if m == nil {
		return
	}
	service, method := splitProcedure(procedure)
	m.requestsTotal.WithLabelValues(service, method, apiVersion, code).Inc()
}

// record is the internal alias retained for existing package-internal tests.
func (m *Metrics) record(procedure, apiVersion, code string) {
	m.Record(procedure, apiVersion, code)
}

// splitProcedure splits a Connect procedure string of the form
// "/pkg.Service/Method" into its service and method components.
//
// Arguments:
//   - procedure: The Connect procedure string.
//
// Returns the service and method names; either may be "unknown" when the
// procedure cannot be parsed.
func splitProcedure(procedure string) (service, method string) {
	trimmed := strings.Trim(procedure, "/")
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return "unknown", "unknown"
	}
	service = trimmed[:idx]
	method = trimmed[idx+1:]
	if service == "" {
		service = "unknown"
	}
	if method == "" {
		method = "unknown"
	}
	return service, method
}
