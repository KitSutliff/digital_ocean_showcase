// Package wire implements the line-oriented TCP protocol for package dependency operations.
// Protocol format: "COMMAND|package|dependencies\n" with strict validation for
// compatibility with external test harnesses and production validation systems.
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

const (
	cmdIndexStr   = "INDEX"
	cmdRemoveStr  = "REMOVE"
	cmdQueryStr   = "QUERY"
	cmdUnknownStr = "UNKNOWN"
)

// String returns the string representation of a command type
func (ct CommandType) String() string {
	switch ct {
	case IndexCommand:
		return cmdIndexStr
	case RemoveCommand:
		return cmdRemoveStr
	case QueryCommand:
		return cmdQueryStr
	default:
		return cmdUnknownStr
	}
}

// Response represents server response codes
type Response int

// Response enumeration for type-safe response handling
const (
	OK Response = iota
	FAIL
	ERROR
)

// Protocol constants for wire format compliance and consistency
const (
	respOK    = "OK\n"
	respFAIL  = "FAIL\n"
	respERROR = "ERROR\n"

	ProtocolSeparator   = "|" // Separates command fields
	DependencySeparator = "," // Separates dependency lists
)

// String returns the protocol response string with required trailing newline.
// Ensures exact specification compliance for external systems and automated testing.
func (r Response) String() string {
	switch r {
	case OK:
		return respOK
	case FAIL:
		return respFAIL
	case ERROR:
		return respERROR
	default:
		return respERROR
	}
}

// ParseCommand parses a line into a Command using exact protocol specification.
// Format: "COMMAND|package|dependencies\n" with strict validation to prevent
// false negatives with external test harnesses.
func ParseCommand(line string) (*Command, error) {
	// Must end with newline per protocol specification
	if !strings.HasSuffix(line, "\n") {
		return nil, fmt.Errorf("line must end with newline")
	}

	// Remove trailing newline
	line = line[:len(line)-1]

	// Split by pipe - must have exactly 3 parts
	parts := strings.Split(line, ProtocolSeparator)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid format: expected 3 parts separated by |, got %d", len(parts))
	}

	cmdStr := parts[0]
	pkg := parts[1]
	depsStr := parts[2]

	// Parse command type
	var cmdType CommandType
	switch cmdStr {
	case cmdIndexStr:
		cmdType = IndexCommand
	case cmdRemoveStr:
		cmdType = RemoveCommand
	case cmdQueryStr:
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
		rawDeps := strings.Split(depsStr, DependencySeparator)
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
