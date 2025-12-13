package shared_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

    "sttbot/internal/shared"
)

// Example_wrap demonstrates how to add context to errors while preserving the original error.
func Example_wrap() {
	// Simulate a database operation that fails
	var originalErr = sql.ErrNoRows

	// Add context while preserving the original error
	err := shared.Wrap(originalErr, "failed to find user")

	fmt.Println(err.Error())
	fmt.Println("Contains original error:", errors.Is(err, sql.ErrNoRows))

	// Output:
	// failed to find user: sql: no rows in result set
	// Contains original error: true
}

// Example_wrapf demonstrates formatted context wrapping.
func Example_wrapf() {
	userID := 123
	originalErr := errors.New("connection timeout")

	err := shared.Wrapf(originalErr, "failed to load user %d", userID)

	fmt.Println(err.Error())

	// Output:
	// failed to load user 123: connection timeout
}

// Example_markKind demonstrates how to classify third-party errors into domain error types.
func Example_markKind() {
	// Simulate adapting a database error to domain error
	dbErr := sql.ErrNoRows
	domainErr := shared.MarkKind(dbErr, shared.KindNotFound)

	fmt.Println("Error:", domainErr.Error())
	fmt.Println("Kind:", shared.KindOf(domainErr))
	fmt.Println("Is NotFound:", shared.IsNotFound(domainErr))
	fmt.Println("Contains original:", errors.Is(domainErr, sql.ErrNoRows))

	// Output:
	// Error: not found: sql: no rows in result set
	// Kind: NotFound
	// Is NotFound: true
	// Contains original: true
}

// Example_kindOf demonstrates error classification and priority handling.
func Example_kindOf() {
	// Single error
	timeoutErr := shared.ErrTimeout
	fmt.Println("Timeout error kind:", shared.KindOf(timeoutErr))

	// Wrapped error
	wrappedErr := shared.Wrap(shared.ErrNotFound, "user lookup failed")
	fmt.Println("Wrapped error kind:", shared.KindOf(wrappedErr))

	// Context errors
	fmt.Println("Canceled kind:", shared.KindOf(context.Canceled))
	fmt.Println("Deadline exceeded kind:", shared.KindOf(context.DeadlineExceeded))

	// Unknown error
	unknownErr := errors.New("some random error")
	fmt.Println("Unknown error kind:", shared.KindOf(unknownErr))

	// Output:
	// Timeout error kind: Timeout
	// Wrapped error kind: NotFound
	// Canceled kind: Canceled
	// Deadline exceeded kind: Timeout
	// Unknown error kind: Unknown
}

// Example_hasKind demonstrates the convenient HasKind predicate function.
func Example_hasKind() {
	err := shared.ErrTimeout

	fmt.Println("Has Timeout:", shared.HasKind(err, shared.KindTimeout))
	fmt.Println("Has NotFound:", shared.HasKind(err, shared.KindNotFound))

	// With joined errors (priority applies)
	joinedErr := errors.Join(shared.ErrNotFound, shared.ErrTimeout, shared.ErrValidation)
	fmt.Println("Joined has Timeout:", shared.HasKind(joinedErr, shared.KindTimeout))
	fmt.Println("Joined has NotFound:", shared.HasKind(joinedErr, shared.KindNotFound))

	// Output:
	// Has Timeout: true
	// Has NotFound: false
	// Joined has Timeout: true
	// Joined has NotFound: false
}

// Example_invariant demonstrates business rule validation.
func Example_invariant() {
	user := struct {
		Age   int
		Email string
	}{Age: 16, Email: "test@example.com"}

	// Check business rules
	if err := shared.Invariant(user.Age >= 18, "user must be at least 18 years old"); err != nil {
		fmt.Println("Validation failed:", err.Error())
		fmt.Println("Kind:", shared.KindOf(err))
	}

	// Formatted invariant
	if err := shared.InvariantF(len(user.Email) >= 5, "email must be at least %d characters", 5); err != nil {
		fmt.Println("This won't print - validation passes")
	} else {
		fmt.Println("Email validation passed")
	}

	// Output:
	// Validation failed: invariant violated: user must be at least 18 years old
	// Kind: InvariantViolated
	// Email validation passed
}

// Example_httpMapping demonstrates mapping error kinds to HTTP status codes in an adapter.
func Example_httpMapping() {
	// This would typically be in an HTTP adapter layer
	mapToHTTPStatus := func(err error) int {
		switch shared.KindOf(err) {
		case shared.KindNotFound:
			return http.StatusNotFound
		case shared.KindValidation:
			return http.StatusBadRequest
		case shared.KindUnauthorized:
			return http.StatusUnauthorized
		case shared.KindForbidden:
			return http.StatusForbidden
		case shared.KindConflict:
			return http.StatusConflict
		case shared.KindTimeout:
			return http.StatusRequestTimeout
		case shared.KindDependencyFailure:
			return http.StatusBadGateway
		case shared.KindInternal:
			return http.StatusInternalServerError
		default:
			return http.StatusInternalServerError
		}
	}

	// Example usage
	notFoundErr := shared.MarkKind(sql.ErrNoRows, shared.KindNotFound)
	timeoutErr := context.DeadlineExceeded
	validationErr := shared.ErrValidation

	fmt.Println("NotFound →", mapToHTTPStatus(notFoundErr))
	fmt.Println("Timeout →", mapToHTTPStatus(timeoutErr))
	fmt.Println("Validation →", mapToHTTPStatus(validationErr))

	// Output:
	// NotFound → 404
	// Timeout → 408
	// Validation → 400
}

// Example_cause demonstrates finding the root cause of complex error chains.
func Example_cause() {
	// Create a chain: base → wrapped → marked → wrapped again
	baseErr := errors.New("connection refused")
	wrappedErr := shared.Wrap(baseErr, "database connection failed")
	markedErr := shared.MarkKind(wrappedErr, shared.KindDependencyFailure)
	finalErr := shared.Wrap(markedErr, "user service unavailable")

	// Find the root cause
	rootCause := shared.Cause(finalErr)
	fmt.Println("Root cause:", rootCause.Error())
	fmt.Println("Is base error:", rootCause == baseErr)

	// With joined errors - note: specific root depends on implementation
	joinedErr := errors.Join(
		shared.Wrap(errors.New("error 1"), "wrapped 1"),
		shared.Wrap(errors.New("error 2"), "wrapped 2"),
	)
	joinedCause := shared.Cause(joinedErr)
	fmt.Printf("Joined has root cause: %t\n",
		joinedCause.Error() == "error 1" || joinedCause.Error() == "error 2")

	// Output:
	// Root cause: connection refused
	// Is base error: true
	// Joined has root cause: true
}

// Example_unwrapAll demonstrates getting all errors in a chain.
func Example_unwrapAll() {
	// Create a complex error chain
	base1 := errors.New("base error 1")
	base2 := errors.New("base error 2")

	wrapped1 := shared.Wrap(base1, "wrapped 1")
	wrapped2 := shared.Wrap(base2, "wrapped 2")

	joinedErr := errors.Join(wrapped1, wrapped2)
	finalErr := shared.Wrap(joinedErr, "final context")

	// Get all errors in the chain
	allErrors := shared.UnwrapAll(finalErr)

	fmt.Printf("Total errors in chain: %d\n", len(allErrors))
	fmt.Printf("First error starts with 'final context': %t\n",
		len(allErrors) > 0 && strings.HasPrefix(allErrors[0].Error(), "final context"))
	fmt.Printf("Contains base1: %v\n", containsError(allErrors, base1))
	fmt.Printf("Contains base2: %v\n", containsError(allErrors, base2))

	// Output:
	// Total errors in chain: 6
	// First error starts with 'final context': true
	// Contains base1: true
	// Contains base2: true
}

// Helper function for example
func containsError(errors []error, target error) bool {
	for _, err := range errors {
		if err == target {
			return true
		}
	}
	return false
}

// Example_priorityDemo demonstrates how error priorities work with errors.Join.
func Example_priorityDemo() {
	// Create errors with different priorities
	low1 := shared.ErrInternal
	low2 := shared.ErrInvariantViolated
	medium := shared.ErrDependencyFailure
	high := shared.ErrTimeout
	highest := context.Canceled

	// Test priority rules
	fmt.Println("=== Priority Examples ===")

	// High priority wins
	mixed1 := errors.Join(low1, high, medium)
	fmt.Printf("Internal + Timeout + Dependency → %s\n", shared.KindOf(mixed1))

	// Highest priority wins
	mixed2 := errors.Join(high, highest, medium)
	fmt.Printf("Timeout + Canceled + Dependency → %s\n", shared.KindOf(mixed2))

	// Among low priorities, dependency failure beats internal
	mixed3 := errors.Join(low1, medium, low2)
	fmt.Printf("Internal + Dependency + Invariant → %s\n", shared.KindOf(mixed3))

	// Output:
	// === Priority Examples ===
	// Internal + Timeout + Dependency → Timeout
	// Timeout + Canceled + Dependency → Canceled
	// Internal + Dependency + Invariant → DependencyFailure
}
