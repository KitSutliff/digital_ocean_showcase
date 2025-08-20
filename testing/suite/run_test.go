package main

import (
	"fmt"
	"testing"
)

type stubClient struct {
	WhatToReturn  ResponseCode
	NumberOfCalls int
	IsCosed       bool
}

// Name returns a hardcoded name
func (client stubClient) Name() string {
	return "stub"
}

// Close does nothing
func (client stubClient) Close() error {
	return nil
}

// Send returns the expected return value and increments the call count
func (client *stubClient) Send(msg string) (ResponseCode, error) {
	client.NumberOfCalls++
	return client.WhatToReturn, nil
}

func testBruteforceAction(
	t *testing.T,
	action func(client PackageIndexerClient, packages []*Package, changeOfBeingUnluckyInPercent int) error,
	expectedMessages int,
	actionName string,
) {
	// Test case with full package list
	allPackages := &AllPackages{}
	for i := 0; i < expectedMessages; i++ {
		allPackages.Named(fmt.Sprintf("pkg-%d", i))
	}

	// Test case with empty package list
	aStubClient := &stubClient{WhatToReturn: OK}
	err := action(aStubClient, []*Package{}, 0)
	if err != nil {
		t.Errorf("%s: Unexpected error for empty package list: %v", actionName, err)
	}
	if aStubClient.NumberOfCalls != 0 {
		t.Errorf("%s: Expected [0] calls for empty package list, got [%d]", actionName, aStubClient.NumberOfCalls)
	}

	// Test case with full package list
	aStubClient = &stubClient{WhatToReturn: OK}
	err = action(aStubClient, allPackages.Packages, 0)
	if err != nil {
		t.Errorf("%s: Unexpected error for full package list: %v", actionName, err)
	}
	if aStubClient.NumberOfCalls != expectedMessages {
		t.Errorf("%s: Expected [%d] calls, got [%d]", actionName, expectedMessages, aStubClient.NumberOfCalls)
	}
}

func TestBruteforceIndexesPackages(t *testing.T) {
	testBruteforceAction(t, bruteforceIndexesPackages, 20, "bruteforceIndexesPackages")
}

func TestBruteforceRemovesAllPackages(t *testing.T) {
	testBruteforceAction(t, bruteforceRemovesAllPackages, 200, "bruteforceRemovesAllPackages")
}

func TestVerifyAllPackages(t *testing.T) {
	allPackages := &AllPackages{}
	expectedMessages := 200
	for i := 0; i < expectedMessages; i++ {
		allPackages.Named(fmt.Sprintf("pkg-%d", i))
	}

	aStubClient := &stubClient{WhatToReturn: OK}

	verifyAllPackages(aStubClient, []*Package{}, OK, 0)

	if aStubClient.NumberOfCalls != 0 {
		t.Errorf("Expected [%d] calls, got [%d]", expectedMessages, aStubClient.NumberOfCalls)
	}

	aStubClient = &stubClient{WhatToReturn: OK}

	verifyAllPackages(aStubClient, allPackages.Packages, FAIL, 0)

	if aStubClient.NumberOfCalls != 1 {
		t.Errorf("Expected to stop after the first failed call, got [%d] calls", aStubClient.NumberOfCalls)
	}
}
