package app

import (
	"fmt"

	"github.com/endermalkoc/cusp/internal/enums"
)

// ValidateEnum checks that value is in the allowed set. An empty value passes
// (optional fields); callers enforce required-ness separately with ValidateRequired.
func ValidateEnum(field, value string, allowed []string) error {
	if value == "" {
		return nil
	}
	if !enums.Valid(allowed, value) {
		return ValidationFailed(fmt.Errorf("invalid %s %q (allowed: %v)", field, value, allowed))
	}
	return nil
}

// ValidateRequired checks that value is non-empty.
func ValidateRequired(field, value string) error {
	if value == "" {
		return ValidationFailed(fmt.Errorf("%s is required", field))
	}
	return nil
}

// ValidateEnumSoft validates a SEED value-set leniently: an empty or known value
// passes silently; an unknown value is ACCEPTED but returns a human-readable note
// (so the caller can warn without blocking). When strict is true it behaves like
// ValidateEnum — an unknown value is a hard error. Used for the open seed sets
// (Domain.kind, Spec.kind, delivery_status, …) vs the hard ValidateEnum.
func ValidateEnumSoft(field, value string, allowed []string, strict bool) (note string, err error) {
	if value == "" || enums.Valid(allowed, value) {
		return "", nil
	}
	if strict {
		return "", ValidationFailed(fmt.Errorf("invalid %s %q (allowed: %v; omit --strict to accept)", field, value, allowed))
	}
	return fmt.Sprintf("note: %s %q is outside the known set %v (accepted; --strict would reject)", field, value, allowed), nil
}
