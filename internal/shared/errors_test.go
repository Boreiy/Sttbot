package shared_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

    "sttbot/internal/shared"
)

func TestWrap(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		context  string
		expected string
		isNil    bool
	}{
		{
			name:     "nil error",
			err:      nil,
			context:  "some context",
			expected: "",
			isNil:    true,
		},
		{
			name:     "simple error",
			err:      errors.New("original"),
			context:  "wrapper",
			expected: "wrapper: original",
			isNil:    false,
		},
		{
			name:     "empty context",
			err:      errors.New("original"),
			context:  "",
			expected: "original",
			isNil:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.Wrap(tt.err, tt.context)
			if tt.isNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected, result.Error())
				// Test that the original error is preserved
				assert.True(t, errors.Is(result, tt.err))
			}
		})
	}
}

func TestWrapf(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		format   string
		args     []interface{}
		expected string
		isNil    bool
	}{
		{
			name:     "nil error",
			err:      nil,
			format:   "context %d",
			args:     []interface{}{42},
			expected: "",
			isNil:    true,
		},
		{
			name:     "formatted context",
			err:      errors.New("original"),
			format:   "user %d operation %s",
			args:     []interface{}{123, "create"},
			expected: "user 123 operation create: original",
			isNil:    false,
		},
		{
			name:     "no format args",
			err:      errors.New("original"),
			format:   "simple context",
			args:     nil,
			expected: "simple context: original",
			isNil:    false,
		},
		{
			name:     "empty format result",
			err:      errors.New("original"),
			format:   "",
			args:     nil,
			expected: "original",
			isNil:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.Wrapf(tt.err, tt.format, tt.args...)
			if tt.isNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected, result.Error())
				assert.True(t, errors.Is(result, tt.err))
			}
		})
	}
}

func TestInvariant(t *testing.T) {
	tests := []struct {
		name      string
		condition bool
		message   string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "condition true",
			condition: true,
			message:   "should not fail",
			wantErr:   false,
		},
		{
			name:      "condition false",
			condition: false,
			message:   "custom message",
			wantErr:   true,
			errMsg:    "invariant violated: custom message",
		},
		{
			name:      "empty message",
			condition: false,
			message:   "",
			wantErr:   true,
			errMsg:    "invariant violated: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := shared.Invariant(tt.condition, tt.message)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				assert.True(t, errors.Is(err, shared.ErrInvariantViolated))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInvariantF(t *testing.T) {
	tests := []struct {
		name      string
		condition bool
		format    string
		args      []interface{}
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "condition true",
			condition: true,
			format:    "user %d not found",
			args:      []interface{}{123},
			wantErr:   false,
		},
		{
			name:      "condition false with format",
			condition: false,
			format:    "user %d with role %s",
			args:      []interface{}{123, "admin"},
			wantErr:   true,
			errMsg:    "invariant violated: user 123 with role admin",
		},
		{
			name:      "condition false no args",
			condition: false,
			format:    "simple message",
			args:      nil,
			wantErr:   true,
			errMsg:    "invariant violated: simple message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := shared.InvariantF(tt.condition, tt.format, tt.args...)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				assert.True(t, errors.Is(err, shared.ErrInvariantViolated))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSentinelErrors(t *testing.T) {
	// Test that all sentinel errors are different and not nil
	sentinelErrors := []error{
		shared.ErrNotFound,
		shared.ErrValidation,
		shared.ErrUnauthorized,
		shared.ErrForbidden,
		shared.ErrConflict,
		shared.ErrInternal,
		shared.ErrTimeout,
		shared.ErrInvariantViolated,
		shared.ErrDependencyFailure,
	}

	for i, err := range sentinelErrors {
		require.NotNil(t, err, "sentinel error %d should not be nil", i)
		require.NotEmpty(t, err.Error(), "sentinel error %d should have a message", i)

		// Check that each error is unique
		for j, other := range sentinelErrors {
			if i != j {
				assert.NotEqual(t, err, other, "sentinel errors %d and %d should be different", i, j)
			}
		}
	}
}

func TestIsCanceled(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: true,
		},
		{
			name:     "wrapped context canceled",
			err:      shared.Wrap(context.Canceled, "operation failed"),
			expected: true,
		},
		{
			name:     "other error",
			err:      shared.ErrNotFound,
			expected: false,
		},
		{
			name:     "timeout error",
			err:      context.DeadlineExceeded,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.IsCanceled(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestErrorPredicates(t *testing.T) {
	// Test all Is* predicates in a comprehensive way
	tests := []struct {
		name      string
		err       error
		predicate func(error) bool
		expected  bool
	}{
		// IsNotFound tests
		{"IsNotFound with ErrNotFound", shared.ErrNotFound, shared.IsNotFound, true},
		{"IsNotFound with wrapped ErrNotFound", shared.Wrap(shared.ErrNotFound, "wrapped"), shared.IsNotFound, true},
		{"IsNotFound with marked error", shared.MarkKind(errors.New("base"), shared.KindNotFound), shared.IsNotFound, true},
		{"IsNotFound with other error", shared.ErrValidation, shared.IsNotFound, false},
		{"IsNotFound with nil", nil, shared.IsNotFound, false},

		// IsValidation tests
		{"IsValidation with ErrValidation", shared.ErrValidation, shared.IsValidation, true},
		{"IsValidation with wrapped ErrValidation", shared.Wrap(shared.ErrValidation, "wrapped"), shared.IsValidation, true},
		{"IsValidation with marked error", shared.MarkKind(errors.New("base"), shared.KindValidation), shared.IsValidation, true},
		{"IsValidation with other error", shared.ErrNotFound, shared.IsValidation, false},
		{"IsValidation with nil", nil, shared.IsValidation, false},

		// IsUnauthorized tests
		{"IsUnauthorized with ErrUnauthorized", shared.ErrUnauthorized, shared.IsUnauthorized, true},
		{"IsUnauthorized with wrapped ErrUnauthorized", shared.Wrap(shared.ErrUnauthorized, "wrapped"), shared.IsUnauthorized, true},
		{"IsUnauthorized with marked error", shared.MarkKind(errors.New("base"), shared.KindUnauthorized), shared.IsUnauthorized, true},
		{"IsUnauthorized with other error", shared.ErrForbidden, shared.IsUnauthorized, false},
		{"IsUnauthorized with nil", nil, shared.IsUnauthorized, false},

		// IsForbidden tests
		{"IsForbidden with ErrForbidden", shared.ErrForbidden, shared.IsForbidden, true},
		{"IsForbidden with wrapped ErrForbidden", shared.Wrap(shared.ErrForbidden, "wrapped"), shared.IsForbidden, true},
		{"IsForbidden with marked error", shared.MarkKind(errors.New("base"), shared.KindForbidden), shared.IsForbidden, true},
		{"IsForbidden with other error", shared.ErrUnauthorized, shared.IsForbidden, false},
		{"IsForbidden with nil", nil, shared.IsForbidden, false},

		// IsConflict tests
		{"IsConflict with ErrConflict", shared.ErrConflict, shared.IsConflict, true},
		{"IsConflict with wrapped ErrConflict", shared.Wrap(shared.ErrConflict, "wrapped"), shared.IsConflict, true},
		{"IsConflict with marked error", shared.MarkKind(errors.New("base"), shared.KindConflict), shared.IsConflict, true},
		{"IsConflict with other error", shared.ErrInternal, shared.IsConflict, false},
		{"IsConflict with nil", nil, shared.IsConflict, false},

		// IsInternal tests
		{"IsInternal with ErrInternal", shared.ErrInternal, shared.IsInternal, true},
		{"IsInternal with wrapped ErrInternal", shared.Wrap(shared.ErrInternal, "wrapped"), shared.IsInternal, true},
		{"IsInternal with marked error", shared.MarkKind(errors.New("base"), shared.KindInternal), shared.IsInternal, true},
		{"IsInternal with other error", shared.ErrTimeout, shared.IsInternal, false},
		{"IsInternal with nil", nil, shared.IsInternal, false},

		// IsInvariantViolated tests
		{"IsInvariantViolated with ErrInvariantViolated", shared.ErrInvariantViolated, shared.IsInvariantViolated, true},
		{"IsInvariantViolated with wrapped ErrInvariantViolated", shared.Wrap(shared.ErrInvariantViolated, "wrapped"), shared.IsInvariantViolated, true},
		{"IsInvariantViolated with marked error", shared.MarkKind(errors.New("base"), shared.KindInvariantViolated), shared.IsInvariantViolated, true},
		{"IsInvariantViolated with other error", shared.ErrValidation, shared.IsInvariantViolated, false},
		{"IsInvariantViolated with nil", nil, shared.IsInvariantViolated, false},

		// IsDependencyFailure tests
		{"IsDependencyFailure with ErrDependencyFailure", shared.ErrDependencyFailure, shared.IsDependencyFailure, true},
		{"IsDependencyFailure with wrapped ErrDependencyFailure", shared.Wrap(shared.ErrDependencyFailure, "wrapped"), shared.IsDependencyFailure, true},
		{"IsDependencyFailure with marked error", shared.MarkKind(errors.New("base"), shared.KindDependencyFailure), shared.IsDependencyFailure, true},
		{"IsDependencyFailure with other error", shared.ErrInternal, shared.IsDependencyFailure, false},
		{"IsDependencyFailure with nil", nil, shared.IsDependencyFailure, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.predicate(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestErrorPredicatesWithJoin(t *testing.T) {
	// Test predicates with errors.Join
	baseErr := errors.New("base error")
	notFoundErr := shared.MarkKind(baseErr, shared.KindNotFound)
	validationErr := shared.MarkKind(errors.New("validation failed"), shared.KindValidation)

	joinedErr := errors.Join(notFoundErr, validationErr)

	// Should detect both kinds in joined error
	assert.True(t, shared.IsNotFound(joinedErr), "should detect NotFound in joined error")
	assert.True(t, shared.IsValidation(joinedErr), "should detect Validation in joined error")
	assert.False(t, shared.IsUnauthorized(joinedErr), "should not detect Unauthorized in joined error")
}

func TestIsTimeout(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "wrapped deadline exceeded",
			err:      shared.Wrap(context.DeadlineExceeded, "operation timed out"),
			expected: true,
		},
		{
			name:     "sentinel timeout error",
			err:      shared.ErrTimeout,
			expected: true,
		},
		{
			name:     "wrapped sentinel timeout",
			err:      shared.Wrap(shared.ErrTimeout, "request failed"),
			expected: true,
		},
		{
			name:     "network timeout error",
			err:      &timeoutError{},
			expected: true,
		},
		{
			name:     "wrapped network timeout",
			err:      shared.Wrap(&timeoutError{}, "network call failed"),
			expected: true,
		},
		{
			name:     "network non-timeout error",
			err:      &nonTimeoutNetError{},
			expected: false,
		},
		{
			name:     "canceled error",
			err:      context.Canceled,
			expected: false,
		},
		{
			name:     "other error",
			err:      shared.ErrNotFound,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.IsTimeout(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// timeoutError is a helper type for testing network timeout errors
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout error" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return false }

// nonTimeoutNetError is a helper type for testing non-timeout network errors
type nonTimeoutNetError struct{}

func (e *nonTimeoutNetError) Error() string   { return "network error" }
func (e *nonTimeoutNetError) Timeout() bool   { return false }
func (e *nonTimeoutNetError) Temporary() bool { return true }

func TestKindString(t *testing.T) {
	tests := []struct {
		kind     shared.Kind
		expected string
	}{
		{shared.KindUnknown, "Unknown"},
		{shared.KindNotFound, "NotFound"},
		{shared.KindValidation, "Validation"},
		{shared.KindUnauthorized, "Unauthorized"},
		{shared.KindForbidden, "Forbidden"},
		{shared.KindConflict, "Conflict"},
		{shared.KindInternal, "Internal"},
		{shared.KindTimeout, "Timeout"},
		{shared.KindInvariantViolated, "InvariantViolated"},
		{shared.KindDependencyFailure, "DependencyFailure"},
		{shared.KindCanceled, "Canceled"},
		{shared.Kind(999), "Unknown"}, // test unknown kind
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.kind.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKindOfDeterministic(t *testing.T) {
	// Test that KindOf returns consistent results in repeated calls
	// and follows priority order for complex error chains
	baseErr := errors.New("base error")
	wrappedWithTimeout := shared.Wrap(shared.ErrTimeout, "timeout wrapper")
	wrappedWithNotFound := shared.Wrap(shared.ErrNotFound, "not found wrapper")

	tests := []struct {
		name     string
		err      error
		expected shared.Kind
		reason   string
	}{
		{
			name:     "timeout has priority over other kinds",
			err:      shared.Wrap(wrappedWithTimeout, "outer wrapper"),
			expected: shared.KindTimeout,
			reason:   "timeout should be detected even when wrapped",
		},
		{
			name:     "canceled has highest priority",
			err:      shared.Wrap(context.Canceled, "operation canceled"),
			expected: shared.KindCanceled,
			reason:   "canceled should be detected with highest priority",
		},
		{
			name:     "not found detected when no higher priority errors",
			err:      wrappedWithNotFound,
			expected: shared.KindNotFound,
			reason:   "not found should be detected when no timeout/canceled present",
		},
		{
			name:     "unknown for non-sentinel errors",
			err:      baseErr,
			expected: shared.KindUnknown,
			reason:   "arbitrary errors should return KindUnknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to ensure deterministic behavior
			var results []shared.Kind
			for i := 0; i < 10; i++ {
				result := shared.KindOf(tt.err)
				results = append(results, result)
			}

			// All results should be identical
			for i, result := range results {
				assert.Equal(t, tt.expected, result,
					"iteration %d: %s. Got %s, expected %s",
					i, tt.reason, result.String(), tt.expected.String())
			}
		})
	}
}

func TestKindPriorities(t *testing.T) {
	// Test that KindOf follows expected priority order for errors.Join combinations
	// Priority order (highest to lowest): Canceled > Timeout > NotFound > Validation >
	// Unauthorized > Forbidden > Conflict > DependencyFailure > Internal > InvariantViolated

	tests := []struct {
		name     string
		errors   []error
		expected shared.Kind
		reason   string
	}{
		// Canceled has highest priority
		{
			name: "canceled beats timeout",
			errors: []error{
				shared.ErrTimeout,
				context.Canceled,
				shared.ErrNotFound,
			},
			expected: shared.KindCanceled,
			reason:   "canceled should have highest priority",
		},
		{
			name: "canceled beats all others",
			errors: []error{
				shared.ErrInternal,
				shared.ErrDependencyFailure,
				context.Canceled,
				shared.ErrValidation,
			},
			expected: shared.KindCanceled,
			reason:   "canceled should have highest priority over any combination",
		},
		// Timeout has second highest priority
		{
			name: "timeout beats not found",
			errors: []error{
				shared.ErrNotFound,
				shared.ErrTimeout,
			},
			expected: shared.KindTimeout,
			reason:   "timeout should beat not found",
		},
		{
			name: "timeout beats validation and below",
			errors: []error{
				shared.ErrValidation,
				shared.ErrUnauthorized,
				shared.ErrTimeout,
				shared.ErrInternal,
			},
			expected: shared.KindTimeout,
			reason:   "timeout should beat all lower priority kinds",
		},
		// Test middle priority ordering
		{
			name: "not found beats validation",
			errors: []error{
				shared.ErrValidation,
				shared.ErrNotFound,
			},
			expected: shared.KindNotFound,
			reason:   "not found should beat validation",
		},
		{
			name: "validation beats unauthorized",
			errors: []error{
				shared.ErrUnauthorized,
				shared.ErrValidation,
			},
			expected: shared.KindValidation,
			reason:   "validation should beat unauthorized",
		},
		{
			name: "unauthorized beats forbidden",
			errors: []error{
				shared.ErrForbidden,
				shared.ErrUnauthorized,
			},
			expected: shared.KindUnauthorized,
			reason:   "unauthorized should beat forbidden",
		},
		{
			name: "forbidden beats conflict",
			errors: []error{
				shared.ErrConflict,
				shared.ErrForbidden,
			},
			expected: shared.KindForbidden,
			reason:   "forbidden should beat conflict",
		},
		{
			name: "conflict beats dependency failure",
			errors: []error{
				shared.ErrDependencyFailure,
				shared.ErrConflict,
			},
			expected: shared.KindConflict,
			reason:   "conflict should beat dependency failure",
		},
		{
			name: "dependency failure beats internal",
			errors: []error{
				shared.ErrInternal,
				shared.ErrDependencyFailure,
			},
			expected: shared.KindDependencyFailure,
			reason:   "dependency failure should beat internal",
		},
		{
			name: "internal beats invariant violated",
			errors: []error{
				shared.ErrInvariantViolated,
				shared.ErrInternal,
			},
			expected: shared.KindInternal,
			reason:   "internal should beat invariant violated",
		},
		// Test complex combinations
		{
			name: "complex mix maintains timeout priority",
			errors: []error{
				shared.ErrInternal,
				shared.ErrNotFound,
				shared.ErrTimeout,
				shared.ErrValidation,
				shared.ErrDependencyFailure,
			},
			expected: shared.KindTimeout,
			reason:   "timeout should win in complex mix",
		},
		{
			name: "no high priority errors defaults to highest available",
			errors: []error{
				shared.ErrDependencyFailure,
				shared.ErrInternal,
				shared.ErrInvariantViolated,
			},
			expected: shared.KindDependencyFailure,
			reason:   "dependency failure should win among low priority errors",
		},
		// Test with wrapped errors
		{
			name: "wrapped errors maintain priority",
			errors: []error{
				shared.Wrap(shared.ErrInternal, "wrapped internal"),
				shared.Wrap(shared.ErrTimeout, "wrapped timeout"),
			},
			expected: shared.KindTimeout,
			reason:   "wrapped timeout should beat wrapped internal",
		},
		// Specific test for DependencyFailure > Internal priority change
		{
			name: "dependency failure beats internal (priority change)",
			errors: []error{
				shared.ErrInternal,
				shared.ErrDependencyFailure,
			},
			expected: shared.KindDependencyFailure,
			reason:   "dependency failure should have higher priority than internal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			joinedErr := errors.Join(tt.errors...)
			result := shared.KindOf(joinedErr)
			assert.Equal(t, tt.expected, result,
				"%s. Got %s, expected %s", tt.reason, result.String(), tt.expected.String())

			// Test determinism by running multiple times
			for i := 0; i < 5; i++ {
				reResult := shared.KindOf(joinedErr)
				assert.Equal(t, tt.expected, reResult,
					"iteration %d: result should be deterministic", i)
			}
		})
	}
}

func TestKindOf(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected shared.Kind
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: shared.KindUnknown,
		},
		{
			name:     "not found error",
			err:      shared.ErrNotFound,
			expected: shared.KindNotFound,
		},
		{
			name:     "wrapped not found",
			err:      shared.Wrap(shared.ErrNotFound, "user not found"),
			expected: shared.KindNotFound,
		},
		{
			name:     "validation error",
			err:      shared.ErrValidation,
			expected: shared.KindValidation,
		},
		{
			name:     "unauthorized error",
			err:      shared.ErrUnauthorized,
			expected: shared.KindUnauthorized,
		},
		{
			name:     "forbidden error",
			err:      shared.ErrForbidden,
			expected: shared.KindForbidden,
		},
		{
			name:     "conflict error",
			err:      shared.ErrConflict,
			expected: shared.KindConflict,
		},
		{
			name:     "internal error",
			err:      shared.ErrInternal,
			expected: shared.KindInternal,
		},
		{
			name:     "timeout error",
			err:      shared.ErrTimeout,
			expected: shared.KindTimeout,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: shared.KindTimeout,
		},
		{
			name:     "invariant violated",
			err:      shared.ErrInvariantViolated,
			expected: shared.KindInvariantViolated,
		},
		{
			name:     "dependency failure",
			err:      shared.ErrDependencyFailure,
			expected: shared.KindDependencyFailure,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: shared.KindCanceled,
		},
		{
			name:     "wrapped context canceled",
			err:      shared.Wrap(context.Canceled, "operation canceled"),
			expected: shared.KindCanceled,
		},
		{
			name:     "unknown error",
			err:      errors.New("some random error"),
			expected: shared.KindUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.KindOf(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMarkKind(t *testing.T) {
	baseErr := errors.New("base error")

	tests := []struct {
		name                  string
		err                   error
		kind                  shared.Kind
		expectedKind          shared.Kind
		shouldContainOriginal bool
		expectedNil           bool
	}{
		{
			name:                  "nil error with valid kind",
			err:                   nil,
			kind:                  shared.KindNotFound,
			expectedKind:          shared.KindNotFound,
			shouldContainOriginal: false,
			expectedNil:           false,
		},
		{
			name:                  "nil error with unknown kind",
			err:                   nil,
			kind:                  shared.KindUnknown,
			expectedKind:          shared.KindUnknown,
			shouldContainOriginal: false,
			expectedNil:           true,
		},
		{
			name:                  "mark error as not found",
			err:                   baseErr,
			kind:                  shared.KindNotFound,
			expectedKind:          shared.KindNotFound,
			shouldContainOriginal: true,
			expectedNil:           false,
		},
		{
			name:                  "mark error as validation",
			err:                   baseErr,
			kind:                  shared.KindValidation,
			expectedKind:          shared.KindValidation,
			shouldContainOriginal: true,
			expectedNil:           false,
		},
		{
			name:                  "mark with unknown kind returns unchanged",
			err:                   baseErr,
			kind:                  shared.KindUnknown,
			expectedKind:          shared.KindUnknown,
			shouldContainOriginal: true,
			expectedNil:           false,
		},
		{
			name:                  "mark with canceled kind returns unchanged",
			err:                   baseErr,
			kind:                  shared.KindCanceled,
			expectedKind:          shared.KindUnknown, // baseErr is not canceled, so KindOf returns Unknown
			shouldContainOriginal: true,
			expectedNil:           false,
		},
		{
			name:                  "already marked error remains unchanged",
			err:                   shared.Wrap(shared.ErrTimeout, "already timeout"),
			kind:                  shared.KindTimeout,
			expectedKind:          shared.KindTimeout,
			shouldContainOriginal: true,
			expectedNil:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.MarkKind(tt.err, tt.kind)

			if tt.expectedNil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)

			// Check kind classification
			assert.Equal(t, tt.expectedKind, shared.KindOf(result))

			// Check original error preservation
			if tt.shouldContainOriginal && tt.err != nil {
				assert.True(t, errors.Is(result, tt.err),
					"marked error should contain original error")
			}
		})
	}
}

func TestMarkKindIdempotent(t *testing.T) {
	baseErr := errors.New("base error")

	// Mark once
	marked := shared.MarkKind(baseErr, shared.KindNotFound)
	require.NotNil(t, marked)
	assert.Equal(t, shared.KindNotFound, shared.KindOf(marked))

	// Mark again with same kind - should not change
	markedAgain := shared.MarkKind(marked, shared.KindNotFound)
	assert.Equal(t, marked, markedAgain, "marking same kind twice should be idempotent")

	// Original error should still be accessible
	assert.True(t, errors.Is(markedAgain, baseErr))
}

func TestMarkKindWithWrappedErrors(t *testing.T) {
	baseErr := errors.New("base error")
	wrappedErr := shared.Wrap(baseErr, "wrapped")

	marked := shared.MarkKind(wrappedErr, shared.KindValidation)

	// Should have validation kind
	assert.Equal(t, shared.KindValidation, shared.KindOf(marked))

	// Should preserve both wrapped and base errors
	assert.True(t, errors.Is(marked, wrappedErr))
	assert.True(t, errors.Is(marked, baseErr))
	assert.True(t, errors.Is(marked, shared.ErrValidation))
}

func TestErrorOf(t *testing.T) {
	tests := []struct {
		name     string
		kind     shared.Kind
		expected error
	}{
		{
			name:     "unknown kind",
			kind:     shared.KindUnknown,
			expected: nil,
		},
		{
			name:     "not found kind",
			kind:     shared.KindNotFound,
			expected: shared.ErrNotFound,
		},
		{
			name:     "validation kind",
			kind:     shared.KindValidation,
			expected: shared.ErrValidation,
		},
		{
			name:     "unauthorized kind",
			kind:     shared.KindUnauthorized,
			expected: shared.ErrUnauthorized,
		},
		{
			name:     "forbidden kind",
			kind:     shared.KindForbidden,
			expected: shared.ErrForbidden,
		},
		{
			name:     "conflict kind",
			kind:     shared.KindConflict,
			expected: shared.ErrConflict,
		},
		{
			name:     "internal kind",
			kind:     shared.KindInternal,
			expected: shared.ErrInternal,
		},
		{
			name:     "timeout kind",
			kind:     shared.KindTimeout,
			expected: shared.ErrTimeout,
		},
		{
			name:     "invariant violated kind",
			kind:     shared.KindInvariantViolated,
			expected: shared.ErrInvariantViolated,
		},
		{
			name:     "dependency failure kind",
			kind:     shared.KindDependencyFailure,
			expected: shared.ErrDependencyFailure,
		},
		{
			name:     "canceled kind",
			kind:     shared.KindCanceled,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.ErrorOf(tt.kind)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCause(t *testing.T) {
	baseErr := errors.New("root cause")
	wrappedOnce := shared.Wrap(baseErr, "level 1")
	wrappedTwice := shared.Wrap(wrappedOnce, "level 2")
	wrappedThrice := shared.Wrap(wrappedTwice, "level 3")

	tests := []struct {
		name     string
		err      error
		expected error
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: nil,
		},
		{
			name:     "unwrapped error",
			err:      baseErr,
			expected: baseErr,
		},
		{
			name:     "wrapped once",
			err:      wrappedOnce,
			expected: baseErr,
		},
		{
			name:     "wrapped twice",
			err:      wrappedTwice,
			expected: baseErr,
		},
		{
			name:     "wrapped thrice",
			err:      wrappedThrice,
			expected: baseErr,
		},
		{
			name:     "sentinel error",
			err:      shared.ErrNotFound,
			expected: shared.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.Cause(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCauseWithJoin(t *testing.T) {
	rootErr1 := errors.New("root cause 1")
	rootErr2 := errors.New("root cause 2")
	rootErr3 := errors.New("root cause 3")

	wrappedErr1 := shared.Wrap(rootErr1, "wrapped 1")
	wrappedErr2 := shared.Wrap(rootErr2, "wrapped 2")

	tests := []struct {
		name        string
		err         error
		expectedAny []error // any of these errors is acceptable as root cause
	}{
		{
			name:        "simple join - returns one of the root errors",
			err:         errors.Join(rootErr1, rootErr2),
			expectedAny: []error{rootErr1, rootErr2},
		},
		{
			name:        "join with wrapped errors",
			err:         errors.Join(wrappedErr1, wrappedErr2),
			expectedAny: []error{rootErr1, rootErr2},
		},
		{
			name:        "nested join",
			err:         errors.Join(errors.Join(rootErr1, rootErr2), rootErr3),
			expectedAny: []error{rootErr1, rootErr2, rootErr3},
		},
		{
			name:        "mixed wrap and join",
			err:         shared.Wrap(errors.Join(rootErr1, rootErr2), "outer wrapper"),
			expectedAny: []error{rootErr1, rootErr2},
		},
		{
			name:        "single error in join",
			err:         errors.Join(rootErr1),
			expectedAny: []error{rootErr1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.Cause(tt.err)
			require.NotNil(t, result, "Cause should not return nil for non-nil error")

			// Check that result is one of the expected root causes
			found := false
			for _, expected := range tt.expectedAny {
				if result == expected {
					found = true
					break
				}
			}
			assert.True(t, found, "Cause should return one of %v, got %v", tt.expectedAny, result)
		})
	}
}

func TestUnwrapAll(t *testing.T) {
	baseErr := errors.New("root cause")
	wrappedOnce := shared.Wrap(baseErr, "level 1")
	wrappedTwice := shared.Wrap(wrappedOnce, "level 2")

	tests := []struct {
		name     string
		err      error
		expected []error
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: nil,
		},
		{
			name:     "unwrapped error",
			err:      baseErr,
			expected: []error{baseErr},
		},
		{
			name:     "wrapped once",
			err:      wrappedOnce,
			expected: []error{wrappedOnce, baseErr},
		},
		{
			name:     "wrapped twice",
			err:      wrappedTwice,
			expected: []error{wrappedTwice, wrappedOnce, baseErr},
		},
		{
			name:     "sentinel error",
			err:      shared.ErrTimeout,
			expected: []error{shared.ErrTimeout},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.UnwrapAll(tt.err)
			assert.Equal(t, len(tt.expected), len(result), "length should match")

			for i, expectedErr := range tt.expected {
				assert.Equal(t, expectedErr, result[i], "error at index %d should match", i)
			}
		})
	}
}

func TestEdgeCases(t *testing.T) {
	t.Run("large error chains", func(t *testing.T) {
		// Create a deep chain of wrapped errors
		baseErr := errors.New("root")
		current := baseErr

		// Create 50-level deep chain
		for i := 0; i < 50; i++ {
			current = shared.Wrapf(current, "level %d", i)
		}

		// Should still work correctly
		assert.Equal(t, shared.KindUnknown, shared.KindOf(current))
		assert.Equal(t, baseErr, shared.Cause(current))

		all := shared.UnwrapAll(current)
		assert.Equal(t, 51, len(all)) // 50 wrappers + 1 base
		assert.Equal(t, current, all[0])
		assert.Equal(t, baseErr, all[len(all)-1])
	})

	t.Run("complex join hierarchies", func(t *testing.T) {
		// Create complex nested joins
		err1 := shared.MarkKind(errors.New("error 1"), shared.KindNotFound)
		err2 := shared.MarkKind(errors.New("error 2"), shared.KindValidation)
		err3 := shared.MarkKind(errors.New("error 3"), shared.KindTimeout)

		level1 := errors.Join(err1, err2)
		level2 := errors.Join(level1, err3)
		level3 := shared.Wrap(level2, "outer context")

		// Should detect all error kinds
		assert.True(t, shared.IsNotFound(level3))
		assert.True(t, shared.IsValidation(level3))
		assert.True(t, shared.IsTimeout(level3))

		// Should prioritize timeout (highest priority)
		assert.Equal(t, shared.KindTimeout, shared.KindOf(level3))

		// Should unwrap everything
		all := shared.UnwrapAll(level3)
		assert.GreaterOrEqual(t, len(all), 6) // at least wrapper + 2 joins + 3 errors
	})

	t.Run("nil and empty cases", func(t *testing.T) {
		// Nil error handling
		assert.Equal(t, shared.KindUnknown, shared.KindOf(nil))
		assert.Nil(t, shared.Cause(nil))
		assert.Nil(t, shared.UnwrapAll(nil))
		assert.Nil(t, shared.Wrap(nil, "context"))
		assert.Nil(t, shared.Wrapf(nil, "context %d", 1))
		assert.Nil(t, shared.MarkKind(nil, shared.KindUnknown))

		// Empty context handling
		err := errors.New("base")
		assert.Equal(t, err, shared.Wrap(err, ""))
		assert.Equal(t, err, shared.Wrapf(err, ""))

		// Predicate with nil
		assert.False(t, shared.IsNotFound(nil))
		assert.False(t, shared.IsTimeout(nil))
		assert.False(t, shared.IsCanceled(nil))
	})

	t.Run("mixed wrapping and joining", func(t *testing.T) {
		// Mix fmt.Errorf %w and errors.Join in complex ways
		base1 := errors.New("base 1")
		base2 := errors.New("base 2")

		wrapped1 := shared.Wrap(base1, "wrapped 1")
		wrapped2 := shared.Wrap(base2, "wrapped 2")

		joined := errors.Join(wrapped1, wrapped2)
		outerWrapped := shared.Wrap(joined, "outer")

		marked := shared.MarkKind(outerWrapped, shared.KindInternal)
		finalWrapped := shared.Wrap(marked, "final")

		// Should preserve all relationships
		assert.True(t, errors.Is(finalWrapped, base1))
		assert.True(t, errors.Is(finalWrapped, base2))
		assert.True(t, errors.Is(finalWrapped, shared.ErrInternal))
		assert.Equal(t, shared.KindInternal, shared.KindOf(finalWrapped))

		// Should unwrap complex hierarchy
		all := shared.UnwrapAll(finalWrapped)
		assert.Greater(t, len(all), 5) // Should have many levels

		// First should be the final wrapped, last should be a root
		assert.Equal(t, finalWrapped, all[0])
	})

	t.Run("cycle protection", func(t *testing.T) {
		// Test that UnwrapAll doesn't infinite loop on theoretical cycles
		// (Note: standard Go errors don't create cycles, but test our protection)

		err := errors.New("base")
		// Create a reasonable chain that our protection should handle
		for i := 0; i < 100; i++ {
			err = shared.Wrap(err, fmt.Sprintf("level %d", i))
		}

		// Should complete without hanging
		all := shared.UnwrapAll(err)
		assert.Equal(t, 101, len(all)) // 100 wrappers + 1 base
	})

	t.Run("multiple marking idempotency", func(t *testing.T) {
		err := errors.New("base")

		// Mark multiple times with same kind
		marked1 := shared.MarkKind(err, shared.KindNotFound)
		marked2 := shared.MarkKind(marked1, shared.KindNotFound)
		marked3 := shared.MarkKind(marked2, shared.KindNotFound)

		// Should all be equal (idempotent)
		assert.Equal(t, marked1, marked2)
		assert.Equal(t, marked2, marked3)

		// Original error should still be accessible
		assert.True(t, errors.Is(marked3, err))
	})
}

func TestInvariantEdgeCases(t *testing.T) {
	t.Run("invariant with complex conditions", func(t *testing.T) {
		// Test invariants with complex expressions
		user := struct {
			Age    int
			Email  string
			Active bool
		}{Age: 25, Email: "test@example.com", Active: true}

		// Valid case
		err := shared.InvariantF(
			user.Age >= 18 && len(user.Email) > 0 && user.Active,
			"user must be adult with email and active, got age=%d email=%s active=%t",
			user.Age, user.Email, user.Active,
		)
		assert.NoError(t, err)

		// Invalid case
		user.Age = 16
		err = shared.InvariantF(
			user.Age >= 18 && len(user.Email) > 0 && user.Active,
			"user must be adult with email and active, got age=%d email=%s active=%t",
			user.Age, user.Email, user.Active,
		)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrInvariantViolated))
		assert.Contains(t, err.Error(), "age=16")
	})

	t.Run("invariant with special characters", func(t *testing.T) {
		// Test invariants with special characters in messages
		err := shared.Invariant(false, "message with: colons, commas; semicolons & ampersands!")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "colons, commas; semicolons & ampersands!")
	})
}

func TestRealWorldScenarios(t *testing.T) {
	t.Run("database layer error handling", func(t *testing.T) {
		// Simulate common database error scenarios
		sqlErr := errors.New("sql: no rows in result set")
		constraintErr := errors.New("sql: constraint violation")
		timeoutErr := errors.New("sql: connection timeout")

		// Adapt SQL errors to domain errors
		notFoundErr := shared.MarkKind(sqlErr, shared.KindNotFound)
		conflictErr := shared.MarkKind(constraintErr, shared.KindConflict)
		dbTimeoutErr := shared.MarkKind(timeoutErr, shared.KindTimeout)

		// Add context
		userNotFound := shared.Wrapf(notFoundErr, "user %d not found", 123)
		emailExists := shared.Wrap(conflictErr, "email already exists")
		dbUnavailable := shared.Wrap(dbTimeoutErr, "database unavailable")

		// Check classifications
		assert.Equal(t, shared.KindNotFound, shared.KindOf(userNotFound))
		assert.Equal(t, shared.KindConflict, shared.KindOf(emailExists))
		assert.Equal(t, shared.KindTimeout, shared.KindOf(dbUnavailable))

		// Original errors should be preserved
		assert.True(t, errors.Is(userNotFound, sqlErr))
		assert.True(t, errors.Is(emailExists, constraintErr))
		assert.True(t, errors.Is(dbUnavailable, timeoutErr))
	})

	t.Run("API error aggregation", func(t *testing.T) {
		// Simulate validation errors from multiple fields
		nameErr := shared.MarkKind(errors.New("name is required"), shared.KindValidation)
		emailErr := shared.MarkKind(errors.New("email format invalid"), shared.KindValidation)
		ageErr := shared.MarkKind(errors.New("age must be positive"), shared.KindValidation)

		// Join validation errors
		validationErrors := errors.Join(nameErr, emailErr, ageErr)

		// Should detect as validation error
		assert.True(t, shared.IsValidation(validationErrors))

		// Should contain all original errors
		assert.True(t, errors.Is(validationErrors, nameErr))
		assert.True(t, errors.Is(validationErrors, emailErr))
		assert.True(t, errors.Is(validationErrors, ageErr))

		// All errors should be accessible
		all := shared.UnwrapAll(validationErrors)
		assert.GreaterOrEqual(t, len(all), 4) // join + 3 errors
	})

	t.Run("service layer error composition", func(t *testing.T) {
		// Simulate complex service interactions
		dbErr := shared.MarkKind(errors.New("connection failed"), shared.KindInternal)
		apiErr := shared.MarkKind(errors.New("rate limited"), shared.KindDependencyFailure)
		cacheErr := shared.MarkKind(errors.New("cache miss"), shared.KindNotFound)

		// Combine different error sources
		serviceErr := errors.Join(dbErr, apiErr)
		fallbackErr := shared.Wrap(cacheErr, "fallback failed")
		compositeErr := errors.Join(serviceErr, fallbackErr)

		finalErr := shared.Wrap(compositeErr, "user profile fetch failed")

		// Should detect not found (highest priority among NotFound/Internal/DependencyFailure)
		// Based on our priority order: NotFound comes before Internal and DependencyFailure
		assert.Equal(t, shared.KindNotFound, shared.KindOf(finalErr))

		// Should preserve all error relationships
		assert.True(t, errors.Is(finalErr, dbErr))
		assert.True(t, errors.Is(finalErr, apiErr))
		assert.True(t, errors.Is(finalErr, cacheErr))
	})
}

func TestUnwrapAllWithJoin(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	err3 := errors.New("error 3")

	wrappedErr1 := shared.Wrap(err1, "wrapped 1")
	wrappedErr2 := shared.Wrap(err2, "wrapped 2")

	tests := []struct {
		name          string
		err           error
		expectedMin   int // minimum expected errors (due to breadth-first traversal variations)
		shouldContain []error
	}{
		{
			name:          "simple join",
			err:           errors.Join(err1, err2),
			expectedMin:   3, // join error + err1 + err2
			shouldContain: []error{err1, err2},
		},
		{
			name:          "join with wrapped errors",
			err:           errors.Join(wrappedErr1, wrappedErr2),
			expectedMin:   5, // join + wrappedErr1 + err1 + wrappedErr2 + err2
			shouldContain: []error{wrappedErr1, err1, wrappedErr2, err2},
		},
		{
			name:          "nested join",
			err:           errors.Join(errors.Join(err1, err2), err3),
			expectedMin:   5, // outer join + inner join + err1 + err2 + err3
			shouldContain: []error{err1, err2, err3},
		},
		{
			name:          "mixed wrap and join",
			err:           shared.Wrap(errors.Join(err1, err2), "outer wrapper"),
			expectedMin:   4, // outer + join + err1 + err2
			shouldContain: []error{err1, err2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.UnwrapAll(tt.err)

			assert.GreaterOrEqual(t, len(result), tt.expectedMin,
				"should have at least %d errors, got %d", tt.expectedMin, len(result))

			// Check that all expected errors are present
			for _, expectedErr := range tt.shouldContain {
				found := false
				for _, actualErr := range result {
					if actualErr == expectedErr {
						found = true
						break
					}
				}
				assert.True(t, found, "should contain error: %v", expectedErr)
			}

			// First error should be the input error
			if len(result) > 0 {
				assert.Equal(t, tt.err, result[0], "first error should be the input error")
			}
		})
	}
}
