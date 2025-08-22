package wire

import (
	"testing"
)

// TestParseCommand_ValidCases validates parsing of properly formatted protocol messages
// including all command types with various dependency configurations.
func TestParseCommand_ValidCases(t *testing.T) {
	tests := []struct {
		input    string
		expected *Command
	}{
		{
			input: "INDEX|package1|dep1,dep2\n",
			expected: &Command{
				Type:         IndexCommand,
				Package:      "package1",
				Dependencies: []string{"dep1", "dep2"},
			},
		},
		{
			input: "REMOVE|package1|\n",
			expected: &Command{
				Type:         RemoveCommand,
				Package:      "package1",
				Dependencies: nil,
			},
		},
		{
			input: "QUERY|package1|\n",
			expected: &Command{
				Type:         QueryCommand,
				Package:      "package1",
				Dependencies: nil,
			},
		},
		{
			input: "INDEX|package1|\n", // No dependencies
			expected: &Command{
				Type:         IndexCommand,
				Package:      "package1",
				Dependencies: nil,
			},
		},
		{
			input: "INDEX|pkg|dep1,dep2,\n", // Trailing comma
			expected: &Command{
				Type:         IndexCommand,
				Package:      "pkg",
				Dependencies: []string{"dep1", "dep2"},
			},
		},
	}

	for _, test := range tests {
		cmd, err := ParseCommand(test.input)
		if err != nil {
			t.Errorf("ParseCommand(%q) returned error: %v", test.input, err)
			continue
		}

		if cmd.Type != test.expected.Type {
			t.Errorf("ParseCommand(%q) Type = %v, expected %v", test.input, cmd.Type, test.expected.Type)
		}

		if cmd.Package != test.expected.Package {
			t.Errorf("ParseCommand(%q) Package = %q, expected %q", test.input, cmd.Package, test.expected.Package)
		}

		if len(cmd.Dependencies) != len(test.expected.Dependencies) {
			t.Errorf("ParseCommand(%q) Dependencies length = %d, expected %d",
				test.input, len(cmd.Dependencies), len(test.expected.Dependencies))
			continue
		}

		for i, dep := range cmd.Dependencies {
			if dep != test.expected.Dependencies[i] {
				t.Errorf("ParseCommand(%q) Dependencies[%d] = %q, expected %q",
					test.input, i, dep, test.expected.Dependencies[i])
			}
		}
	}
}

// TestParseCommand_ErrorCases validates proper error handling for malformed protocol messages
// including invalid commands, missing fields, and format violations.
func TestParseCommand_ErrorCases(t *testing.T) {
	errorCases := []string{
		"INVALID|package|\n",         // Invalid command
		"INDEX||\n",                  // Empty package name
		"INDEX\n",                    // Missing parts
		"INDEX|package\n",            // Missing third part
		"INDEX|package|deps|extra\n", // Too many parts
		"",                           // Empty line
		"INDEX|package|deps",         // Missing newline
	}

	for _, input := range errorCases {
		_, err := ParseCommand(input)
		if err == nil {
			t.Errorf("ParseCommand(%q) should have returned an error", input)
		}
	}
}

// TestResponse_String validates that response codes generate correct protocol-compliant
// strings with proper newline termination.
func TestResponse_String(t *testing.T) {
	tests := []struct {
		response Response
		expected string
	}{
		{OK, OK.String()},
		{FAIL, FAIL.String()},
		{ERROR, ERROR.String()},
		{Response(999), ERROR.String()}, // Test default case
	}

	for _, test := range tests {
		result := test.response.String()
		if result != test.expected {
			t.Errorf("Response(%v).String() = %q, expected %q", test.response, result, test.expected)
		}
	}
}

// TestCommandType_String validates string representation of command types
// including handling of unknown command values.
func TestCommandType_String(t *testing.T) {
	tests := []struct {
		cmdType  CommandType
		expected string
	}{
		{IndexCommand, "INDEX"},
		{RemoveCommand, "REMOVE"},
		{QueryCommand, "QUERY"},
		{CommandType(999), "UNKNOWN"}, // Test default case
	}

	for _, test := range tests {
		result := test.cmdType.String()
		if result != test.expected {
			t.Errorf("CommandType(%v).String() = %q, expected %q", test.cmdType, result, test.expected)
		}
	}
}
