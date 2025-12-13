package shared_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

    "sttbot/internal/shared"
)

func TestHasKind(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		kind     shared.Kind
		expected bool
	}{
		// Test nil error
		{
			name:     "nil error with any kind",
			err:      nil,
			kind:     shared.KindNotFound,
			expected: false,
		},
		{
			name:     "nil error with unknown kind",
			err:      nil,
			kind:     shared.KindUnknown,
			expected: true, // KindOf(nil) == KindUnknown
		},

		// Test basic sentinel errors
		{
			name:     "ErrNotFound has KindNotFound",
			err:      shared.ErrNotFound,
			kind:     shared.KindNotFound,
			expected: true,
		},
		{
			name:     "ErrNotFound does not have KindValidation",
			err:      shared.ErrNotFound,
			kind:     shared.KindValidation,
			expected: false,
		},
		{
			name:     "ErrTimeout has KindTimeout",
			err:      shared.ErrTimeout,
			kind:     shared.KindTimeout,
			expected: true,
		},

		// Test wrapped errors
		{
			name:     "wrapped ErrNotFound has KindNotFound",
			err:      shared.Wrap(shared.ErrNotFound, "user not found"),
			kind:     shared.KindNotFound,
			expected: true,
		},
		{
			name:     "wrapped ErrNotFound does not have KindValidation",
			err:      shared.Wrap(shared.ErrNotFound, "user not found"),
			kind:     shared.KindValidation,
			expected: false,
		},

		// Test marked errors
		{
			name:     "marked error has correct kind",
			err:      shared.MarkKind(errors.New("base"), shared.KindValidation),
			kind:     shared.KindValidation,
			expected: true,
		},
		{
			name:     "marked error does not have other kinds",
			err:      shared.MarkKind(errors.New("base"), shared.KindValidation),
			kind:     shared.KindInternal,
			expected: false,
		},

		// Test special cases: context errors
		{
			name:     "context.Canceled has KindCanceled",
			err:      context.Canceled,
			kind:     shared.KindCanceled,
			expected: true,
		},
		{
			name:     "context.Canceled does not have KindTimeout",
			err:      context.Canceled,
			kind:     shared.KindTimeout,
			expected: false,
		},
		{
			name:     "context.DeadlineExceeded has KindTimeout",
			err:      context.DeadlineExceeded,
			kind:     shared.KindTimeout,
			expected: true,
		},
		{
			name:     "context.DeadlineExceeded does not have KindCanceled",
			err:      context.DeadlineExceeded,
			kind:     shared.KindCanceled,
			expected: false,
		},

		// Test unknown errors
		{
			name:     "random error has KindUnknown",
			err:      errors.New("random error"),
			kind:     shared.KindUnknown,
			expected: true,
		},
		{
			name:     "random error does not have KindNotFound",
			err:      errors.New("random error"),
			kind:     shared.KindNotFound,
			expected: false,
		},

		// Test all kinds for completeness
		{
			name:     "ErrValidation has KindValidation",
			err:      shared.ErrValidation,
			kind:     shared.KindValidation,
			expected: true,
		},
		{
			name:     "ErrUnauthorized has KindUnauthorized",
			err:      shared.ErrUnauthorized,
			kind:     shared.KindUnauthorized,
			expected: true,
		},
		{
			name:     "ErrForbidden has KindForbidden",
			err:      shared.ErrForbidden,
			kind:     shared.KindForbidden,
			expected: true,
		},
		{
			name:     "ErrConflict has KindConflict",
			err:      shared.ErrConflict,
			kind:     shared.KindConflict,
			expected: true,
		},
		{
			name:     "ErrInternal has KindInternal",
			err:      shared.ErrInternal,
			kind:     shared.KindInternal,
			expected: true,
		},
		{
			name:     "ErrInvariantViolated has KindInvariantViolated",
			err:      shared.ErrInvariantViolated,
			kind:     shared.KindInvariantViolated,
			expected: true,
		},
		{
			name:     "ErrDependencyFailure has KindDependencyFailure",
			err:      shared.ErrDependencyFailure,
			kind:     shared.KindDependencyFailure,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.HasKind(tt.err, tt.kind)
			assert.Equal(t, tt.expected, result)

			// Verify equivalence with KindOf(err) == kind
			kindOfResult := shared.KindOf(tt.err) == tt.kind
			assert.Equal(t, kindOfResult, result, "HasKind should be equivalent to KindOf(err) == kind")
		})
	}
}

func TestHasKindWithJoin(t *testing.T) {
	// Test HasKind with errors.Join to ensure it follows priority rules
	tests := []struct {
		name     string
		errors   []error
		kind     shared.Kind
		expected bool
		reason   string
	}{
		{
			name: "joined errors with timeout - has timeout",
			errors: []error{
				shared.ErrNotFound,
				shared.ErrTimeout,
				shared.ErrValidation,
			},
			kind:     shared.KindTimeout,
			expected: true,
			reason:   "should detect timeout as highest priority",
		},
		{
			name: "joined errors with timeout - does not have not found",
			errors: []error{
				shared.ErrNotFound,
				shared.ErrTimeout,
				shared.ErrValidation,
			},
			kind:     shared.KindNotFound,
			expected: false,
			reason:   "should not detect not found when timeout has higher priority",
		},
		{
			name: "joined errors without high priority - has dependency failure",
			errors: []error{
				shared.ErrDependencyFailure,
				shared.ErrInternal,
				shared.ErrInvariantViolated,
			},
			kind:     shared.KindDependencyFailure,
			expected: true,
			reason:   "should detect dependency failure as highest among these",
		},
		{
			name: "joined errors without high priority - does not have internal",
			errors: []error{
				shared.ErrDependencyFailure,
				shared.ErrInternal,
				shared.ErrInvariantViolated,
			},
			kind:     shared.KindInternal,
			expected: false,
			reason:   "should not detect internal when dependency failure has higher priority",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			joinedErr := errors.Join(tt.errors...)
			result := shared.HasKind(joinedErr, tt.kind)
			assert.Equal(t, tt.expected, result, tt.reason)
		})
	}
}

func TestSentinelOf(t *testing.T) {
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
			name:     "timeout kind",
			kind:     shared.KindTimeout,
			expected: shared.ErrTimeout,
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
			result := shared.SentinelOf(tt.kind)
			assert.Equal(t, tt.expected, result)

			// Verify equivalence with ErrorOf
			errorOfResult := shared.ErrorOf(tt.kind)
			assert.Equal(t, errorOfResult, result, "SentinelOf should be equivalent to ErrorOf")
		})
	}
}
