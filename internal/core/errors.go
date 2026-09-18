// Package core holds the domain types and rules. It imports only the standard
// library: no storage, no transport, no logging.
package core

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned by stores when a requested entity does not exist.
var ErrNotFound = errors.New("core: not found")

// ValidationError describes a rejected field.
type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}
