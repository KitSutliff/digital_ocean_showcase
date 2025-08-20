package main

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// MakeIndexMessage Generates a message to index this package
func MakeIndexMessage(pkg *Package) string {
	dependenciesNames := []string{}

	for _, dep := range pkg.Dependencies {
		dependenciesNames = append(dependenciesNames, dep.Name)
	}

	namesAsString := strings.Join(dependenciesNames, ",")
	return fmt.Sprintf("INDEX|%s|%s", pkg.Name, namesAsString)
}

// MakeRemoveMessage generates a message to remove a pakcage from the server's index
func MakeRemoveMessage(pkg *Package) string {
	return fmt.Sprintf("REMOVE|%s|", pkg.Name)
}

// MakeQueryMessage generates a message to check if a package is currently indexed
func MakeQueryMessage(pkg *Package) string {
	return fmt.Sprintf("QUERY|%s|", pkg.Name)
}

var possibleInvalidCommands = []string{"BLINDEX", "REMOVES", "QUER", "LIZARD", "I"}
var possibleInvalidChars = []string{"=", "☃", " "}
var messageCounter int64

// MakeBrokenMessage returns a message that's somehow broken and should be rejected
// by the server
func MakeBrokenMessage() string {
	counter := atomic.AddInt64(&messageCounter, 1)

	// Deterministic but varied broken messages
	if counter%2 == 0 {
		// Syntax errors with guaranteed uniqueness
		invalidChar := possibleInvalidChars[counter%int64(len(possibleInvalidChars))]
		return fmt.Sprintf("INDEX|emacs%selisp-%d\n", invalidChar, counter)
	} else {
		// Invalid commands with guaranteed uniqueness
		invalidCommand := possibleInvalidCommands[counter%int64(len(possibleInvalidCommands))]
		return fmt.Sprintf("%s|package-%d|deps\n", invalidCommand, counter)
	}
}
