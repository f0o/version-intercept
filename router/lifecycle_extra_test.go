// lifecycle_extra_test.go covers small but behaviorally important helpers on the
// core router types: version ordering, method deprecation reporting, and header
// configuration/formatting. These round out the core package's own coverage.
package router

import "testing"

// TestVersionAtLeast verifies the AtLeast ordering helper.
func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.0", "1.9", true},
		{"1.0", "1.0", true},
		{"1.0", "1.1", false},
		{"2.1", "2.0", true},
	}
	for _, c := range cases {
		got := MustParseVersion(c.a).AtLeast(MustParseVersion(c.b))
		if got != c.want {
			t.Fatalf("%s.AtLeast(%s) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// TestMethodLifecycleIsDeprecated verifies deprecation reporting across the
// deprecation boundary and when no deprecation is declared.
func TestMethodLifecycleIsDeprecated(t *testing.T) {
	deprecated := MethodLifecycle{
		HasDeprecated: true,
		Deprecated:    MustParseVersion("2.0"),
	}
	if deprecated.IsDeprecated(MustParseVersion("1.9")) {
		t.Fatal("method should not be deprecated before its deprecated_in version")
	}
	if !deprecated.IsDeprecated(MustParseVersion("2.0")) {
		t.Fatal("method should be deprecated at its deprecated_in version")
	}
	if !deprecated.IsDeprecated(MustParseVersion("2.5")) {
		t.Fatal("method should remain deprecated after its deprecated_in version")
	}

	none := MethodLifecycle{}
	if none.IsDeprecated(MustParseVersion("9.9")) {
		t.Fatal("method without deprecated_in should never report deprecated")
	}
}

// TestWithHeaderConfigAndFormat verifies WithHeaderConfig overrides the router's
// header config and that FormatResponseValue honors the service token.
func TestWithHeaderConfigAndFormat(t *testing.T) {
	cfg := HeaderConfig{Service: "greeter"}
	r := New("ignored", WithHeaderConfig(cfg))
	if r.header.Service != "greeter" {
		t.Fatalf("WithHeaderConfig not applied: got %q", r.header.Service)
	}

	if got := cfg.FormatResponseValue(MustParseVersion("2.1")); got != "greeter 2.1" {
		t.Fatalf("FormatResponseValue with service: got %q, want %q", got, "greeter 2.1")
	}

	bare := HeaderConfig{}
	if got := bare.FormatResponseValue(MustParseVersion("2.1")); got != "2.1" {
		t.Fatalf("FormatResponseValue without service: got %q, want %q", got, "2.1")
	}
}
