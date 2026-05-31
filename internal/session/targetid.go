// Package session defines the core session types shared across rdw subsystems.
package session

import (
	"fmt"
	"regexp"
)

const (
	TargetIDMaxLen = 64
)

var targetIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_ -]*$`)

// TargetID is a validated string identifier that maps a data stream to a Pane.
type TargetID string

// ParseTargetID validates and returns a TargetID or an error.
func ParseTargetID(s string) (TargetID, error) {
	if len(s) == 0 {
		return "", fmt.Errorf("target ID must not be empty")
	}

	if len(s) > TargetIDMaxLen {
		return "", fmt.Errorf("target ID %q exceeds maximum length of %d characters", s, TargetIDMaxLen)
	}

	if !targetIDPattern.MatchString(s) {
		return "", fmt.Errorf("target ID %q contains invalid characters: must match [a-zA-Z0-9_][a-zA-Z0-9_ -]*", s)
	}

	return TargetID(s), nil
}

// String returns the string representation of the TargetID.
func (t TargetID) String() string {
	return string(t)
}
