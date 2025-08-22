package main

import "testing"

// TestMakeRemoveMessage validates generation of REMOVE protocol messages
// according to the wire format specification.
func TestMakeRemoveMessage(t *testing.T) {
	allPackages := AllPackages{}

	aPackage := allPackages.Named("a")

	expectedLine := "REMOVE|a|"
	actualLine := MakeRemoveMessage(aPackage)

	if actualLine != expectedLine {
		t.Errorf("Expected %#v to serialise to [%s], got [%s]", aPackage, expectedLine, actualLine)
	}

}

// TestMakeIndexMessage validates generation of INDEX protocol messages
// with proper dependency serialization for multi-dependency packages.
func TestMakeIndexMessage(t *testing.T) {
	allPackages := AllPackages{}

	aPackage := allPackages.Named("a")
	otherPackage := allPackages.Named("o")
	someOtherPackage := allPackages.Named("so")
	aPackage.AddDependency(otherPackage)
	aPackage.AddDependency(someOtherPackage)

	expectedLine := "INDEX|a|o,so"
	actualLine := MakeIndexMessage(aPackage)

	if actualLine != expectedLine {
		t.Errorf("Expected %#v to serialise to [%s], got [%s]", aPackage, expectedLine, actualLine)
	}

}

// TestMakeQueryMessage validates generation of QUERY protocol messages
// according to the wire format specification.
func TestMakeQueryMessage(t *testing.T) {
	allPackages := AllPackages{}

	aPackage := allPackages.Named("a")

	expectedLine := "QUERY|a|"
	actualLine := MakeQueryMessage(aPackage)

	if actualLine != expectedLine {
		t.Errorf("Expected %#v to serialise to [%s], got [%s]", aPackage, expectedLine, actualLine)
	}
}

// TestMakeBrokenMessage verifies generation of malformed protocol messages
// for chaos testing and server error handling validation.
func TestMakeBrokenMessage(t *testing.T) {
	oneLine := MakeBrokenMessage()
	otherLine := MakeBrokenMessage()
	if oneLine == otherLine {
		t.Errorf("Expected messages with different random seeds to be different, got [%s] and [%s]", oneLine, otherLine)
	}
}
