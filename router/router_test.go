package router

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in      string
		want    Version
		wantErr bool
	}{
		{"1", Version{1, 0}, false},
		{"2.3", Version{2, 3}, false},
		{"  2.10 ", Version{2, 10}, false},
		{"", Version{}, true},
		{"x", Version{}, true},
		{"1.y", Version{}, true},
		{"-1.0", Version{}, true},
	}
	for _, tt := range tests {
		got, err := ParseVersion(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseVersion(%q) expected error, got %v", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVersion(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseVersion(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		a, b Version
		want int
	}{
		{Version{1, 0}, Version{1, 0}, 0},
		{Version{1, 0}, Version{1, 1}, -1},
		{Version{2, 0}, Version{1, 9}, 1},
		{Version{1, 10}, Version{1, 2}, 1},
	}
	for _, tt := range tests {
		if got := tt.a.Compare(tt.b); got != tt.want {
			t.Errorf("%v.Compare(%v) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRangeContains(t *testing.T) {
	bounded := Range{Min: Version{1, 0}, Max: Version{1, 9}}
	if !bounded.Contains(Version{1, 5}) {
		t.Error("bounded range should contain 1.5")
	}
	if bounded.Contains(Version{2, 0}) {
		t.Error("bounded range should not contain 2.0")
	}
	if bounded.Contains(Version{0, 9}) {
		t.Error("bounded range should not contain 0.9")
	}

	unbounded := Range{Min: Version{2, 0}}
	if !unbounded.Contains(Version{9, 9}) {
		t.Error("unbounded range should contain 9.9")
	}
	if unbounded.Contains(Version{1, 9}) {
		t.Error("unbounded range should not contain 1.9")
	}
}

func TestParseRequestValue(t *testing.T) {
	cfg := DefaultHeaderConfig("greeter")

	v, latest, err := cfg.ParseRequestValue("greeter 2.3")
	if err != nil || latest || v != (Version{2, 3}) {
		t.Errorf("service+version parse failed: v=%v latest=%v err=%v", v, latest, err)
	}

	v, latest, err = cfg.ParseRequestValue("1.5")
	if err != nil || latest || v != (Version{1, 5}) {
		t.Errorf("bare version parse failed: v=%v latest=%v err=%v", v, latest, err)
	}

	_, latest, err = cfg.ParseRequestValue("latest")
	if err != nil || !latest {
		t.Errorf("latest keyword parse failed: latest=%v err=%v", latest, err)
	}

	_, latest, err = cfg.ParseRequestValue("greeter latest")
	if err != nil || !latest {
		t.Errorf("service+latest parse failed: latest=%v err=%v", latest, err)
	}

	if _, _, err := cfg.ParseRequestValue("other 2.3"); err == nil {
		t.Error("mismatched service token should error")
	}
	if _, _, err := cfg.ParseRequestValue(""); err == nil {
		t.Error("empty value should error")
	}
}

// tagHandler records the negotiated version it observed via the request context.
type tagHandler struct{ tag string }

func (h tagHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Handler", h.tag)
	if v, ok := NegotiatedVersion(r.Context()); ok {
		w.Header().Set("X-Negotiated", v.String())
	}
	w.WriteHeader(http.StatusOK)
}

func newTestRouter() *Router {
	r := New("greeter")
	r.Register(Version{1, 0}, Range{Min: Version{1, 0}, Max: Version{1, 9}}, tagHandler{"legacy"})
	r.Register(Version{2, 1}, Range{Min: Version{2, 0}}, tagHandler{"modern"})
	return r
}

func doRequest(r *Router, apiVersion string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/greeter.v1.GreeterService/Hello", nil)
	if apiVersion != "" {
		req.Header.Set(DefaultRequestHeader, apiVersion)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestRouterDispatch(t *testing.T) {
	r := newTestRouter()

	if got := doRequest(r, "greeter 1.5").Header().Get("X-Handler"); got != "legacy" {
		t.Errorf("1.5 routed to %q, want legacy", got)
	}
	if got := doRequest(r, "greeter 2.0").Header().Get("X-Handler"); got != "modern" {
		t.Errorf("2.0 routed to %q, want modern", got)
	}
	if got := doRequest(r, "latest").Header().Get("X-Handler"); got != "modern" {
		t.Errorf("latest routed to %q, want modern", got)
	}
	if got := doRequest(r, "").Header().Get("X-Handler"); got != "modern" {
		t.Errorf("absent header routed to %q, want modern (latest)", got)
	}
}

func TestRouterNegotiatedVersionInContext(t *testing.T) {
	r := newTestRouter()
	if got := doRequest(r, "greeter 1.0").Header().Get("X-Negotiated"); got != "1.0" {
		t.Errorf("negotiated = %q, want 1.0", got)
	}
	if got := doRequest(r, "latest").Header().Get("X-Negotiated"); got != "2.1" {
		t.Errorf("negotiated = %q, want 2.1", got)
	}
}

func TestRouterAdvertisesRange(t *testing.T) {
	r := newTestRouter()
	rec := doRequest(r, "greeter 1.0")
	if got := rec.Header().Get(DefaultMinHeader); got != "1.0" {
		t.Errorf("min header = %q, want 1.0", got)
	}
	if got := rec.Header().Get(DefaultMaxHeader); got != "2.1" {
		t.Errorf("max header = %q, want 2.1", got)
	}
	if got := rec.Header().Get(DefaultResponseHeader); got != "greeter 1.0" {
		t.Errorf("response version header = %q, want 'greeter 1.0'", got)
	}
}

func TestRouterUnsupportedVersion(t *testing.T) {
	r := newTestRouter()
	if rec := doRequest(r, "greeter 5.0"); rec.Code != http.StatusNotAcceptable {
		t.Errorf("unsupported version status = %d, want %d", rec.Code, http.StatusNotAcceptable)
	}
}

func TestRouterMalformedVersion(t *testing.T) {
	r := newTestRouter()
	if rec := doRequest(r, "greeter bogus"); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed version status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRouterServiceTokenMismatch(t *testing.T) {
	r := newTestRouter()
	if rec := doRequest(r, "other 1.0"); rec.Code != http.StatusBadRequest {
		t.Errorf("service-token mismatch status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRouterNoVersionsRegistered(t *testing.T) {
	r := New("greeter")
	if rec := doRequest(r, "1.0"); rec.Code != http.StatusNotAcceptable {
		t.Errorf("empty router status = %d, want %d", rec.Code, http.StatusNotAcceptable)
	}
}

// TestRouterConcurrentRegisterAndServe exercises the Router under the race
// detector with concurrent registration and dispatch to prove thread-safety.
func TestRouterConcurrentRegisterAndServe(_ *testing.T) {
	r := newTestRouter()
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			r.Register(Version{3, i}, Range{Min: Version{3, 0}}, tagHandler{"future"})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			doRequest(r, "greeter 2.0")
		}
	}()

	wg.Wait()
}

func BenchmarkRouterDispatch(b *testing.B) {
	r := newTestRouter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doRequest(r, "greeter 2.0")
	}
}
