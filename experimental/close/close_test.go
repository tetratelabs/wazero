package close_test

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/experimental/close"
	internalclose "github.com/tetratelabs/wazero/internal/close"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestWithNotification(t *testing.T) {
	tests := []struct {
		name         string
		notification close.Notification
		expected     bool
	}{
		{
			name:     "returns input when notification nil",
			expected: false,
		},
		{
			name:         "decorates with notification",
			notification: close.NotificationFunc(func(context.Context, uint32) {}),
			expected:     true,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			if decorated := close.WithNotification(testCtx, tc.notification); tc.expected {
				require.NotNil(t, decorated.Value(internalclose.NotificationKey{}))
			} else {
				require.Same(t, testCtx, decorated)
			}
		})
	}
}
