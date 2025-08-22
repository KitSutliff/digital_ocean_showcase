package main

import (
	"fmt"
	"strings"
	"sync/atomic"
)

const (
	// ProtocolSeparators defines the characters used in the wire protocol
	ProtocolSeparator   = "|"
	DependencySeparator = ","
)

// MakeIndexMessage Generates a message to index this package
func MakeIndexMessage(pkg *Package) string {
	dependenciesNames := []string{}

	for _, dep := range pkg.Dependencies {
		dependenciesNames = append(dependenciesNames, dep.Name)
	}

	namesAsString := strings.Join(dependenciesNames, DependencySeparator)
	return fmt.Sprintf("INDEX%s%s%s%s", ProtocolSeparator, pkg.Name, ProtocolSeparator, namesAsString)
}

// MakeRemoveMessage generates a message to remove a package from the server's index
func MakeRemoveMessage(pkg *Package) string {
	return fmt.Sprintf("REMOVE%s%s%s", ProtocolSeparator, pkg.Name, ProtocolSeparator)
}

// MakeQueryMessage generates a message to check if a package is currently indexed
func MakeQueryMessage(pkg *Package) string {
	return fmt.Sprintf("QUERY%s%s%s", ProtocolSeparator, pkg.Name, ProtocolSeparator)
}

// Chaos testing data for malformed message generation
var (
	possibleInvalidCommands = []string{"BLINDEX", "REMOVES", "QUER", "LIZARD", "I"}
	possibleInvalidChars    = []string{"=", "☃", " "}
	messageCounter          int64
)

// MakeBrokenMessage generates deterministically malformed protocol messages for chaos testing.
// Returns various types of invalid messages that should trigger ERROR responses from the server.
func MakeBrokenMessage() string {
	counter := atomic.AddInt64(&messageCounter, 1)

	// Deterministic but varied broken messages
	if counter%2 == 0 {
		// Syntax errors with guaranteed uniqueness
		invalidChar := possibleInvalidChars[counter%int64(len(possibleInvalidChars))]
		return fmt.Sprintf("INDEX|emacs%selisp-%d\n", invalidChar, counter)
	}
	// Invalid commands with guaranteed uniqueness
	invalidCommand := possibleInvalidCommands[counter%int64(len(possibleInvalidCommands))]
	return fmt.Sprintf("%s%spackage-%d%sdeps\n", invalidCommand, ProtocolSeparator, counter, ProtocolSeparator)
}
