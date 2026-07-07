// metrics_test.go verifies the opt-in Prometheus instrumentation used by the
// versioning interceptor: procedure parsing, result-code mapping, registration
// on a caller-supplied registry, and counter recording.
package router

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestSplitProcedure(t *testing.T) {
	tests := []struct {
		procedure   string
		wantService string
		wantMethod  string
	}{
		{"/greeter.v1.GreeterService/Hello", "greeter.v1.GreeterService", "Hello"},
		{"greeter.v1.GreeterService/Farewell", "greeter.v1.GreeterService", "Farewell"},
		{"", "unknown", "unknown"},
		{"/onlyservice", "unknown", "unknown"},
	}
	for _, tt := range tests {
		service, method := splitProcedure(tt.procedure)
		if service != tt.wantService || method != tt.wantMethod {
			t.Errorf("splitProcedure(%q) = (%q, %q), want (%q, %q)", tt.procedure, service, method, tt.wantService, tt.wantMethod)
		}
	}
}

func TestNewMetricsRegistersOnProvidedRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := NewMetrics(reg)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}

	m.record("/greeter.v1.GreeterService/Hello", "2.1", "ok")

	const expected = `
# HELP version_intercept_requests_total Total number of requests processed by the versioning interceptor, labeled by service, method, negotiated API version, and result code.
# TYPE version_intercept_requests_total counter
version_intercept_requests_total{api_version="2.1",code="ok",method="Hello",service="greeter.v1.GreeterService"} 1
`
	if err := testutil.CollectAndCompare(m.requestsTotal, strings.NewReader(expected), "version_intercept_requests_total"); err != nil {
		t.Errorf("unexpected collect result: %v", err)
	}
}

func TestNilMetricsRecordIsNoop(t *testing.T) {
	var m *Metrics
	m.record("/greeter.v1.GreeterService/Hello", "2.1", "ok")
	if got := m.Collectors(); got != nil {
		t.Errorf("nil Metrics Collectors() = %v, want nil", got)
	}
}

func TestNewMetricsNilRegistererLeavesUnregistered(t *testing.T) {
	m, err := NewMetrics(nil)
	if err != nil {
		t.Fatalf("NewMetrics(nil): %v", err)
	}
	if len(m.Collectors()) != 1 {
		t.Errorf("Collectors() length = %d, want 1", len(m.Collectors()))
	}
	reg := prometheus.NewRegistry()
	for _, c := range m.Collectors() {
		if err := reg.Register(c); err != nil {
			t.Errorf("register collector: %v", err)
		}
	}
}
