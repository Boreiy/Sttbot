// Package shared contains common error types and utilities.
package shared

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// Common domain errors that can be used across the application
var (
	// ErrNotFound indicates that a requested resource was not found
	ErrNotFound = errors.New("not found")

	// ErrValidation indicates that input validation failed
	ErrValidation = errors.New("validation failed")

	// ErrUnauthorized indicates that the request lacks valid authentication
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates that the request is understood but forbidden
	ErrForbidden = errors.New("forbidden")

	// ErrConflict indicates that the request conflicts with current state
	ErrConflict = errors.New("conflict")

	// ErrInternal indicates an internal server error
	ErrInternal = errors.New("internal error")

	// ErrTimeout indicates that an operation timed out
	ErrTimeout = errors.New("operation timed out")

	// ErrInvariantViolated indicates that a business rule was violated
	ErrInvariantViolated = errors.New("invariant violated")

	// ErrDependencyFailure indicates that an external dependency failed
	ErrDependencyFailure = errors.New("dependency failure")
)

// Kind represents a category of error for easier classification and handling.
type Kind int

const (
	// KindUnknown represents an unclassified error
	KindUnknown Kind = iota
	// KindNotFound represents resource not found errors
	KindNotFound
	// KindValidation represents input validation errors
	KindValidation
	// KindUnauthorized represents authentication errors
	KindUnauthorized
	// KindForbidden represents authorization errors
	KindForbidden
	// KindConflict represents resource conflict errors
	KindConflict
	// KindInternal represents internal server errors
	KindInternal
	// KindTimeout represents timeout errors
	KindTimeout
	// KindInvariantViolated represents business rule violations
	KindInvariantViolated
	// KindDependencyFailure represents external dependency failures
	KindDependencyFailure
	// KindCanceled represents context cancellation
	KindCanceled
)

// String returns the string representation of the Kind.
func (k Kind) String() string {
	switch k {
	case KindNotFound:
		return "NotFound"
	case KindValidation:
		return "Validation"
	case KindUnauthorized:
		return "Unauthorized"
	case KindForbidden:
		return "Forbidden"
	case KindConflict:
		return "Conflict"
	case KindInternal:
		return "Internal"
	case KindTimeout:
		return "Timeout"
	case KindInvariantViolated:
		return "InvariantViolated"
	case KindDependencyFailure:
		return "DependencyFailure"
	case KindCanceled:
		return "Canceled"
	default:
		return "Unknown"
	}
}

// kindToSentinel maps error kinds to their corresponding sentinel errors.
var kindToSentinel = map[Kind]error{
	KindNotFound:          ErrNotFound,
	KindValidation:        ErrValidation,
	KindUnauthorized:      ErrUnauthorized,
	KindForbidden:         ErrForbidden,
	KindConflict:          ErrConflict,
	KindInternal:          ErrInternal,
	KindTimeout:           ErrTimeout,
	KindInvariantViolated: ErrInvariantViolated,
	KindDependencyFailure: ErrDependencyFailure,
}

// kindPriorities defines the deterministic order for error classification.
// Higher priority (lower index) kinds are checked first in KindOf.
var kindPriorities = []struct {
	kind Kind
	err  error
}{
	{KindCanceled, nil},       // context.Canceled (special case)
	{KindTimeout, ErrTimeout}, // timeout errors have high priority
	{KindNotFound, ErrNotFound},
	{KindValidation, ErrValidation},
	{KindUnauthorized, ErrUnauthorized},
	{KindForbidden, ErrForbidden},
	{KindConflict, ErrConflict},
	{KindDependencyFailure, ErrDependencyFailure}, // dependency failures should be visible
	{KindInternal, ErrInternal},
	{KindInvariantViolated, ErrInvariantViolated},
}

// KindOf returns the Kind of the given error by checking against known sentinel errors.
// It traverses the error chain to find the root classification using a deterministic priority order.
//
// The classification priority (highest to lowest):
//  1. KindCanceled (context.Canceled)
//  2. KindTimeout (context.DeadlineExceeded, ErrTimeout, net timeout errors)
//  3. KindNotFound, KindValidation, KindUnauthorized, KindForbidden, KindConflict
//  4. KindDependencyFailure (external dependencies have higher visibility than internal errors)
//  5. KindInternal, KindInvariantViolated (lowest priority)
//
// For errors created with errors.Join, the first matching kind in priority order is returned.
// Returns KindUnknown for unrecognized errors.
//
// Example:
//
//	switch shared.KindOf(err) {
//	case shared.KindNotFound:
//	    return http.StatusNotFound
//	case shared.KindValidation:
//	    return http.StatusBadRequest
//	case shared.KindTimeout:
//	    return http.StatusRequestTimeout
//	default:
//	    return http.StatusInternalServerError
//	}
func KindOf(err error) Kind {
	if err == nil {
		return KindUnknown
	}

	// Check kinds in priority order (deterministic)
	for _, priority := range kindPriorities {
		switch priority.kind {
		case KindCanceled:
			if IsCanceled(err) {
				return KindCanceled
			}
		case KindTimeout:
			if IsTimeout(err) {
				return KindTimeout
			}
		default:
			if priority.err != nil && errors.Is(err, priority.err) {
				return priority.kind
			}
		}
	}

	return KindUnknown
}

// HasKind reports whether the given error has the specified kind.
// It is equivalent to KindOf(err) == kind but provides a more explicit API.
// This is particularly useful for checking special kinds like KindCanceled and KindTimeout
// that have specific detection logic beyond simple errors.Is checks.
//
// Example:
//
//	if shared.HasKind(err, shared.KindTimeout) {
//	    // Handle timeout specifically
//	}
//	if shared.HasKind(err, shared.KindNotFound) {
//	    // Handle not found
//	}
func HasKind(err error, kind Kind) bool {
	return KindOf(err) == kind
}

// ErrorOf returns the sentinel error for the given Kind.
// For KindUnknown and KindCanceled, it returns nil.
func ErrorOf(kind Kind) error {
	if sentinel, exists := kindToSentinel[kind]; exists {
		return sentinel
	}
	return nil
}

// SentinelOf is an alias for ErrorOf that provides a more intuitive name.
// It returns the sentinel error for the given Kind.
// For KindUnknown and KindCanceled, it returns nil.
//
// Recommended usage: prefer SentinelOf over ErrorOf for better API clarity.
func SentinelOf(kind Kind) error {
	return ErrorOf(kind)
}

// MarkKind wraps an error with the appropriate sentinel error for the given kind,
// preserving the original error through error wrapping.
// This allows both KindOf(MarkKind(err, kind)) == kind and errors.Is(MarkKind(err, kind), err) to be true.
// If err is nil, returns the sentinel error for the kind (or nil for unsupported kinds).
// If kind is KindUnknown or KindCanceled, returns the original error unchanged.
//
// This function is idempotent: marking an error with a kind it already has returns the error unchanged.
//
// Example usage for adapting third-party errors:
//
//	// Adapt database errors to domain errors
//	if err := db.GetUser(id); err != nil {
//	    if errors.Is(err, sql.ErrNoRows) {
//	        return shared.MarkKind(err, shared.KindNotFound)
//	    }
//	    return shared.MarkKind(err, shared.KindInternal)
//	}
//
//	// Adapt HTTP client errors
//	resp, err := client.Get(url)
//	if err != nil {
//	    if isTimeoutError(err) {
//	        return shared.MarkKind(err, shared.KindTimeout)
//	    }
//	    return shared.MarkKind(err, shared.KindDependencyFailure)
//	}
func MarkKind(err error, kind Kind) error {
	// Handle nil error
	if err == nil {
		return ErrorOf(kind)
	}

	// Special handling for kinds without sentinel errors
	switch kind {
	case KindUnknown:
		return err // no marking needed
	case KindCanceled:
		// For canceled, we typically don't mark - return original
		return err
	}

	// Get the sentinel error for this kind
	sentinel := ErrorOf(kind)
	if sentinel == nil {
		return err // unknown kind, return unchanged
	}

	// If the error already has this kind, return as-is to avoid double wrapping
	if KindOf(err) == kind {
		return err
	}

	// Wrap with the sentinel error
	return fmt.Errorf("%w: %w", sentinel, err)
}

// Wrap wraps an error with additional context.
// It returns a new error that formats as "context: err".
// If err is nil, Wrap returns nil.
// If context is empty, returns the original error.
func Wrap(err error, context string) error {
	if err == nil {
		return nil
	}
	if context == "" {
		return err
	}
	return fmt.Errorf("%s: %w", context, err)
}

// Wrapf wraps an error with a formatted context message.
// It returns a new error that formats as "context: err".
// If err is nil, Wrapf returns nil.
// If formatted context is empty, returns the original error.
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	context := fmt.Sprintf(format, args...)
	if context == "" {
		return err
	}
	return fmt.Errorf("%s: %w", context, err)
}

// Invariant checks a condition and returns an error if it's false.
// This is useful for domain invariant validation.
func Invariant(condition bool, message string) error {
	if condition {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvariantViolated, message)
}

// InvariantF checks a condition and returns a formatted error if it's false.
func InvariantF(condition bool, format string, args ...interface{}) error {
	if condition {
		return nil
	}
	message := fmt.Sprintf(format, args...)
	return fmt.Errorf("%w: %s", ErrInvariantViolated, message)
}

// IsCanceled reports whether the error indicates a canceled context.
// It checks for context.Canceled and other cancellation-related errors.
func IsCanceled(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled)
}

// IsTimeout reports whether the error indicates a timeout.
// It checks for context.DeadlineExceeded, net.Error timeouts, and our ErrTimeout.
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}

	// Check for standard timeout errors
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrTimeout) {
		return true
	}

	// Check for network timeout errors
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return false
}

// IsNotFound reports whether the error indicates a resource not found condition.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsValidation reports whether the error indicates input validation failure.
func IsValidation(err error) bool {
	return errors.Is(err, ErrValidation)
}

// IsUnauthorized reports whether the error indicates lack of valid authentication.
func IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}

// IsForbidden reports whether the error indicates that the request is forbidden.
func IsForbidden(err error) bool {
	return errors.Is(err, ErrForbidden)
}

// IsConflict reports whether the error indicates a resource conflict.
func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}

// IsInternal reports whether the error indicates an internal server error.
func IsInternal(err error) bool {
	return errors.Is(err, ErrInternal)
}

// IsInvariantViolated reports whether the error indicates a business rule violation.
func IsInvariantViolated(err error) bool {
	return errors.Is(err, ErrInvariantViolated)
}

// IsDependencyFailure reports whether the error indicates an external dependency failure.
func IsDependencyFailure(err error) bool {
	return errors.Is(err, ErrDependencyFailure)
}

// Cause returns the underlying cause of the error by repeatedly unwrapping it.
// For simple wrap chains, returns the deepest cause.
// For errors.Join, returns the first root cause found in depth-first order.
// If the error doesn't wrap anything, it returns the error itself.
// If err is nil, Cause returns nil.
func Cause(err error) error {
	if err == nil {
		return nil
	}

	// Use UnwrapAll to flatten the error graph, then find the first leaf
	all := UnwrapAll(err)
	if len(all) == 0 {
		return err
	}

	// The last error in the flattened list is typically the root cause
	// For Join errors, this gives us a deterministic first leaf
	for i := len(all) - 1; i >= 0; i-- {
		candidate := all[i]

		// Check if this error has no further nested errors
		hasNested := false
		if unwrapper, ok := candidate.(interface{ Unwrap() []error }); ok {
			hasNested = len(unwrapper.Unwrap()) > 0
		} else {
			hasNested = errors.Unwrap(candidate) != nil
		}

		if !hasNested {
			return candidate
		}
	}

	// Fallback: return the original error if no leaf found
	return err
}

// UnwrapAll returns all errors in the error chain, from outermost to innermost.
// The first element is the original error, and the remaining are causes.
// For errors created with errors.Join, this flattens the entire error graph.
// If err is nil, returns nil slice.
func UnwrapAll(err error) []error {
	if err == nil {
		return nil
	}

	var result []error
	seen := make(map[error]bool) // prevent infinite loops
	queue := []error{err}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Skip if we've already seen this error (cycle protection)
		if seen[current] {
			continue
		}
		seen[current] = true
		result = append(result, current)

		// Check for both single and multiple unwrap methods
		if unwrapper, ok := current.(interface{ Unwrap() []error }); ok {
			// Multiple errors (errors.Join case)
			nested := unwrapper.Unwrap()
			queue = append(queue, nested...)
		} else if nested := errors.Unwrap(current); nested != nil {
			// Single error (fmt.Errorf %w case)
			queue = append(queue, nested)
		}
	}

	return result
}
