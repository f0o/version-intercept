// header.go implements parsing and rendering of the OpenStack-style
// microversion request/response headers used to negotiate the served version.
package router

import (
	"fmt"
	"strings"
)

// Default header names follow the OpenStack microversion convention where a
// single request header carries "<service> <version>" and the response echoes
// the negotiated version alongside the advertised supported range.
const (
	// DefaultRequestHeader is the header clients set to request a microversion.
	DefaultRequestHeader = "Api-Version"
	// DefaultResponseHeader is the header the server sets to report the served
	// microversion.
	DefaultResponseHeader = "Api-Version"
	// DefaultMinHeader advertises the lowest supported microversion.
	DefaultMinHeader = "Api-Min-Version"
	// DefaultMaxHeader advertises the highest supported microversion.
	DefaultMaxHeader = "Api-Max-Version"
)

// LatestKeyword is the request value that selects the newest supported version.
const LatestKeyword = "latest"

// HeaderConfig describes how microversion information is carried in headers.
// The zero value is not usable; use DefaultHeaderConfig to obtain sensible
// defaults and override as needed.
type HeaderConfig struct {
	// Service is the token that prefixes the version in the request header,
	// matching the OpenStack "<service> <version>" convention. When empty, the
	// request value is treated as a bare version string.
	Service string

	// RequestHeader is the header name clients use to request a version.
	RequestHeader string
	// ResponseHeader is the header name used to report the served version.
	ResponseHeader string
	// MinHeader advertises the minimum supported version in responses.
	MinHeader string
	// MaxHeader advertises the maximum supported version in responses.
	MaxHeader string
}

// DefaultHeaderConfig returns a HeaderConfig using the default header names for
// the supplied service token.
//
// Arguments:
//   - service: The service token that prefixes the version in requests. May be
//     empty to accept bare version values.
//
// Returns a ready-to-use HeaderConfig.
func DefaultHeaderConfig(service string) HeaderConfig {
	return HeaderConfig{
		Service:        service,
		RequestHeader:  DefaultRequestHeader,
		ResponseHeader: DefaultResponseHeader,
		MinHeader:      DefaultMinHeader,
		MaxHeader:      DefaultMaxHeader,
	}
}

// ParseRequestValue extracts a Version from a raw request header value.
// Accepted forms are "<service> <version>", a bare "<version>", or the special
// keyword "latest". When the service token is configured it must match.
//
// Arguments:
//   - value: The raw header value provided by the client.
//
// Returns:
//   - Version: The parsed version (zero when latest is requested).
//   - bool: True if the client explicitly requested the latest version.
//   - error: Non-nil if the value is malformed or the service token mismatches.
func (h HeaderConfig) ParseRequestValue(value string) (Version, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Version{}, false, fmt.Errorf("version-intercept: empty request version")
	}

	fields := strings.Fields(value)
	switch len(fields) {
	case 1:
		if strings.EqualFold(fields[0], LatestKeyword) {
			return Version{}, true, nil
		}
		v, err := ParseVersion(fields[0])
		return v, false, err
	case 2:
		if h.Service != "" && !strings.EqualFold(fields[0], h.Service) {
			return Version{}, false, fmt.Errorf("version-intercept: service %q does not match configured %q", fields[0], h.Service)
		}
		if strings.EqualFold(fields[1], LatestKeyword) {
			return Version{}, true, nil
		}
		v, err := ParseVersion(fields[1])
		return v, false, err
	default:
		return Version{}, false, fmt.Errorf("version-intercept: malformed version header %q", value)
	}
}

// FormatResponseValue renders the response header value for a negotiated
// version, prefixing the service token when configured.
//
// Arguments:
//   - v: The negotiated version to report.
//
// Returns the formatted header value.
func (h HeaderConfig) FormatResponseValue(v Version) string {
	if h.Service != "" {
		return h.Service + " " + v.String()
	}
	return v.String()
}
