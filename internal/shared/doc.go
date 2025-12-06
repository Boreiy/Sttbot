// Package shared contains common error types and utilities for error handling
// across the application without domain-specific logic.
//
// # Error Types and Classification
//
// This package provides a set of standard error types (sentinel errors) that
// represent common failure conditions:
//
//   - ErrNotFound: Resource not found
//   - ErrValidation: Input validation failed
//   - ErrUnauthorized: Authentication required
//   - ErrForbidden: Access denied
//   - ErrConflict: Resource conflict
//   - ErrInternal: Internal server error
//   - ErrTimeout: Operation timed out
//   - ErrInvariantViolated: Business rule violation
//   - ErrDependencyFailure: External dependency failed
//
// # Error Classification
//
// Use KindOf() to classify errors into categories:
//
//	err := someOperation()
//	switch shared.KindOf(err) {
//	case shared.KindNotFound:
//	    // Handle not found
//	case shared.KindTimeout:
//	    // Handle timeout
//	default:
//	    // Handle other errors
//	}
//
// Or use predicate functions for cleaner code:
//
//	if shared.IsNotFound(err) {
//	    // Handle not found
//	}
//	if shared.IsTimeout(err) {
//	    // Handle timeout
//	}
//
// Or use the HasKind() function for explicit kind checking:
//
//	if shared.HasKind(err, shared.KindTimeout) {
//	    // Handle timeout specifically
//	}
//
// # Kind Priority Table
//
// When multiple error kinds are present (e.g., with errors.Join), KindOf returns the highest priority kind:
//
//	Priority | Kind                  | Description
//	---------|----------------------|--------------------
//	1        | KindCanceled         | Context cancellation (highest)
//	2        | KindTimeout          | Timeout/deadline errors
//	3        | KindNotFound         | Resource not found
//	4        | KindValidation       | Input validation failures
//	5        | KindUnauthorized     | Authentication required
//	6        | KindForbidden        | Access denied
//	7        | KindConflict         | Resource conflicts
//	8        | KindDependencyFailure| External service failures
//	9        | KindInternal         | Internal server errors
//	10       | KindInvariantViolated| Business rule violations (lowest)
//
// # Error Wrapping and Context
//
// Add context to errors while preserving the original error:
//
//	if err := repo.GetUser(id); err != nil {
//	    return shared.Wrap(err, "failed to get user")
//	}
//
// Use formatted wrapping for dynamic context:
//
//	if err := repo.GetUser(id); err != nil {
//	    return shared.Wrapf(err, "failed to get user %d", id)
//	}
//
// # Error Marking
//
// Mark errors with specific kinds while preserving the original error:
//
//	// Mark a database error as "not found"
//	if errors.Is(err, sql.ErrNoRows) {
//	    return shared.MarkKind(err, shared.KindNotFound)
//	}
//
//	// Now both work:
//	// shared.IsNotFound(markedErr) == true
//	// errors.Is(markedErr, sql.ErrNoRows) == true
//
// # Business Rule Validation
//
// Use Invariant functions for business rule validation:
//
//	if err := shared.Invariant(user.Age >= 18, "user must be 18 or older"); err != nil {
//	    return err
//	}
//
//	// Or with formatting:
//	if err := shared.InvariantF(len(password) >= 8, "password must be at least %d characters", 8); err != nil {
//	    return err
//	}
//
// # Error Unwrapping and Root Causes
//
// Get the root cause of wrapped errors:
//
//	rootErr := shared.Cause(err)
//
// Get all errors in the chain (supports both fmt.Errorf %w and errors.Join):
//
//	allErrors := shared.UnwrapAll(err)
//
// # Best Practices
//
// 1. Use sentinel errors for known conditions that callers might want to handle
// 2. Use Wrap/Wrapf to add context without losing the original error
// 3. Use MarkKind to classify third-party errors into your error taxonomy
// 4. Use predicate functions (IsNotFound, etc.) or HasKind for readable error checking
// 5. Don't expose infrastructure details (database errors, HTTP status codes) in error messages
// 6. Keep error messages lowercase and without punctuation for easy composition
// 7. Use Invariant functions for business rule validation
// 8. Map Kind to HTTP/GRPC codes in adapter layers, not in shared package
// 9. Prefer SentinelOf over ErrorOf for better API clarity
//
// # Error Message Style Guide
//
// - Use lowercase messages: "user not found" not "User not found"
// - Avoid punctuation: "invalid email format" not "Invalid email format."
// - Keep messages composable: they will often be wrapped with additional context
// - Use present tense: "cannot connect" not "could not connect"
//
// # Adapter Integration
//
// Map error kinds to transport-specific codes in adapter layers:
//
//	func (h *Handler) handleError(err error) (int, interface{}) {
//	    switch shared.KindOf(err) {
//	    case shared.KindNotFound:
//	        return http.StatusNotFound, ErrorResponse{Message: "resource not found"}
//	    case shared.KindValidation:
//	        return http.StatusBadRequest, ErrorResponse{Message: "invalid input"}
//	    case shared.KindTimeout:
//	        return http.StatusRequestTimeout, ErrorResponse{Message: "request timeout"}
//	    case shared.KindDependencyFailure:
//	        return http.StatusBadGateway, ErrorResponse{Message: "service unavailable"}
//	    default:
//	        return http.StatusInternalServerError, ErrorResponse{Message: "internal error"}
//	    }
//	}
//
// # Supported Go Versions
//
// This package supports errors.Join (available since Go 1.20) and provides
// deterministic error classification and unwrapping for complex error chains.
package shared
