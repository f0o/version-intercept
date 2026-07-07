// router.go implements the microversion registry and HTTP dispatch. A Router
// holds one HTTP handler per registered version range and forwards each
// incoming request to the handler whose range covers the negotiated version.
// All handlers share a single service contract and path; the negotiated
// microversion selects the behavioral implementation.
package router

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// contextKey is an unexported type for context keys defined in this package to
// avoid collisions with keys from other packages.
type contextKey int

const negotiatedVersionKey contextKey = iota

// registration binds a version range to the HTTP handler that serves it.
type registration struct {
	version Version
	rng     Range
	handler http.Handler
}

// Router dispatches incoming ConnectRPC/gRPC requests to a version-specific
// handler based on the microversion header. It implements http.Handler and can
// be mounted directly on an HTTP server or multiplexer.
type Router struct {
	header HeaderConfig

	mu            sync.RWMutex
	registrations []registration
	aggMin        Version
	aggMax        Version
	hasRange      bool
}

// Option configures a Router at construction time.
type Option func(*Router)

// WithHeaderConfig overrides the default header configuration.
//
// Arguments:
//   - cfg: The header configuration to use for version negotiation.
//
// Returns an Option applying the configuration.
func WithHeaderConfig(cfg HeaderConfig) Option {
	return func(r *Router) { r.header = cfg }
}

// New creates a Router for the given service token using the supplied options.
//
// Arguments:
//   - service: The service token advertised in the microversion header. May be
//     empty to accept bare version values.
//   - opts: Optional configuration overrides.
//
// Returns the initialized Router.
func New(service string, opts ...Option) *Router {
	r := &Router{
		header: DefaultHeaderConfig(service),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Register associates a version and its inclusive support range with the
// ConnectRPC handler that provides that microversion's behavior. All registered
// handlers implement the same service contract; the negotiated version selects
// which behavioral implementation runs.
//
// Arguments:
//   - version: The primary version this handler represents (used when the
//     handler is selected as "latest" and reported in the response header).
//   - rng: The inclusive version range the handler serves.
//   - handler: The ConnectRPC HTTP handler serving this version.
func (r *Router) Register(version Version, rng Range, handler http.Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registrations = append(r.registrations, registration{
		version: version,
		rng:     rng,
		handler: handler,
	})
	sort.SliceStable(r.registrations, func(i, j int) bool {
		return r.registrations[i].version.LessThan(r.registrations[j].version)
	})
	r.recomputeRange()
}

// recomputeRange refreshes the cached aggregate minimum and maximum versions
// from the current registrations. Callers must hold r.mu for writing.
func (r *Router) recomputeRange() {
	if len(r.registrations) == 0 {
		r.hasRange = false
		return
	}
	min := r.registrations[0].rng.Min
	max := r.registrations[0].version
	for _, reg := range r.registrations[1:] {
		if reg.rng.Min.LessThan(min) {
			min = reg.rng.Min
		}
		if max.LessThan(reg.version) {
			max = reg.version
		}
	}
	r.aggMin, r.aggMax, r.hasRange = min, max, true
}

// SupportedRange reports the aggregate minimum and maximum versions currently
// registered. It reads the value cached at registration time.
//
// Returns:
//   - Version: The lowest registered minimum version.
//   - Version: The highest registered version.
//   - bool: False when no versions are registered.
func (r *Router) SupportedRange() (Version, Version, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.aggMin, r.aggMax, r.hasRange
}

// resolve selects the registration serving the requested version. When latest
// is true (or no version is requested) the highest registered version wins.
//
// Arguments:
//   - v: The requested version (ignored when latest is true).
//   - latest: Whether the latest version was explicitly requested.
//
// Returns the matching registration or an error when no handler covers v.
func (r *Router) resolve(v Version, latest bool) (registration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.registrations) == 0 {
		return registration{}, fmt.Errorf("version-intercept: no versions registered")
	}
	if latest {
		return r.registrations[len(r.registrations)-1], nil
	}
	if r.hasRange && r.aggMax.LessThan(v) {
		return registration{}, fmt.Errorf("version-intercept: requested version %s exceeds maximum supported %s", v, r.aggMax)
	}
	for i := len(r.registrations) - 1; i >= 0; i-- {
		if r.registrations[i].rng.Contains(v) {
			return r.registrations[i], nil
		}
	}
	return registration{}, fmt.Errorf("version-intercept: unsupported version %s", v)
}

// ServeHTTP implements http.Handler. It negotiates the microversion from the
// request header, sets response version headers, stores the negotiated version
// in the request context, and forwards to the chosen handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	requested := req.Header.Get(r.header.RequestHeader)
	var (
		version Version
		latest  bool
		err     error
	)
	if strings.TrimSpace(requested) == "" {
		latest = true
	} else {
		version, latest, err = r.header.ParseRequestValue(requested)
		if err != nil {
			r.writeSupportHeaders(w)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	reg, err := r.resolve(version, latest)
	if err != nil {
		r.writeSupportHeaders(w)
		http.Error(w, err.Error(), http.StatusNotAcceptable)
		return
	}

	r.writeSupportHeaders(w)
	w.Header().Set(r.header.ResponseHeader, r.header.FormatResponseValue(reg.version))

	ctx := context.WithValue(req.Context(), negotiatedVersionKey, reg.version)
	reg.handler.ServeHTTP(w, req.WithContext(ctx))
}

// writeSupportHeaders advertises the aggregate supported version range.
func (r *Router) writeSupportHeaders(w http.ResponseWriter) {
	if min, max, ok := r.SupportedRange(); ok {
		w.Header().Set(r.header.MinHeader, min.String())
		w.Header().Set(r.header.MaxHeader, max.String())
	}
}

// NegotiatedVersion returns the version selected by the Router for the current
// request, if any. It is intended for use inside service implementations and by
// the versioning interceptor.
//
// Arguments:
//   - ctx: The request context passed to the handler.
//
// Returns the negotiated version and true when present.
func NegotiatedVersion(ctx context.Context) (Version, bool) {
	v, ok := ctx.Value(negotiatedVersionKey).(Version)
	return v, ok
}

// withNegotiatedVersion returns a context carrying the negotiated version. It is
// used by the interceptor when the router did not populate the context (e.g. a
// handler mounted without the router in front of it).
func withNegotiatedVersion(ctx context.Context, v Version) context.Context {
	return context.WithValue(ctx, negotiatedVersionKey, v)
}
