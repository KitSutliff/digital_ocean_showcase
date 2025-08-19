package wire

import (
	"testing"
)

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

func TestParseCommand_ErrorCases(t *testing.T) {
	errorCases := []string{
		"INVALID|package|\n",     // Invalid command
		"INDEX||\n",              // Empty package name
		"INDEX\n",                // Missing parts
		"INDEX|package\n",        // Missing third part
		"INDEX|package|deps|extra\n", // Too many parts
		"",                       // Empty line
		"INDEX|package|deps",     // Missing newline
	}
	
	for _, input := range errorCases {
		_, err := ParseCommand(input)
		if err == nil {
			t.Errorf("ParseCommand(%q) should have returned an error", input)
		}
	}
}

func TestResponse_String(t *testing.T) {
	tests := []struct {
		response Response
		expected string
	}{
		{OK, "OK\n"},
		{FAIL, "FAIL\n"},
		{ERROR, "ERROR\n"},
		{Response(999), "ERROR\n"}, // Test default case
	}
	
	for _, test := range tests {
		result := test.response.String()
		if result != test.expected {
			t.Errorf("Response(%v).String() = %q, expected %q", test.response, result, test.expected)
		}
	}
}

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

func TestValidateCommand(t *testing.T) {
	// Test ValidateCommand function (which currently does nothing but needs coverage)
	cmd := &Command{
		Type:         IndexCommand,
		Package:      "test",
		Dependencies: []string{"dep1"},
	}
	
	err := ValidateCommand(cmd)
	if err != nil {
		t.Errorf("ValidateCommand should return nil, got: %v", err)
	}
	
	// Test with nil command
	err = ValidateCommand(nil)
	if err != nil {
		t.Errorf("ValidateCommand with nil should return nil, got: %v", err)
	}
}
