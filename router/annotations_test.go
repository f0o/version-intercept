package router

import (
	"testing"
)

func TestMethodLifecycleFromGeneratedDescriptors(t *testing.T) {
	svc := fixtureService(t)
	hello := svc.Methods().ByName("Hello")
	farewell := svc.Methods().ByName("Farewell")

	helloLC := MethodLifecycleOf(hello)
	if !helloLC.HasIntroduced || helloLC.Introduced != (Version{1, 0}) {
		t.Errorf("Hello introduced = %+v, want 1.0", helloLC)
	}
	if !helloLC.Available(Version{1, 0}) || !helloLC.Available(Version{2, 0}) {
		t.Error("Hello should be available at 1.0 and 2.0")
	}

	farewellLC := MethodLifecycleOf(farewell)
	if !farewellLC.HasIntroduced || farewellLC.Introduced != (Version{2, 0}) {
		t.Errorf("Farewell introduced = %+v, want 2.0", farewellLC)
	}
	if farewellLC.Available(Version{1, 5}) {
		t.Error("Farewell should NOT be available at 1.5")
	}
	if !farewellLC.Available(Version{2, 0}) {
		t.Error("Farewell should be available at 2.0")
	}
}

func TestGateFieldsRequest(t *testing.T) {
	req := newFixtureMessage(t, "HelloRequest", map[string]string{
		"name": "Ada", "title": "Dr.", "language": "de",
	})

	// At 1.0, title and language are not yet introduced -> cleared.
	gateFields(req, Version{1, 0})
	if fixtureString(req, "name") != "Ada" {
		t.Errorf("name cleared unexpectedly: %q", fixtureString(req, "name"))
	}
	if fixtureString(req, "title") != "" || fixtureString(req, "language") != "" {
		t.Errorf("2.0 fields not gated at 1.0: title=%q language=%q", fixtureString(req, "title"), fixtureString(req, "language"))
	}

	// At 2.0, they are retained.
	req2 := newFixtureMessage(t, "HelloRequest", map[string]string{
		"name": "Ada", "title": "Dr.", "language": "de",
	})
	gateFields(req2, Version{2, 0})
	if fixtureString(req2, "title") != "Dr." || fixtureString(req2, "language") != "de" {
		t.Errorf("2.0 fields wrongly gated at 2.0: title=%q language=%q", fixtureString(req2, "title"), fixtureString(req2, "language"))
	}
}

func TestGateFieldsResponse(t *testing.T) {
	resp := newFixtureMessage(t, "HelloResponse", map[string]string{
		"message": "Hello, Ada", "served_by": "modern",
	})

	gateFields(resp, Version{1, 0})
	if fixtureString(resp, "message") != "Hello, Ada" {
		t.Errorf("message cleared unexpectedly: %q", fixtureString(resp, "message"))
	}
	if fixtureString(resp, "served_by") != "" {
		t.Errorf("served_by not gated at 1.0: %q", fixtureString(resp, "served_by"))
	}
}
