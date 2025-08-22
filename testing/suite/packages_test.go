package main

import (
	"reflect"
	"testing"
)

// TestAllPackages_Named verifies that the Named method returns singleton instances
// for the same package name to maintain referential integrity across the test graph.
func TestAllPackages_Named(t *testing.T) {
	allPackages := AllPackages{}

	aPackage := allPackages.Named("pkg-a")
	theSamePackage := allPackages.Named("pkg-a")
	if aPackage != theSamePackage {
		t.Error("Returning different instances for same package")
	}
}

// TestAddingDependencies validates that package dependency relationships are correctly
// established and maintained through the AddDependency method.
func TestAddingDependencies(t *testing.T) {
	allPackages := AllPackages{}

	pkg1 := allPackages.Named("pkg-1")
	pkg2 := allPackages.Named("pkg-2")
	pkg3 := allPackages.Named("pkg-3")
	pkg4 := allPackages.Named("pkg-4")

	pkg1.AddDependency(pkg2)
	pkg2.AddDependency(pkg3)
	pkg2.AddDependency(pkg4)
	pkg3.AddDependency(pkg4)

	if !reflect.DeepEqual(pkg1.Dependencies, []*Package{pkg2}) {
		t.Errorf("pkg1 should depend on pkg2")
	}

	if !reflect.DeepEqual(pkg2.Dependencies, []*Package{pkg3, pkg4}) {
		t.Errorf("pkg2 should depend on pkg3 and pkg4")
	}

	if !reflect.DeepEqual(pkg3.Dependencies, []*Package{pkg4}) {
		t.Errorf("pkg3 should depend on pkg4")
	}

	if !reflect.DeepEqual(pkg4.Dependencies, []*Package{}) {
		t.Errorf("pkg4 shouldnt depend on anything")
	}
}

// TestTokeniseLine verifies parsing of dependency specification lines from the embedded
// test data format, including both packages with and without dependencies.
func TestTokeniseLine(t *testing.T) {
	lineWithoutDependencies := "a:"
	expectedTokens := []string{"a"}

	tokens, err := TokeniseLine(lineWithoutDependencies)

	if err != nil {
		t.Fatalf("err: %#v", err)
	}

	if !reflect.DeepEqual(tokens, expectedTokens) {
		t.Errorf("Couldn't parse package without dependencies: %#v != %#v", tokens, expectedTokens)
	}

	lineWithDependencies := "abcde:  autoconf  automake  cd-discid "
	expectedTokens = []string{"abcde", "autoconf", "automake", "cd-discid"}

	tokens, err = TokeniseLine(lineWithDependencies)

	if err != nil {
		t.Fatalf("err: %#v", err)
	}

	if !reflect.DeepEqual(tokens, expectedTokens) {
		t.Errorf("Couldn't parse package with dependencies: %#v != %#v", tokens, expectedTokens)
	}

	brokenLine := "missing tokens"
	_, err = TokeniseLine(brokenLine)

	if err == nil {
		t.Error("Didn't throw error on broken line")
	}
}

// TestTokensToPackage validates conversion of parsed tokens into Package objects
// with proper dependency relationship establishment.
func TestTokensToPackage(t *testing.T) {
	allPackages := &AllPackages{}

	packageName := "a"
	dependencies := []string{"b", "c"}

	tokensWithDependency := append([]string{packageName}, dependencies...)
	pkg, err := TokensToPackage(allPackages, tokensWithDependency)

	if err != nil {
		t.Fatalf("Didn't parse tokens correctly: %#v", err)
	}

	if pkg.Name != "a" {
		t.Errorf("Didn't give package correct name: %#v", pkg.Name)
	}

	actualNameForDependencies := []string{}
	for _, dep := range pkg.Dependencies {
		actualNameForDependencies = append(actualNameForDependencies, dep.Name)
	}

	if !reflect.DeepEqual(actualNameForDependencies, dependencies) {
		t.Errorf("Didn't parse dependencies correctly: %#v != %#v", actualNameForDependencies, dependencies)
	}

	allPackages = &AllPackages{}
	tokensWithoutDependency := []string{packageName}
	pkg, err = TokensToPackage(allPackages, tokensWithoutDependency)

	if err != nil {
		t.Fatalf("Didn't parse tokens correctly: %#v", err)
	}

	if pkg.Name != "a" {
		t.Errorf("Didn't give package correct name: %#v", pkg.Name)
	}

	if len(pkg.Dependencies) != 0 {
		t.Errorf("Should have zero dependencies, had %#v: %#v", len(pkg.Dependencies), pkg.Dependencies)
	}

	allPackages = &AllPackages{}
	_, err = TokensToPackage(allPackages, []string{})
	if err == nil {
		t.Error("Didn't return an error if no tokens sent")
	}

}

// TestTextToPackages verifies parsing of multi-line dependency specification text
// into a complete package graph with referential integrity.
func TestTextToPackages(t *testing.T) {
	allPackages := &AllPackages{}

	textWithInternalConsistency := `a: b c
b: c d e
c: d
d:
e: a
`
	_, err := TextToPackages(allPackages, textWithInternalConsistency)

	if err != nil {
		t.Fatalf("Error parsing internally consistent text: %#v", err)
	}

	expectedPackageNames := []string{"a", "b", "c", "d", "e"}
	actualPackageNames := allPackages.Names()
	if !reflect.DeepEqual(actualPackageNames, expectedPackageNames) {
		t.Errorf("Didn't parse internally consistent text: %#v != %#v", actualPackageNames, expectedPackageNames)
	}

	if len(allPackages.Named("a").Dependencies) != 2 {
		t.Errorf("Package a has weird dependencies: %#v", allPackages.Named("a").Dependencies)
	}

	if len(allPackages.Named("b").Dependencies) != 3 {
		t.Errorf("Package b has weird dependencies: %#v", allPackages.Named("b").Dependencies)
	}

	if len(allPackages.Named("c").Dependencies) != 1 {
		t.Errorf("Package c has weird dependencies: %#v", allPackages.Named("c").Dependencies)
	}

	if len(allPackages.Named("d").Dependencies) != 0 {
		t.Errorf("Package d has weird dependencies: %#v", allPackages.Named("d").Dependencies)
	}

	if len(allPackages.Named("e").Dependencies) != 1 {
		t.Errorf("Package e has weird dependencies: %#v", allPackages.Named("e").Dependencies)
	}

	allPackages = &AllPackages{}
	_, err = TextToPackages(allPackages, "")

	if err != nil {
		t.Errorf("Should error on empty text: %#v", err)
	}

	if len(allPackages.Packages) != 0 {
		t.Errorf("Shouldn't add any packages for empty text: %#v", allPackages.Packages)
	}

	textWithBrokenLines := `a: b c
z
b: c z
`
	allPackages = &AllPackages{}
	_, err = TextToPackages(allPackages, textWithBrokenLines)

	if err == nil {
		t.Errorf("Didn't detect broken line: %#v", allPackages.Packages)
	}
}

// TestParseBrewPackages validates parsing of the embedded Homebrew package data
// used for comprehensive integration testing scenarios.
func TestParseBrewPackages(t *testing.T) {
	allPackages := &AllPackages{}
	allPackages, err := BrewToPackages(allPackages)
	if err != nil {
		t.Errorf("Could not parse brew packages: %s", err)
	}

	if len(allPackages.Packages) == 0 {
		t.Errorf("Expected more than 0 packages in brew-dependencies.txt, found %d", len(allPackages.Packages))
	}
}

// TestSegmentListPackages verifies partitioning of package lists for concurrent
// client distribution during multi-threaded test execution.
func TestSegmentListPackages(t *testing.T) {
	allPackages := &AllPackages{}

	pkgA := allPackages.Named("a")
	pkgB := allPackages.Named("b")
	pkgC := allPackages.Named("c")

	list := []*Package{pkgA, pkgB, pkgC}

	expectedList := [][]*Package{list}
	actualList := SegmentListPackages(list, 0)
	if !reflect.DeepEqual(actualList, expectedList) {
		t.Errorf("Expected %v got %v", expectedList, actualList)
	}

	expectedList = [][]*Package{list}
	actualList = SegmentListPackages(list, 1)

	if !reflect.DeepEqual(actualList, expectedList) {
		t.Errorf("Expected %v got %v", expectedList, actualList)
	}

	expectedList = [][]*Package{[]*Package{pkgA}, []*Package{pkgB, pkgC}}
	actualList = SegmentListPackages(list, 2)
	if !reflect.DeepEqual(actualList, expectedList) {
		t.Errorf("Expected %v got %v", expectedList, actualList)
	}

	expectedList = [][]*Package{[]*Package{pkgA}, []*Package{pkgB}, []*Package{pkgC}}
	actualList = SegmentListPackages(list, 3)
	if !reflect.DeepEqual(actualList, expectedList) {
		t.Errorf("Expected %v got %v", expectedList, actualList)
	}

	expectedList = [][]*Package{[]*Package{pkgA}, []*Package{pkgB}, []*Package{pkgC}}
	actualList = SegmentListPackages(list, 4)
	if !reflect.DeepEqual(actualList, expectedList) {
		t.Errorf("Expected %v got %v", expectedList, actualList)
	}
}
