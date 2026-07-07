// Package router provides a microversion-aware router for gRPC/ConnectRPC
// services. It inspects an OpenStack-style microversion header on incoming
// requests and dispatches to the handler registered for the negotiated
// version, allowing multiple versions of a service to be served concurrently
// behind a single endpoint.
package router

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents an OpenStack-style microversion expressed as a
// major.minor pair. Ordering is defined lexicographically by (Major, Minor).
type Version struct {
	Major int
	Minor int
}

// ParseVersion parses a "major.minor" (or bare "major") microversion string.
// Surrounding whitespace is ignored.
//
// Arguments:
//   - s: The version string to parse, e.g. "2", "2.3".
//
// Returns the parsed Version or an error if the string is not a valid
// major[.minor] number.
func ParseVersion(s string) (Version, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Version{}, fmt.Errorf("version-intercept: empty version string")
	}

	parts := strings.SplitN(s, ".", 2)
	major, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || major < 0 {
		return Version{}, fmt.Errorf("version-intercept: invalid major version %q", s)
	}

	minor := 0
	if len(parts) == 2 {
		minor, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || minor < 0 {
			return Version{}, fmt.Errorf("version-intercept: invalid minor version %q", s)
		}
	}

	return Version{Major: major, Minor: minor}, nil
}

// MustParseVersion parses a microversion string and panics on error. It is
// intended for use with constant, known-good version literals such as those
// used when registering handlers.
//
// Arguments:
//   - s: The version string to parse.
//
// Returns the parsed Version.
func MustParseVersion(s string) Version {
	v, err := ParseVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

// String renders the version in canonical "major.minor" form.
func (v Version) String() string {
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor)
}

// Compare returns -1 if v is less than other, 0 if equal, and 1 if greater.
//
// Arguments:
//   - other: The version to compare against.
//
// Returns the ordering result as -1, 0, or 1.
func (v Version) Compare(other Version) int {
	switch {
	case v.Major != other.Major:
		if v.Major < other.Major {
			return -1
		}
		return 1
	case v.Minor != other.Minor:
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	default:
		return 0
	}
}

// LessThan reports whether v orders before other.
func (v Version) LessThan(other Version) bool { return v.Compare(other) < 0 }

// AtLeast reports whether v is greater than or equal to other.
func (v Version) AtLeast(other Version) bool { return v.Compare(other) >= 0 }

// Range is an inclusive span of versions [Min, Max] that a single handler
// supports. A zero Max value is treated as unbounded above.
type Range struct {
	Min Version
	Max Version
}

// unbounded reports whether the range has no explicit upper bound.
func (r Range) unbounded() bool { return r.Max == Version{} }

// Contains reports whether v falls within the inclusive range. A range with a
// zero Max has no upper bound.
//
// Arguments:
//   - v: The version to test for membership.
//
// Returns true if v is within [Min, Max].
func (r Range) Contains(v Version) bool {
	if v.LessThan(r.Min) {
		return false
	}
	if r.unbounded() {
		return true
	}
	return !r.Max.LessThan(v)
}
