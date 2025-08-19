package wire

import (
	"fmt"
	"strings"
)

// Command represents a parsed client command
type Command struct {
	Type         CommandType
	Package      string
	Dependencies []string
}

// CommandType represents the type of command
type CommandType int

const (
	IndexCommand CommandType = iota
	RemoveCommand
	QueryCommand
)

// String returns the string representation of a command type
func (ct CommandType) String() string {
	switch ct {
	case IndexCommand:
		return "INDEX"
	case RemoveCommand:
		return "REMOVE"
	case QueryCommand:
		return "QUERY"
	default:
		return "UNKNOWN"
	}
}

// Response represents server response codes
type Response int

const (
	OK Response = iota
	FAIL
	ERROR
)

// String returns the protocol response string with newline
func (r Response) String() string {
	switch r {
	case OK:
		return "OK\n"
	case FAIL:
		return "FAIL\n"
	case ERROR:
		return "ERROR\n"
	default:
		return "ERROR\n"
	}
}

// ParseCommand parses a line into a Command using exact protocol specification
// Format: "COMMAND|package|dependencies\n"
func ParseCommand(line string) (*Command, error) {
	// Must end with newline (GPT-5's explicit check)
	if !strings.HasSuffix(line, "\n") {
		return nil, fmt.Errorf("line must end with newline")
	}
	
	// Remove trailing newline
	line = line[:len(line)-1]
	
	// Split by pipe - must have exactly 3 parts
	parts := strings.Split(line, "|")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid format: expected 3 parts separated by |, got %d", len(parts))
	}
	
	cmdStr := parts[0]
	pkg := parts[1]
	depsStr := parts[2]
	
	// Parse command type
	var cmdType CommandType
	switch cmdStr {
	case "INDEX":
		cmdType = IndexCommand
	case "REMOVE":
		cmdType = RemoveCommand
	case "QUERY":
		cmdType = QueryCommand
	default:
		return nil, fmt.Errorf("unknown command: %s", cmdStr)
	}
	
	// Validate package name (non-empty)
	if pkg == "" {
		return nil, fmt.Errorf("package name cannot be empty")
	}
	
	// Parse dependencies (comma-separated, empty allowed)
	var deps []string
	if depsStr != "" {
		rawDeps := strings.Split(depsStr, ",")
		for _, dep := range rawDeps {
			dep = strings.TrimSpace(dep)
			if dep != "" { // Ignore empty deps from trailing commas
				deps = append(deps, dep)
			}
		}
	}
	
	return &Command{
		Type:         cmdType,
		Package:      pkg,
		Dependencies: deps,
	}, nil
}

// ValidateCommand performs additional validation on a parsed command
// Note: Keep minimal to avoid over-validation that breaks test harness compatibility
func ValidateCommand(cmd *Command) error {
	// All validation is already done in ParseCommand to avoid over-validation
	// This function is kept for interface compatibility but does nothing
	return nil
}
