package engine

import (
	"reflect"
	"testing"
)

// withEmptyRegistry swaps in a fresh, empty variant registry for the duration of
// a test and restores the real one afterwards, so registry-mutating tests do not
// leak entries into (or depend on) the package's init-time registrations.
func withEmptyRegistry(t *testing.T) {
	t.Helper()
	registryMu.Lock()
	saved := registry
	registry = map[string]Variant{}
	registryMu.Unlock()
	t.Cleanup(func() {
		registryMu.Lock()
		registry = saved
		registryMu.Unlock()
	})
}

// mustPanic runs fn and fails the test unless it panics.
func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: expected panic, got none", what)
		}
	}()
	fn()
}

func TestRegisterGetList(t *testing.T) {
	withEmptyRegistry(t)

	alpha := NewStandard()
	beta := NewStandard()
	Register("alpha", alpha)
	Register("beta", beta)

	// Get returns the exact registered instance.
	got, err := Get("alpha")
	if err != nil {
		t.Fatalf("Get(alpha): %v", err)
	}
	if got != Variant(alpha) {
		t.Errorf("Get(alpha) returned a different instance than was registered")
	}

	// List returns every registered name, sorted.
	if want := []string{"alpha", "beta"}; !reflect.DeepEqual(List(), want) {
		t.Errorf("List() = %v, want %v", List(), want)
	}
}

func TestGetUnknownVariant(t *testing.T) {
	withEmptyRegistry(t)
	Register("alpha", NewStandard())

	if _, err := Get("does-not-exist"); err == nil {
		t.Error("Get(unknown) = nil error, want error")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	withEmptyRegistry(t)
	Register("dup", NewStandard())
	mustPanic(t, "duplicate registration", func() { Register("dup", NewStandard()) })
}

func TestRegisterEmptyNamePanics(t *testing.T) {
	withEmptyRegistry(t)
	mustPanic(t, "empty name", func() { Register("", NewStandard()) })
}

func TestRegisterNilVariantPanics(t *testing.T) {
	withEmptyRegistry(t)
	mustPanic(t, "nil variant", func() { Register("nilv", nil) })
}

// TestStandardRegisteredByInit checks the package-init registration: "standard"
// is reachable via Get and appears in List without any test setup.
func TestStandardRegisteredByInit(t *testing.T) {
	v, err := Get("standard")
	if err != nil {
		t.Fatalf("Get(standard): %v", err)
	}
	if v.Name() != "standard" {
		t.Errorf("Get(standard).Name() = %q, want %q", v.Name(), "standard")
	}
	found := false
	for _, n := range List() {
		if n == "standard" {
			found = true
		}
	}
	if !found {
		t.Errorf("List() = %v, missing %q", List(), "standard")
	}
}
