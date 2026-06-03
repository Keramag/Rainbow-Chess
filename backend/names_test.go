package main

import (
	"regexp"
	"testing"
)

// nameRe matches the Adjective+Animal+NN format: one or more letters followed by
// exactly two digits.
var nameRe = regexp.MustCompile(`^[A-Za-z]+[0-9]{2}$`)

func TestGenerateRandomName_Format(t *testing.T) {
	for i := 0; i < 200; i++ {
		name := GenerateRandomName()
		if name == "" {
			t.Fatal("generated name is empty")
		}
		if !nameRe.MatchString(name) {
			t.Fatalf("name %q does not match Adjective+Animal+NN", name)
		}
	}
}

func TestGenerateRandomName_Varies(t *testing.T) {
	// Over many draws we should see more than one distinct name; a generator
	// stuck on a constant would be a real bug.
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		seen[GenerateRandomName()] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected varied names, only saw %d distinct in 50 draws", len(seen))
	}
}
